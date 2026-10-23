package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// timeoutError 는 http.Client의 Timeout이 지났을 때 url.Error 안에 들어 있는 에러를 흉내 낸다.
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
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
		{"other", errors.New("boom"), "예상하지 못한 문제가 생겼어요"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorReason(tt.err, "/home/me"); got != tt.want {
				t.Errorf("errorReason = %q, want %q", got, tt.want)
			}
		})
	}
}

// manifest 형식이 잘못됐으면 사유 아래에 틀린 곳을 한 줄씩 보여 준다.
func TestViewShowsManifestProblems(t *testing.T) {
	m := loadedModel()
	m.loadErr = &manifest.InvalidError{Problems: []string{"a 항목에 source가 없어요", "a 항목에 target이 없어요"}}
	v := viewText(m)
	for _, want := range []string{"manifest를 불러오지 못했어요. manifest.yaml 형식이 잘못됐어요", "a 항목에 source가 없어요", "a 항목에 target이 없어요"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

// 목록에는 사유 문장만 보이고 원래 에러는 오류 항목의 메뉴에서만 보인다.
func TestMenuShowsOriginalError(t *testing.T) {
	m := loadedModel(item{
		entry: entry("gitconfig", "/home/me/.gitconfig"),
		state: errored,
		err:   fmt.Errorf("fetch files/gitconfig: %w", github.ErrNotFound),
	})
	if v := viewText(m); strings.Contains(v, "fetch files/gitconfig") || !strings.Contains(v, "(저장소에 파일이 없어요)") {
		t.Errorf("list should show only the reason:\n%s", v)
	}
	m, _ = press(t, m, "enter")
	if v := viewText(m); !strings.Contains(v, "fetch files/gitconfig: not found") {
		t.Errorf("menu should show the original error:\n%s", v)
	}
}
