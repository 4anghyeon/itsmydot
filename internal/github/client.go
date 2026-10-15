// Package github 는 GitHub Contents API로 리포의 파일 내용을 받아 온다.
// public 리포만 다루므로 git 바이너리나 인증 토큰 없이 GET 요청만 보낸다.
package github

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
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
	resp, err := c.get(ctx, "contents/"+escapePath(path))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

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

// treeResponse 는 Git Trees API 응답 JSON 중 필요한 필드만 담는다.
type treeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"` // 파일은 blob, 폴더는 tree
		SHA  string `json:"sha"`
	} `json:"tree"`
}

// FileHashes 는 기본 브랜치에 있는 모든 파일의 git blob 해시를 리포 경로별로 돌려준다.
// 파일마다 Contents API를 부르지 않고 Git Trees API를 한 번만 불러서 요청 한도를 아낀다.
func (c *Client) FileHashes(ctx context.Context) (map[string]string, error) {
	resp, err := c.get(ctx, "git/trees/HEAD?recursive=1")
	if err != nil {
		return nil, fmt.Errorf("fetch file hashes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("fetch file hashes: %w", err)
	}
	hashes := make(map[string]string, len(body.Tree))
	for _, entry := range body.Tree {
		if entry.Type == "blob" {
			hashes[entry.Path] = entry.SHA
		}
	}
	return hashes, nil
}

// BlobHash 는 data의 git blob 해시다. [Client.FileHashes] 가 돌려주는 해시와 비교할 수 있다.
// git은 내용 앞에 "blob <크기>" 와 NUL 한 바이트를 붙여서 SHA-1을 구한다.
func BlobHash(data []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(data))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// get 은 리포 아래 path로 GET 요청을 보내고, 200이 아니면 상태 코드에 맞는 에러를 돌려준다.
// 성공하면 호출한 쪽이 응답 본문을 닫아야 한다.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/%s",
		strings.TrimRight(c.BaseURL, "/"), url.PathEscape(c.Owner), url.PathEscape(c.Repo), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// User-Agent가 없으면 GitHub가 요청을 거부한다. 나머지 둘은 GitHub가 권장하는 헤더다.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "itsmydot")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
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
