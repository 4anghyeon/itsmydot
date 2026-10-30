package tui

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
)

// loadEntries가 manifest를 받아 항목마다 상태를 제대로 판단하는지 가짜 서버와 임시 폴더로 확인한다.
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

	// .a는 로컬 사본을 가리키는 symlink이고 .b는 없고 .c는 일반 파일이다.
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

// 연결된 항목 중 로컬 사본이 저장소 파일과 다른 것만 업데이트 있음으로 본다.
func TestLoadEntriesMarksOutdated(t *testing.T) {
	home := t.TempDir()
	copies := localcopy.New(t.TempDir())
	client := fakeRepo(t, map[string]string{
		"manifest.yaml": "entries:\n" +
			"  - {name: changed, source: files/a, target: " + filepath.Join(home, ".a") + "}\n" +
			"  - {name: same, source: files/b, target: " + filepath.Join(home, ".b") + "}\n" +
			"  - {name: off, source: files/c, target: " + filepath.Join(home, ".c") + "}\n",
		"files/a": "new",
		"files/b": "same",
		"files/c": "new",
	})

	// .a와 .b는 연결돼 있고 .c는 예전에 받은 사본만 남아 있다.
	for source, content := range map[string]string{"files/a": "old", "files/b": "same", "files/c": "old"} {
		if _, err := copies.Write(source, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for source, target := range map[string]string{"files/a": ".a", "files/b": ".b"} {
		copyPath, _ := copies.Path(source)
		if err := os.Symlink(copyPath, filepath.Join(home, target)); err != nil {
			t.Fatal(err)
		}
	}

	msg := loadEntries(context.Background(), client, copies)
	if msg.err != nil {
		t.Fatalf("loadEntries: %v", msg.err)
	}
	if len(msg.hashes) != 4 {
		t.Errorf("hashes = %v, want the repo file hashes for the preview", msg.hashes)
	}
	for i, want := range []bool{true, false, false} {
		if got := msg.items[i].outdated; got != want {
			t.Errorf("%s: outdated = %v, want %v", msg.items[i].entry.Name, got, want)
		}
	}
}

// 저장소 파일 해시를 못 받아도 목록은 그대로 뜨고 업데이트 표시만 빠진다.
func TestLoadEntriesWithoutHashes(t *testing.T) {
	home := t.TempDir()
	copies := localcopy.New(t.TempDir())
	manifestYAML := "entries:\n  - {name: a, source: files/a, target: " + filepath.Join(home, ".a") + "}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/contents/manifest.yaml" {
			http.NotFound(w, r)
			return
		}
		content := base64.StdEncoding.EncodeToString([]byte(manifestYAML))
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + content + `"}`))
	}))
	t.Cleanup(srv.Close)
	client := github.NewClient("o", "r")
	client.BaseURL = srv.URL

	copyPath, _ := copies.Write("files/a", []byte("old"))
	if err := os.Symlink(copyPath, filepath.Join(home, ".a")); err != nil {
		t.Fatal(err)
	}

	msg := loadEntries(context.Background(), client, copies)
	if msg.err != nil || len(msg.items) != 1 {
		t.Fatalf("items = %d err = %v", len(msg.items), msg.err)
	}
	if msg.items[0].state != synced || msg.items[0].outdated {
		t.Errorf("state = %d outdated = %v, want synced without the mark", msg.items[0].state, msg.items[0].outdated)
	}
}
