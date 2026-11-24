package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// timeoutError 는 http.Client의 Timeout이 지났을 때 url.Error 안에 들어 있는 에러를 흉내 낸다.
type timeoutError struct{}

func (timeoutError) Error() string { return "timeout" }

func (timeoutError) Timeout() bool { return true }

func (timeoutError) Temporary() bool { return true }

func TestErrorReason(t *testing.T) {
	reset := time.Date(2026, 1, 1, 15, 4, 0, 0, time.Local)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not found", fmt.Errorf("fetch x: %w", github.ErrNotFound), "저장소에 파일이 없어요"},
		{"rate limit", fmt.Errorf("fetch x: %w", &github.RateLimitError{Reset: reset}), "GitHub 요청 한도를 다 썼어요. 15:04 이후에 다시 해 주세요"},
		{"rate limit without reset", &github.RateLimitError{}, "GitHub 요청 한도를 다 썼어요. 잠시 뒤에 다시 해 주세요"},
		{"invalid manifest", &manifest.InvalidError{Problems: []string{"a 항목에 target이 없어요"}}, "manifest.yaml 형식이 잘못됐어요"},
		{"timeout", &url.Error{Op: "Get", URL: "https://api.github.com", Err: timeoutError{}}, "GitHub가 응답하지 않아요. 잠시 뒤에 다시 해 주세요"},
		{"offline", &url.Error{Op: "Get", URL: "https://api.github.com", Err: errors.New("no such host")}, "GitHub에 접속하지 못했어요. 네트워크를 확인해 주세요"},
		{"permission", &fs.PathError{Op: "open", Path: "/home/me/.zshrc", Err: fs.ErrPermission}, "~/.zshrc 파일에 접근할 권한이 없어요"},
		{"permission on symlink", &os.LinkError{Op: "symlink", Old: "/a", New: "/home/me/.zshrc", Err: fs.ErrPermission}, "~/.zshrc 파일에 접근할 권한이 없어요"},
		{"brew failed", fmt.Errorf("brew tap: %w", &exec.ExitError{}), "brew 명령이 실패했어요"},
		{"other", errors.New("boom"), "예상하지 못한 문제가 생겼어요"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorReason(tt.err, "/home/me"); got != tt.want {
				t.Errorf("errorReason = %q, want %q", got, tt.want)
			}
		})
	}
}
