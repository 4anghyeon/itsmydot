package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// groupFixture 는 그룹 zshrc와 하위 항목 env, git으로 Model을 만든다. 가짜 저장소에는 세 파일이 모두 있다.
func groupFixture(t *testing.T) (m Model, home string) {
	t.Helper()
	home = t.TempDir()
	copies := localcopy.New(t.TempDir())
	client := fakeRepo(t, map[string]string{
		"files/zsh/.zshrc":     "for f in ~/.config/zsh/*.zsh; do source $f; done\n",
		"files/zsh/00-env.zsh": "export EDITOR=vim\n",
		"files/zsh/20-git.zsh": "alias g=git\n",
	})

	entries := []struct {
		entry  manifest.Entry
		parent string
	}{
		{manifest.Entry{Name: "zshrc", Source: "files/zsh/.zshrc", Target: filepath.Join(home, ".zshrc")}, ""},
		{manifest.Entry{Name: "env", Source: "files/zsh/00-env.zsh", Target: filepath.Join(home, ".config/zsh/00-env.zsh")}, "zshrc"},
		{manifest.Entry{Name: "git", Source: "files/zsh/20-git.zsh", Target: filepath.Join(home, ".config/zsh/20-git.zsh")}, "zshrc"},
	}
	items := make([]item, len(entries))
	for i, e := range entries {
		copyPath, err := copies.Path(e.entry.Source)
		if err != nil {
			t.Fatal(err)
		}
		items[i] = item{entry: e.entry, parent: e.parent, copyPath: copyPath}
	}

	m = New(client, copies, "")
	m.home = home
	next, _ := m.Update(entriesLoadedMsg{items: items})
	return next.(Model), home
}

// turnOnEnv 는 하위 항목 env를 켜서 그룹 zshrc까지 연결된 상태를 만든다.
func turnOnEnv(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, "down")
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced || m.items[1].state != synced {
		t.Fatalf("setup: zshrc=%d env=%d, want both synced", m.items[0].state, m.items[1].state)
	}
	return m
}

func assertSymlink(t *testing.T, path string, want bool) {
	t.Helper()
	info, err := os.Lstat(path)
	isLink := err == nil && info.Mode()&os.ModeSymlink != 0
	if isLink != want {
		t.Errorf("%s: symlink = %v, want %v (err %v)", path, isLink, want, err)
	}
}

func TestGroupView(t *testing.T) {
	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced},
		item{entry: entry("env", "/home/me/.config/zsh/00-env.zsh"), parent: "zshrc", state: synced},
		item{entry: entry("android", "/home/me/.config/zsh/40-android.zsh"), parent: "zshrc"},
	)
	v := viewText(m)
	for _, want := range []string{
		"▌   [+] gitconfig  ~/.gitconfig",
		"  v [+] zshrc      ~/.zshrc  1/2",
		"  ├── [+] env      ~/.config/zsh/00-env.zsh",
		"  └── [ ] android  ~/.config/zsh/40-android.zsh",
		"연결됨 3/4",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

// 왼쪽 방향키는 그룹을 접고 커서를 그룹 줄로 옮긴다. 접힌 하위 항목은 커서가 건너뛰고 오른쪽 방향키로 다시 펼친다.
func TestCollapseAndExpand(t *testing.T) {
	m := loadedModel(
		item{entry: entry("zshrc", "/home/me/.zshrc")},
		item{entry: entry("env", "/home/me/.config/zsh/00-env.zsh"), parent: "zshrc"},
		item{entry: entry("gitconfig", "/home/me/.gitconfig")},
	)
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "left")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (the group line)", m.cursor)
	}
	v := viewText(m)
	if strings.Contains(v, "env") || !strings.Contains(v, "▌ > [ ] zshrc") {
		t.Errorf("collapsed group should hide its children:\n%s", v)
	}

	m, _ = press(t, m, "down")
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (skip the hidden child)", m.cursor)
	}
	m, _ = press(t, m, "up")
	m, _ = press(t, m, "right")
	if v := viewText(m); !strings.Contains(v, "env") || !strings.Contains(v, "▌ v [ ] zshrc") {
		t.Errorf("expanded group should show its children:\n%s", v)
	}

	// 그룹이 아닌 줄에서는 아무 일도 없다.
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "left")
	if m.cursor != 2 || len(m.collapsed) != 1 || m.collapsed["zshrc"] {
		t.Errorf("left on a plain item: cursor=%d collapsed=%v", m.cursor, m.collapsed)
	}
}

// 그룹 파일이 꺼져 있으면 하위 항목을 켤 때 그룹 파일도 함께 받는다.
func TestChildTurnsOnGroup(t *testing.T) {
	m, home := groupFixture(t)

	m, _ = press(t, m, "down")
	m, _ = press(t, m, "space")
	v := viewText(m)
	for _, want := range []string{"env  ~/.config/zsh/00-env.zsh 파일을 연결할까요? 그룹 파일 zshrc  ~/.zshrc 도 함께 연결해요.", "> 1. 연결", "  2. 취소"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	m, cmd := press(t, m, "1")
	if m.items[0].state != syncing || m.items[1].state != syncing {
		t.Fatalf("zshrc=%d env=%d, want both syncing", m.items[0].state, m.items[1].state)
	}
	if m.items[2].state != notSynced {
		t.Errorf("git should stay off, got %d", m.items[2].state)
	}
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced || m.items[1].state != synced {
		t.Fatalf("zshrc=%d env=%d, want both synced", m.items[0].state, m.items[1].state)
	}
	assertContent(t, filepath.Join(home, ".config/zsh/00-env.zsh"), "export EDITOR=vim\n")

	// 그룹 파일이 이미 켜져 있으면 하위 항목만 받는다.
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "space")
	if v := viewText(m); strings.Contains(v, "함께 연결해요") {
		t.Errorf("group is already on, prompt should not mention it:\n%s", v)
	}
	m, cmd = press(t, m, "1")
	if m.items[0].state != synced || m.items[2].state != syncing {
		t.Fatalf("zshrc=%d git=%d, want synced and syncing", m.items[0].state, m.items[2].state)
	}
	m = runCmd(t, m, cmd)
	if m.items[2].state != synced {
		t.Errorf("git = %d, want synced", m.items[2].state)
	}
}

// 그룹 파일 자리에 다른 파일이 있으면 하위 항목을 켤 때 한 번만 묻고, 답하면 둘 다 연결한다.
func TestChildTurnOnAsksOnceForGroupConflict(t *testing.T) {
	m, home := groupFixture(t)
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirmKind != confirmBackupLink {
		t.Fatalf("want a link prompt, got cmd=%v kind=%d", cmd != nil, m.confirmKind)
	}
	v := viewText(m)
	for _, want := range []string{"그룹 파일 zshrc 도 함께 연결해요. ~/.zshrc 에 이미 파일이 있어요."} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}

	m, cmd = press(t, m, "1")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced || m.items[1].state != synced {
		t.Fatalf("zshrc=%d env=%d, want both synced", m.items[0].state, m.items[1].state)
	}
	assertContent(t, zshrc+".itsmydot.bak", "mine")
	if want := "zshrc 파일을 연결했어요. 기존 파일은 ~/.zshrc.itsmydot.bak 에 백업했어요 / env 파일을 연결했어요"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
}

// 백업이 없는 그룹 파일을 끄면 하위 항목도 함께 꺼진다는 걸 같은 질문에서 알린다.
func TestGroupUnlinkWithoutBackupTurnsOffChildren(t *testing.T) {
	m, home := groupFixture(t)
	m = turnOnEnv(t, m)

	m, _ = press(t, m, "up")
	m, _ = press(t, m, "space")
	if m.confirmKind != confirmUnlinkWithoutBackup {
		t.Fatalf("kind = %d, want confirmUnlinkWithoutBackup", m.confirmKind)
	}
	if v := viewText(m); !strings.Contains(v, "하위 항목 1개(env)도 함께 연결이 끊겨요.") {
		t.Errorf("prompt should mention the children:\n%s", v)
	}

	m, _ = press(t, m, "2")
	if m.items[0].state != notSynced || m.items[1].state != notSynced {
		t.Fatalf("zshrc=%d env=%d, want both off", m.items[0].state, m.items[1].state)
	}
	if !strings.Contains(m.status, "하위 항목 1개도 함께 연결을 끊었어요") {
		t.Errorf("status = %q", m.status)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config/zsh/00-env.zsh")); !os.IsNotExist(err) {
		t.Errorf("child symlink should be removed, got err %v", err)
	}
}

// 백업이 있는 그룹 파일을 끌 때도 하위 항목이 켜져 있으면 먼저 묻는다.
func TestGroupUnlinkWithBackupAsksFirst(t *testing.T) {
	m, home := groupFixture(t)
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "space")
	m, cmd := press(t, m, "1")
	m = runCmd(t, m, cmd)
	m, _ = press(t, m, "up")

	m, cmd = press(t, m, "space")
	if cmd != nil || m.confirmKind != confirmGroupUnlink {
		t.Fatalf("want a group unlink prompt, got cmd=%v kind=%d", cmd != nil, m.confirmKind)
	}
	if v := viewText(m); !strings.Contains(v, "zshrc 연결을 끊으면 하위 항목 1개(env)도 함께 연결이 끊겨요. 계속할까요?") {
		t.Errorf("view:\n%s", v)
	}

	// 취소하면 아무것도 바뀌지 않는다.
	m, _ = press(t, m, "esc")
	assertSymlink(t, zshrc, true)
	assertSymlink(t, filepath.Join(home, ".config/zsh/00-env.zsh"), true)

	m, _ = press(t, m, "space")
	m, _ = press(t, m, "1")
	if m.items[0].state != conflict || m.items[1].state != notSynced {
		t.Fatalf("zshrc=%d env=%d, want restored backup and child off", m.items[0].state, m.items[1].state)
	}
	assertContent(t, zshrc, "mine")
}

// 하위 항목은 그룹 바로 뒤에 이어 붙고 그룹 이름을 parent로 가진다.
func TestLoadEntriesWithChildren(t *testing.T) {
	home := t.TempDir()
	client := fakeRepo(t, map[string]string{"manifest.yaml": `entries:
  - {name: gitconfig, source: files/git/.gitconfig, target: ` + filepath.Join(home, ".gitconfig") + `}
  - name: zshrc
    source: files/zsh/.zshrc
    target: ` + filepath.Join(home, ".zshrc") + `
    children:
      - {name: env, source: files/zsh/00-env.zsh, target: ` + filepath.Join(home, "env.zsh") + `}
`})

	msg := loadEntries(context.Background(), client, localcopy.New(t.TempDir()))
	if msg.err != nil {
		t.Fatalf("loadEntries: %v", msg.err)
	}
	var got []string
	for _, it := range msg.items {
		got = append(got, it.entry.Name+"<"+it.parent+">")
	}
	if want := "gitconfig<> zshrc<> env<zshrc>"; strings.Join(got, " ") != want {
		t.Errorf("items = %s, want %s", strings.Join(got, " "), want)
	}
}

// 접힌 그룹은 하위 항목의 업데이트를 그룹 줄에 모아 보여 준다. 연결되지 않은 항목에는 표시하지 않는다.
func TestOutdatedMarkInGroup(t *testing.T) {
	m := loadedModel(
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced, outdated: true},
		item{entry: entry("env", "/home/me/.config/zsh/env.zsh"), parent: "zshrc", state: synced, outdated: true},
		item{entry: entry("git", "/home/me/.config/zsh/git.zsh"), parent: "zshrc", state: synced, outdated: true},
		item{entry: entry("android", "/home/me/.config/zsh/android.zsh"), parent: "zshrc", outdated: true},
	)
	v := viewText(m)
	for _, want := range []string{
		"~/.zshrc  2/3  ^ 업데이트 있음",
		"~/.config/zsh/env.zsh  ^ 업데이트 있음",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "android.zsh  ^") || strings.Contains(v, "하위 2개") {
		t.Errorf("unexpected mark:\n%s", v)
	}

	m, _ = press(t, m, "left")
	if v := viewText(m); !strings.Contains(v, "~/.zshrc  2/3  ^ 업데이트 있음  ^ 하위 2개 업데이트 있음") {
		t.Errorf("collapsed group should collect the children:\n%s", v)
	}
}

// 그룹 줄 끝의 하위 항목 수는 경로와 구분되게 따로 칠한다.
func TestGroupCountColor(t *testing.T) {
	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig")},
		item{entry: entry("zshrc", "/home/me/.zshrc")},
		item{entry: entry("env", "/home/me/.config/zsh/env.zsh"), parent: "zshrc", state: synced},
	)
	if want := newStyles(true).count.Render("1/1"); !strings.Contains(m.View().Content, want) {
		t.Errorf("group count should be drawn in the count color %q", want)
	}
}
