package tui

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

func TestToggleOnAndOff(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "[user]\n"})

	// 켜면 바로 syncing이 되고 작업이 끝나면 synced가 된다.
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

	// 백업이 없어서 끌 때 먼저 묻는다. 2번 파일도 지우기를 고르면 target이 사라진다.
	m, cmd = press(t, m, "space")
	if cmd != nil || m.confirming != "gitconfig" || m.confirmKind != confirmUnlink {
		t.Fatalf("want unlink prompt, got cmd=%v confirming=%q kind=%d", cmd != nil, m.confirming, m.confirmKind)
	}
	m, _ = press(t, m, "2")
	if !strings.Contains(m.status, "남아 있음") {
		t.Errorf("status = %q, want where the content remains", m.status)
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

	// 다른 파일이 있으면 먼저 묻는다. 답하기 전에는 디스크를 건드리지 않는다.
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirming != "gitconfig" {
		t.Fatalf("want confirm prompt, got cmd=%v confirming=%q", cmd != nil, m.confirming)
	}
	assertContent(t, target, "mine")

	m, cmd = press(t, m, "1")
	if m.confirming != "" || m.items[0].state != syncing {
		t.Fatalf("after 1: confirming=%q state=%d", m.confirming, m.items[0].state)
	}
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

func TestConfirmCancel(t *testing.T) {
	for _, key := range []string{"2", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
			if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
				t.Fatal(err)
			}

			m, _ = press(t, m, "space")
			m, cmd := press(t, m, key)
			if cmd != nil || m.confirming != "" {
				t.Fatalf("want cancelled, got cmd=%v confirming=%q", cmd != nil, m.confirming)
			}
			if m.items[0].state != conflict {
				t.Errorf("state = %d, want conflict", m.items[0].state)
			}
			if !strings.Contains(m.status, "취소") {
				t.Errorf("status = %q", m.status)
			}
			assertContent(t, target, "mine")
			if _, err := os.Lstat(target + ".itsmydot.bak"); !os.IsNotExist(err) {
				t.Error("no backup should be made when cancelled")
			}
		})
	}
}

// 확인 질문 중에는 q, y, n처럼 선택과 상관없는 키를 무시하고 ctrl+c만 종료한다.
func TestConfirmIgnoresOtherKeys(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "space")

	for _, key := range []string{"q", "y", "n", "x"} {
		var cmd tea.Cmd
		m, cmd = press(t, m, key)
		if cmd != nil || m.confirming != "gitconfig" {
			t.Fatalf("%s: should be ignored while confirming", key)
		}
	}
	assertContent(t, target, "mine")

	v := viewText(m)
	for _, want := range []string{"이미 파일이 있습니다", "> 1. 백업 후 연결", "  2. 취소", "1/2 바로 선택"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c should quit while confirming")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c should quit while confirming")
	}
}

func TestToggleFetchError(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{}) // 리포에 파일이 없어서 404가 난다

	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)
	if m.items[0].state != errored || m.items[0].err == nil {
		t.Fatalf("state = %d err = %v, want errored", m.items[0].state, m.items[0].err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target should be untouched, got err %v", err)
	}
	if v := viewText(m); !strings.Contains(v, "not found") {
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

// 백업이 없을 때 1번을 고르면 연결만 끊고 지금 설정은 일반 파일로 남긴다.
func TestUnlinkWithoutBackupKeepsContent(t *testing.T) {
	m, target := linkedFixture(t)

	m, _ = press(t, m, "space")
	v := viewText(m)
	for _, want := range []string{
		"원래 파일(백업)을 찾을 수 없습니다",
		"1. 지금 설정은 남겨 두고 연결만 끊기 (추천)",
		"2. 연결을 끊고 파일도 지우기",
		"3. 취소",
		"1/2/3 바로 선택",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}

	m, _ = press(t, m, "1")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("target should be a regular file now")
	}
	assertContent(t, target, "from repo")
	if m.items[0].state != conflict {
		t.Errorf("state = %d, want conflict (a regular file is left)", m.items[0].state)
	}
}

// 백업이 없을 때 3번이나 esc를 누르면 아무것도 바꾸지 않는다.
func TestUnlinkWithoutBackupCancel(t *testing.T) {
	for _, key := range []string{"3", "esc"} {
		t.Run(key, func(t *testing.T) {
			m, target := linkedFixture(t)
			m, _ = press(t, m, "space")
			m, _ = press(t, m, key)
			if m.confirming != "" || !strings.Contains(m.status, "취소") {
				t.Fatalf("want cancelled, got confirming=%q status=%q", m.confirming, m.status)
			}
			info, err := os.Lstat(target)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("target should still be a symlink (err %v)", err)
			}
		})
	}
}

// 연결 확인 질문은 선택지가 2개라서 3은 무시한다.
func TestConfirmLinkIgnoresOutOfRange(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "space")
	m, cmd := press(t, m, "3")
	if cmd != nil || m.confirming == "" {
		t.Fatal("3 should be ignored for a 2-option prompt")
	}
}

// 확인 질문에서 방향키로 커서를 옮기고 enter로 고른다. 커서는 처음에 1번이고 범위를 벗어나지 않는다.
func TestConfirmArrowSelection(t *testing.T) {
	m, target := linkedFixture(t)
	m, _ = press(t, m, "space")
	if m.choice != 0 {
		t.Fatalf("initial choice = %d, want 0", m.choice)
	}

	m, _ = press(t, m, "up") // 맨 위라서 그대로
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down") // 맨 아래라서 그대로
	if m.choice != 2 {
		t.Fatalf("choice = %d, want 2", m.choice)
	}
	if v := viewText(m); !strings.Contains(v, "> 3. 취소") {
		t.Errorf("cursor should be on 3:\n%s", v)
	}

	m, _ = press(t, m, "k") // 2번 파일도 지우기로 올라간다
	m, _ = press(t, m, "enter")
	if m.confirming != "" {
		t.Fatal("enter should answer the prompt")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("option 2 should delete the target, got err %v", err)
	}
}

// space도 커서가 있는 선택지를 고른다.
func TestConfirmSpaceSelects(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "space")
	m, _ = press(t, m, "down") // 2. 취소
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirming != "" || !strings.Contains(m.status, "취소") {
		t.Fatalf("space on 2 should cancel: cmd=%v confirming=%q status=%q", cmd != nil, m.confirming, m.status)
	}
	assertContent(t, target, "mine")

	// 다시 열면 커서는 1번부터
	m, _ = press(t, m, "space")
	if m.choice != 0 {
		t.Errorf("choice = %d, want 0 when reopened", m.choice)
	}
}
