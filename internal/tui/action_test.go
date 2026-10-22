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

	// 켜기 전에 먼저 묻는다. 답하기 전에는 디스크를 건드리지 않는다.
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirming != "gitconfig" || m.confirmKind != confirmTurnOn {
		t.Fatalf("want turn on prompt, got cmd=%v confirming=%q kind=%d", cmd != nil, m.confirming, m.confirmKind)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target should be untouched before answering, got err %v", err)
	}

	// 1번을 고르면 syncing이 되고 작업이 끝나면 synced가 된다.
	m, cmd = press(t, m, "1")
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
	if !strings.Contains(m.status, "에 남아 있어요") || m.statusTone != statusUnlinked {
		t.Errorf("status = %q tone = %d, want where the content remains with the unlinked marker", m.status, m.statusTone)
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

	// 백업이 있으면 끄기 전에 묻고, 답하면 원래 파일을 되돌린다.
	m, _ = press(t, m, "space")
	if v := viewText(m); !strings.Contains(v, "gitconfig  ~/.gitconfig 연결을 끊을까요?") || !strings.Contains(v, "> 1. 연결 끊기") {
		t.Errorf("view should ask before turning off:\n%s", v)
	}
	assertSymlink(t, target, true)
	m, _ = press(t, m, "1")
	if m.items[0].state != conflict {
		t.Errorf("state = %d, want conflict (original file restored)", m.items[0].state)
	}
	assertContent(t, target, "mine")
}

func TestConfirmCancel(t *testing.T) {
	for _, key := range []string{"3", "esc"} {
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
			if m.status != "" {
				t.Errorf("status = %q", m.status)
			}
			assertContent(t, target, "mine")
			if _, err := os.Lstat(target + ".itsmydot.bak"); !os.IsNotExist(err) {
				t.Error("no backup should be made when cancelled")
			}
		})
	}
}

// 토글 확인 질문에서 2번이나 esc를 누르면 켜지도 끄지도 않는다.
func TestToggleConfirmCancel(t *testing.T) {
	for _, key := range []string{"2", "esc"} {
		t.Run("on/"+key, func(t *testing.T) {
			m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
			m, _ = press(t, m, "space")
			m, cmd := press(t, m, key)
			if cmd != nil || m.confirming != "" || m.items[0].state != notSynced || m.status != "" {
				t.Fatalf("cmd=%v confirming=%q state=%d status=%q", cmd != nil, m.confirming, m.items[0].state, m.status)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Errorf("target should be untouched, got err %v", err)
			}
		})
		t.Run("off/"+key, func(t *testing.T) {
			m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
			if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
				t.Fatal(err)
			}
			m, _ = press(t, m, "space")
			m, cmd := press(t, m, "1")
			m = runCmd(t, m, cmd)

			m, _ = press(t, m, "space")
			if m.confirmKind != confirmTurnOff {
				t.Fatalf("kind = %d, want confirmTurnOff", m.confirmKind)
			}
			m, _ = press(t, m, key)
			if m.confirming != "" || m.items[0].state != synced || m.status != "" {
				t.Fatalf("confirming=%q state=%d status=%q", m.confirming, m.items[0].state, m.status)
			}
			assertSymlink(t, target, true)
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
	for _, want := range []string{"이미 파일이 있어요", "> 1. 백업 후 연결", "  2. 차이 보기", "  3. 취소", "1/2/3 바로 선택"} {
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
	m, target := toggleFixture(t, map[string]string{}) // 저장소에 파일이 없어서 404가 난다

	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)
	if m.items[0].state != errored || m.items[0].err == nil {
		t.Fatalf("state = %d err = %v, want errored", m.items[0].state, m.items[0].err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target should be untouched, got err %v", err)
	}
	if v := viewText(m); !strings.Contains(v, "(저장소에 파일이 없어요)") {
		t.Errorf("view should show the error:\n%s", v)
	}
}

// Update가 돌려준 새 Model을 바꿔도, 이전 Model의 항목은 그대로여야 한다.
func TestToggleDoesNotMutatePreviousModel(t *testing.T) {
	before, _ := toggleFixture(t, map[string]string{"files/git/.gitconfig": "x"})
	_, _ = confirmToggle(t, before)
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
		"백업이 없어서 되돌릴 원본 파일이 없어요",
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
			if m.confirming != "" || m.status != "" {
				t.Fatalf("want cancelled, got confirming=%q status=%q", m.confirming, m.status)
			}
			info, err := os.Lstat(target)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("target should still be a symlink (err %v)", err)
			}
		})
	}
}

// 연결 확인 질문은 선택지가 3개라서 4는 무시한다.
func TestConfirmLinkIgnoresOutOfRange(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	if err := os.WriteFile(target, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "space")
	m, cmd := press(t, m, "4")
	if cmd != nil || m.confirming == "" {
		t.Fatal("4 should be ignored for a 3-option prompt")
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
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down") // 3. 취소
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirming != "" || m.status != "" {
		t.Fatalf("space on 3 should cancel: cmd=%v confirming=%q status=%q", cmd != nil, m.confirming, m.status)
	}
	assertContent(t, target, "mine")

	// 다시 열면 커서는 1번부터
	m, _ = press(t, m, "space")
	if m.choice != 0 {
		t.Errorf("choice = %d, want 0 when reopened", m.choice)
	}
}

// 연결된 항목에서 enter를 누르면 메뉴가 뜨고, 1번을 고르면 연결은 그대로 두고 로컬 사본만 새로 받는다.
func TestMenuUpdateCopy(t *testing.T) {
	files := map[string]string{"files/git/.gitconfig": "v1"}
	m, target := toggleFixture(t, files)
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)

	files["files/git/.gitconfig"] = "v2"
	m, _ = press(t, m, "enter")
	v := viewText(m)
	for _, want := range []string{"> 1. 최신으로 받기", "  2. 차이 보기", "  3. 연결 끊기", "  4. 닫기"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}

	m, cmd = press(t, m, "1")
	if m.items[0].state != syncing {
		t.Fatalf("state = %d, want syncing", m.items[0].state)
	}
	if m.status != "gitconfig 최신본을 받는 중이에요..." {
		t.Errorf("status = %q", m.status)
	}
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced || m.status != "gitconfig 사본을 최신으로 바꿨어요" {
		t.Fatalf("state = %d status = %q", m.items[0].state, m.status)
	}
	assertContent(t, target, "v2")
	if _, err := os.Lstat(target + ".itsmydot.bak"); !os.IsNotExist(err) {
		t.Error("updating the copy should not make a backup")
	}
}

// 받기에 실패해도 연결과 이전 사본은 그대로라 연결된 상태로 남는다.
func TestMenuUpdateCopyFetchError(t *testing.T) {
	files := map[string]string{"files/git/.gitconfig": "v1"}
	m, target := toggleFixture(t, files)
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)

	delete(files, "files/git/.gitconfig")
	m, _ = press(t, m, "enter")
	m, cmd = press(t, m, "1")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced || !strings.Contains(m.status, "최신본을 받지 못해서 이전 사본을 그대로 써요") {
		t.Fatalf("state = %d status = %q", m.items[0].state, m.status)
	}
	assertContent(t, target, "v1")
}

// 메뉴를 연 뒤 연결이 사라졌으면 받지 않고 상태만 바로잡는다.
func TestMenuUpdateCopyWhenNoLongerLinked(t *testing.T) {
	m, target := linkedFixture(t)
	m, _ = press(t, m, "enter")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	m, cmd := press(t, m, "1")
	if cmd != nil || m.items[0].state != notSynced || !strings.Contains(m.status, "연결이 풀려 있어서 받지 않았어요") {
		t.Fatalf("cmd=%v state=%d status=%q", cmd != nil, m.items[0].state, m.status)
	}
}

// 연결되지 않은 항목의 메뉴에는 켜기만 있고, 고르면 space와 똑같이 한 번 더 묻는다.
func TestMenuTurnOn(t *testing.T) {
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	m, _ = press(t, m, "enter")
	v := viewText(m)
	for _, want := range []string{"gitconfig  ~/.gitconfig", "> 1. 연결", "  2. 닫기"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	m, cmd := press(t, m, "1")
	if cmd != nil || m.confirmKind != confirmTurnOn {
		t.Fatalf("want turn on prompt, got cmd=%v kind=%d", cmd != nil, m.confirmKind)
	}
	m, cmd = press(t, m, "1")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("state = %d, want synced", m.items[0].state)
	}
	assertContent(t, target, "from repo")
}

// 메뉴에서 끄기를 고르면 space와 같은 흐름을 탄다. 백업이 없으니 이어서 해제 방법을 묻는다.
func TestMenuTurnOffAsksLikeSpace(t *testing.T) {
	m, _ := linkedFixture(t)
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "3")
	if m.confirmKind != confirmUnlink || m.confirming != "gitconfig" {
		t.Fatalf("want the unlink prompt, got kind=%d confirming=%q", m.confirmKind, m.confirming)
	}
}

// esc로 메뉴를 닫으면 아무것도 하지 않고 취소 메시지도 남기지 않는다. 받는 중에는 메뉴를 열지 않는다.
func TestMenuCancelAndSyncing(t *testing.T) {
	m, _ := toggleFixture(t, map[string]string{"files/git/.gitconfig": "x"})
	m, _ = press(t, m, "enter")
	m, cmd := press(t, m, "esc")
	if cmd != nil || m.confirming != "" || m.status != "" {
		t.Fatalf("cmd=%v confirming=%q status=%q", cmd != nil, m.confirming, m.status)
	}

	m, _ = confirmToggle(t, m)
	m, _ = press(t, m, "enter")
	if m.confirming != "" {
		t.Error("menu should not open while syncing")
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

	// .a와 .b는 연결돼 있고, .c는 예전에 받은 사본만 남아 있다.
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

// 최신으로 받으면 업데이트 표시가 사라진다.
func TestUpdateCopyClearsOutdated(t *testing.T) {
	m, _ := linkedFixture(t)
	m.items[0].outdated = true
	if v := viewText(m); !strings.Contains(v, "~/.gitconfig  ^ 업데이트 있음") {
		t.Fatalf("view should show the mark:\n%s", v)
	}

	m, _ = press(t, m, "enter")
	m, cmd := press(t, m, "1")
	m = runCmd(t, m, cmd)
	if m.items[0].outdated || strings.Contains(viewText(m), "^ 업데이트") {
		t.Errorf("mark should disappear after updating the copy:\n%s", viewText(m))
	}
}
