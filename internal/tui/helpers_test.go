package tui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// press 는 키 하나를 누른 것처럼 Update를 호출하고 새 Model을 돌려준다.
func press(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyPressMsg
	switch key {
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "space":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func loadedModel(items ...item) Model {
	m := Model{home: "/home/me"}
	next, _ := m.Update(entriesLoadedMsg{items: items})
	return next.(Model)
}

func entry(name, target string) manifest.Entry {
	return manifest.Entry{Name: name, Source: "files/" + name, Target: target}
}

// viewText 는 화면에서 색상 코드를 걷어낸 글자만 돌려준다.
func viewText(m Model) string {
	return ansi.Strip(m.View().Content)
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

// fakeRepo 는 files(리포 경로 → 내용)를 Contents API 형식으로 돌려주는 가짜 GitHub 서버를 띄운다.
// files에 없는 경로는 404를 돌려준다.
func fakeRepo(t *testing.T, files map[string]string) *github.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// toggleFixture 는 가짜 리포, 임시 로컬 사본 폴더, 임시 홈에 놓인 항목 하나로 Model을 만든다.
func toggleFixture(t *testing.T, files map[string]string) (m Model, target string) {
	t.Helper()
	home := t.TempDir()
	copies := localcopy.New(t.TempDir())
	target = filepath.Join(home, ".gitconfig")

	e := manifest.Entry{Name: "gitconfig", Source: "files/git/.gitconfig", Target: target}
	copyPath, err := copies.Path(e.Source)
	if err != nil {
		t.Fatal(err)
	}
	m = New(fakeRepo(t, files), copies)
	m.home = home
	next, _ := m.Update(entriesLoadedMsg{items: []item{{entry: e, copyPath: copyPath}}})
	return next.(Model), target
}

// linkedFixture 는 백업 없이 연결된 항목 하나로 Model을 만든다.
func linkedFixture(t *testing.T) (Model, string) {
	t.Helper()
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("setup: state = %d (err %v)", m.items[0].state, m.items[0].err)
	}
	return m, target
}

// runCmd 는 백그라운드 작업을 바로 실행하고, 그 결과 메시지를 Update에 넣는다.
// 묶인 작업(tea.Batch)은 풀어서 하나씩 실행하고, 스피너 tick은 건너뛴다.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			continue
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// runAll 은 cmd를 실행해 나온 메시지를 모두 모은다. tea.Batch로 묶인 작업도 풀어서 실행한다.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, runAll(c)...)
	}
	return msgs
}
