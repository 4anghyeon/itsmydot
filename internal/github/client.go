// Package github 는 GitHub Contents API로 리포의 파일 내용을 받아 온다.
// public 리포만 다루므로 git 바이너리나 인증 토큰 없이 GET 요청만 보낸다.
package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL 은 GitHub REST API 주소다. 테스트에서는 가짜 서버 주소로 바꿔 끼운다.
const DefaultBaseURL = "https://api.github.com"

// ErrNotFound 는 요청한 경로가 리포에 없을 때 [Client.Fetch] 가 돌려준다.
var ErrNotFound = errors.New("not found")

// RateLimitError 는 인증 없이 쓸 수 있는 요청 한도인 시간당 60회를 다 썼을 때 돌려주는 에러다.
type RateLimitError struct {
	Reset time.Time // 한도가 다시 채워지는 시각
}

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "github API rate limit exceeded"
	}
	return fmt.Sprintf("github API rate limit exceeded (resets at %s)", e.Reset.Local().Format("15:04"))
}

// Client 는 Owner의 Repo 리포에서 파일을 받아 온다.
type Client struct {
	Owner      string
	Repo       string
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient 는 기본 주소와 10초 타임아웃을 쓰는 Client를 만든다.
func NewClient(owner, repo string) *Client {
	return &Client{
		Owner:      owner,
		Repo:       repo,
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// contentResponse 는 Contents API 응답 JSON 중 필요한 필드만 담는다.
type contentResponse struct {
	Type     string `json:"type"`     // file, dir, symlink 중 하나
	Encoding string `json:"encoding"` // 보통 base64이고 1MB가 넘는 파일은 none으로 온다
	Content  string `json:"content"`
}

// Fetch 는 files/zsh/.zshrc 처럼 리포 루트 기준 경로에 있는 파일 내용을 받아 온다.
// 파일이 없으면 [ErrNotFound] 를, 요청 한도를 넘었으면 [*RateLimitError] 를 감싼 에러를 돌려준다.
func (c *Client) Fetch(ctx context.Context, path string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s",
		strings.TrimRight(c.BaseURL, "/"), url.PathEscape(c.Owner), url.PathEscape(c.Repo), escapePath(path))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}
	// User-Agent가 없으면 GitHub가 요청을 거부한다. 나머지 둘은 GitHub가 권장하는 헤더다.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "itsmydot")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if err := checkStatus(resp); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}

	var body contentResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("fetch %s: unexpected response (is it a directory?): %w", path, err)
	}
	if body.Type != "file" {
		return nil, fmt.Errorf("fetch %s: not a file (type %q)", path, body.Type)
	}
	if body.Encoding != "base64" {
		return nil, fmt.Errorf("fetch %s: unsupported encoding %q (file larger than 1MB?)", path, body.Encoding)
	}

	data, err := base64.StdEncoding.DecodeString(body.Content)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: decode content: %w", path, err)
	}
	return data, nil
}

// checkStatus 는 200이 아닌 응답을 상태 코드에 맞는 에러로 바꾼다.
func checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusOK:
		return nil

	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound

	// 요청 한도 초과는 403이나 429로 온다. 403은 권한 문제일 때도 오기 때문에
	// 남은 횟수 헤더가 0일 때만 한도 초과로 본다.
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return &RateLimitError{Reset: parseReset(resp.Header.Get("X-RateLimit-Reset"))}

	default:
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
}

func parseReset(v string) time.Time {
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// escapePath 는 경로를 / 로 나눈 각 부분만 URL 인코딩한다. / 는 경로 구분자로 남아 있어야 한다.
func escapePath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
