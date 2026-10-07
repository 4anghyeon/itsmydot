package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient 는 handler로 응답하는 가짜 서버를 띄우고, 그 서버를 바라보는 Client를 만든다.
// 테스트가 끝나면 t.Cleanup이 서버를 자동으로 닫는다.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewClient("owner", "repo")
	c.BaseURL = srv.URL
	return c
}

// writeJSON 은 테스트용 응답 JSON을 쓴다.
func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestFetch(t *testing.T) {
	want := "export PATH=$PATH:~/go/bin\n"
	// GitHub처럼 base64 결과를 중간중간 줄바꿈해서 보낸다.
	encoded := base64.StdEncoding.EncodeToString([]byte(want))
	encoded = encoded[:10] + `\n` + encoded[10:]

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/contents/files/zsh/.zshrc" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent header missing")
		}
		writeJSON(w, `{"type":"file","encoding":"base64","content":"`+encoded+`"}`)
	})

	got, err := c.Fetch(context.Background(), "files/zsh/.zshrc")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestFetchNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	_, err := c.Fetch(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFetchRateLimit(t *testing.T) {
	reset := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1791374400") // reset 과 같은 시각
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	})

	_, err := c.Fetch(context.Background(), "manifest.yaml")
	// errors.As 는 감싸진 에러 안에서 특정 타입(*RateLimitError)을 찾아 rle에 넣어 준다.
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *RateLimitError", err)
	}
	if !rle.Reset.Equal(reset) {
		t.Errorf("Reset = %v, want %v", rle.Reset, reset)
	}
}

func TestFetchErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"forbidden without rate limit", http.StatusForbidden, `{}`, "unexpected status 403"},
		{"server error", http.StatusInternalServerError, `{}`, "unexpected status 500"},
		{"directory", http.StatusOK, `[{"type":"file"}]`, "is it a directory?"},
		{"symlink", http.StatusOK, `{"type":"symlink"}`, `not a file (type "symlink")`},
		{"too large", http.StatusOK, `{"type":"file","encoding":"none","content":""}`, `unsupported encoding "none"`},
		{"bad base64", http.StatusOK, `{"type":"file","encoding":"base64","content":"!!!"}`, "decode content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := c.Fetch(context.Background(), "x")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEscapePath(t *testing.T) {
	tests := map[string]string{
		"files/zsh/.zshrc":  "files/zsh/.zshrc",
		"/manifest.yaml":    "manifest.yaml",
		"files/my file.txt": "files/my%20file.txt",
		"files/a#b":         "files/a%23b",
	}
	for in, want := range tests {
		if got := escapePath(in); got != want {
			t.Errorf("escapePath(%q) = %q, want %q", in, got, want)
		}
	}
}
