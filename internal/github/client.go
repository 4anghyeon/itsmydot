// Package github 는 GitHub Contents API로 리포의 파일 내용을 읽어 온다.
// git 바이너리나 인증 토큰 없이, public 리포에 GET 요청만 보낸다.
//
// 전체 흐름:
//
//	Fetch(ctx, 경로)
//	  ├─ 1. 요청 URL 조립: {BaseURL}/repos/{owner}/{repo}/contents/{경로}
//	  ├─ 2. GET 요청 전송
//	  ├─ 3. 상태 코드 확인: 404 → ErrNotFound, 레이트 리밋 → *RateLimitError
//	  ├─ 4. JSON 응답 → contentResponse 구조체로 변환
//	  └─ 5. base64로 인코딩된 content 디코딩 → 원본 파일 바이트
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

// DefaultBaseURL 은 GitHub REST API의 기본 주소다. 테스트에서는 가짜 서버 주소로 바꿔 끼운다.
const DefaultBaseURL = "https://api.github.com"

var ErrNotFound = errors.New("not found")

// RateLimitError 는 비인증 요청 한도(시간당 60회)를 다 썼을 때 돌려주는 에러
type RateLimitError struct {
	Reset time.Time // 한도가 다시 채워지는 시각
}

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "github API rate limit exceeded"
	}
	return fmt.Sprintf("github API rate limit exceeded (resets at %s)", e.Reset.Local().Format("15:04"))
}

type Client struct {
	Owner      string
	Repo       string
	BaseURL    string
	HTTPClient *http.Client
}

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
	Type     string `json:"type"`     // "file", "dir", "symlink" 등
	Encoding string `json:"encoding"` // 보통 "base64". 1MB가 넘는 파일은 "none"
	Content  string `json:"content"`
}

// Fetch 는 리포 루트 기준 경로(예: "files/zsh/.zshrc")의 파일 내용을 받아 온다.
func (c *Client) Fetch(ctx context.Context, path string) ([]byte, error) {
	// 1. 요청 URL 조립.
	//    경로의 각 부분을 URL 인코딩해서 공백이나 특수문자가 있어도 안전하게 만든다.
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s",
		strings.TrimRight(c.BaseURL, "/"), url.PathEscape(c.Owner), url.PathEscape(c.Repo), escapePath(path))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}
	// GitHub 권장 헤더. User-Agent가 없으면 GitHub가 요청을 거부한다.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "itsmydot")

	// 2. 요청 전송. 네트워크 오류나 타임아웃이면 여기서 에러가 난다.
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	// 3. 상태 코드 확인.
	if err := checkStatus(resp); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}

	// 4. JSON 응답 → 구조체.
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

	// 5. base64 디코딩.
	data, err := base64.StdEncoding.DecodeString(body.Content)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: decode content: %w", path, err)
	}
	return data, nil
}

// checkStatus 는 HTTP 상태 코드를 보고 실패면 알맞은 에러를 만든다. 200이면 nil.
func checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusOK:
		return nil

	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound

	// rate limit은 403 또는 429로 온다.
	// 403은 권한 문제일 수도 있으므로, 남은 횟수 헤더가 0일 때만 rate limit으로 본다.
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

// escapePath 는 "files/zsh/.zshrc"처럼 / 로 나뉜 경로의 각 부분만 URL 인코딩한다.
func escapePath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
