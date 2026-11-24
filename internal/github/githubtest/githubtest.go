// Package githubtest 는 테스트에서 실제 GitHub 대신 쓰는 가짜 저장소를 띄운다.
package githubtest

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/4anghyeon/itsmydot/internal/github"
)

// NewClient 는 files에 담긴 저장소 경로별 내용을 Contents API 형식으로 돌려주는 가짜 GitHub 서버를 띄우고
// 그 서버를 바라보는 클라이언트를 돌려준다. 서버는 테스트가 끝나면 닫힌다.
// files에 없는 경로는 404를 돌려준다. Git Trees API에는 files 전체의 blob 해시로 답한다.
func NewClient(t *testing.T, files map[string]string) *github.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/o/r/git/trees/HEAD" {
			var entries []string
			for path, content := range files {
				entries = append(entries, fmt.Sprintf(`{"path":%q,"type":"blob","sha":%q}`, path, github.BlobHash([]byte(content))))
			}
			_, _ = w.Write([]byte(`{"tree":[` + strings.Join(entries, ",") + `]}`))
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/repos/o/r/contents/")
		content, ok := files[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + encoded + `"}`))
	}))
	t.Cleanup(srv.Close)
	c := github.NewClient("o", "r")
	c.BaseURL = srv.URL
	return c
}
