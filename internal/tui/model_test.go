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
	)
	v := m.View().Content
	for _, want := range []string{
		"> [x] gitconfig  ~/.gitconfig",
		"  [!] zshrc      ~/.zshrc   (다른 파일이 있음)",
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
