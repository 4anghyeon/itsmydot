package tui

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

func TestCursorMovesWithinBounds(t *testing.T) {
	m := loadedModel(
		item{entry: entry("a", "/home/me/.a")},
		item{entry: entry("b", "/home/me/.b")},
	)

	m, _ = press(t, m, "up") // 맨 위에서 위로 → 그대로
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "j")
	if m.cursor != 1 { // 맨 아래에서 더 내려가지 않음
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
	m, _ = press(t, m, "k")
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestQuit(t *testing.T) {
	_, cmd := press(t, loadedModel(), "q")
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit")
	}
}

func TestView(t *testing.T) {
	loading := Model{}.View().Content
	if !strings.Contains(loading, "불러오는 중") {
		t.Errorf("loading view = %q", loading)
	}

	failed := loadedModel()
	failed.loadErr = errors.New("boom")
	if v := failed.View().Content; !strings.Contains(v, "boom") {
		t.Errorf("error view = %q", v)
	}

	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: conflict},
		item{entry: entry("vimrc", "/home/me/.vimrc"), state: errored, err: errors.New("not found")},
		item{entry: entry("tmux", "/home/me/.tmux.conf"), state: syncing},
	)
	v := m.View().Content
	for _, want := range []string{
		"> [✓] gitconfig  ~/.gitconfig",
		"  [!] zshrc      ~/.zshrc   (다른 파일이 있음)",
		"  [✗] vimrc      ~/.vimrc   (not found)",
		"  [~] tmux       ~/.tmux.conf",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

// loadEntries는 GitHub에서 manifest를 받아 각 항목의 상태를 확인한다. 가짜 서버와 임시 폴더로 확인한다.
func TestLoadEntries(t *testing.T) {
	home := t.TempDir()
	copies := localcopy.New(t.TempDir())

	manifestYAML := "entries:\n" +
		"  - {name: linked, source: files/a, target: " + filepath.Join(home, ".a") + "}\n" +
		"  - {name: empty, source: files/b, target: " + filepath.Join(home, ".b") + "}\n" +
		"  - {name: taken, source: files/c, target: " + filepath.Join(home, ".c") + "}\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := base64.StdEncoding.EncodeToString([]byte(manifestYAML))
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + content + `"}`))
	}))
	t.Cleanup(srv.Close)
	client := github.NewClient("o", "r")
	client.BaseURL = srv.URL

	// .a는 로컬 사본을 가리키는 symlink, .b는 없음, .c는 일반 파일
	aCopy, err := copies.Path("files/a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(aCopy, filepath.Join(home, ".a")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".c"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := loadEntries(context.Background(), client, copies)
	if msg.err != nil {
		t.Fatalf("loadEntries: %v", msg.err)
	}
	want := []itemState{synced, notSynced, conflict}
	if len(msg.items) != len(want) {
		t.Fatalf("got %d items, want %d", len(msg.items), len(want))
	}
	for i, it := range msg.items {
		if it.state != want[i] {
			t.Errorf("%s: state = %d, want %d", it.entry.Name, it.state, want[i])
		}
	}
	if msg.items[0].copyPath != aCopy {
		t.Errorf("copyPath = %q, want %q", msg.items[0].copyPath, aCopy)
	}
}

func TestTildify(t *testing.T) {
	tests := map[string]string{
		"/home/me":          "~",
		"/home/me/.zshrc":   "~/.zshrc",
		"/home/me/a/b":      "~/a/b",
		"/home/meow/.zshrc": "/home/meow/.zshrc",
		"/etc/x":            "/etc/x",
	}
	for in, want := range tests {
		if got := tildify(in, "/home/me"); got != want {
			t.Errorf("tildify(%q) = %q, want %q", in, got, want)
		}
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

// runCmd 는 백그라운드 작업을 바로 실행하고, 그 결과 메시지를 Update에 넣는다.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestToggleOnAndOff(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "[user]\n"})

	// 켜기: 바로 syncing이 되고, 작업이 끝나면 synced
	m, cmd := press(t, m, "space")
	if m.items[0].state != syncing {
		t.Fatalf("state = %d, want syncing", m.items[0].state)
	}
	m, _ = press(t, m, "space") // syncing 중에는 무시
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("state = %d, want synced (err %v)", m.items[0].state, m.items[0].err)
	}
	assertContent(t, target, "[user]\n")

	// 끄기: 백업이 없었으므로 target이 사라지고 notSynced
	m, cmd = press(t, m, "space")
	if cmd != nil {
		t.Error("turning off should not start a background command")
	}
	if m.items[0].state != notSynced {
		t.Errorf("state = %d, want notSynced", m.items[0].state)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target should be removed, got err %v", err)
	}
}

func TestToggleBacksUpAndRestores(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("state = %d, want synced (err %v)", m.items[0].state, m.items[0].err)
	}
	assertContent(t, target, "from repo")
	assertContent(t, target+".itsmydot.bak", "mine")
	if !strings.Contains(m.status, "백업") {
		t.Errorf("status = %q, want backup notice", m.status)
	}

	m, _ = press(t, m, "space")
	if m.items[0].state != conflict {
		t.Errorf("state = %d, want conflict (original file restored)", m.items[0].state)
	}
	assertContent(t, target, "mine")
}

func TestToggleFetchError(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{}) // 리포에 파일이 없음 → 404

	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)
	if m.items[0].state != errored || m.items[0].err == nil {
		t.Fatalf("state = %d err = %v, want errored", m.items[0].state, m.items[0].err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target should be untouched, got err %v", err)
	}
	if v := m.View().Content; !strings.Contains(v, "not found") {
		t.Errorf("view should show the error:\n%s", v)
	}
}

// Update가 돌려준 새 Model을 바꿔도, 이전 Model의 항목은 그대로여야 한다.
func TestToggleDoesNotMutatePreviousModel(t *testing.T) {
	before, _ := toggleFixture(t, map[string]string{"files/git/.gitconfig": "x"})
	_, _ = press(t, before, "space")
	if before.items[0].state != notSynced {
		t.Errorf("previous model changed: state = %d", before.items[0].state)
	}
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
