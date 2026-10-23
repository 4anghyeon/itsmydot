package tui

import (
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/diff"
)

// compareFixture 는 사본 내용이 local인 연결된 항목을 만들고 저장소 내용을 remote로 바꾼 뒤 메뉴에서 비교 보기를 고른다.
func compareFixture(t *testing.T, local, remote string) (Model, tea.Cmd, string) {
	t.Helper()
	files := map[string]string{"files/git/.gitconfig": local}
	m, target := toggleFixture(t, files)
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("setup: state = %d (err %v)", m.items[0].state, m.items[0].err)
	}

	files["files/git/.gitconfig"] = remote
	m, _ = press(t, m, "enter")
	m, cmd = press(t, m, "2")
	return m, cmd, target
}

// 메뉴에서 비교 보기를 고르면 저장소 최신본을 받는 동안 상태 줄에 스피너가 돌고, 다 받으면 diff 화면이 열린다.
// esc로 닫으면 목록으로 돌아가고 사본은 바뀌지 않는다.
func TestMenuCompareOpensDiff(t *testing.T) {
	m, cmd, target := compareFixture(t, "[user]\n\tname = me\n", "[user]\n\tname = you\n")
	if m.comparingName != "gitconfig" || !strings.Contains(viewText(m), "] gitconfig 저장소 최신본과 비교하는 중이에요...") {
		t.Fatalf("comparing = %q view:\n%s", m.comparingName, viewText(m))
	}

	m = runCmd(t, m, cmd)
	if m.diffScreen.name != "gitconfig" || m.comparingName != "" {
		t.Fatalf("diff = %q comparing = %q status = %q", m.diffScreen.name, m.comparingName, m.status)
	}
	v := viewText(m)
	for _, want := range []string{
		"gitconfig  ~/.gitconfig",
		"@@ -1,2 +1,2 @@",
		"1 1     [user]",
		"2    -      name = me",
		"  2  +      name = you",
		"esc 닫기",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "--- 사본") || strings.Contains(v, "\t") {
		t.Errorf("diff header and tabs should not be shown:\n%s", v)
	}
	if strings.ContainsAny(v, "█░") {
		t.Errorf("scrollbar should not be shown when everything fits:\n%s", v)
	}

	m, _ = press(t, m, "esc")
	if m.diffScreen.name != "" || !strings.Contains(viewText(m), "▌ [+] gitconfig") {
		t.Fatalf("esc should go back to the list:\n%s", viewText(m))
	}
	assertContent(t, target, "[user]\n\tname = me\n")
}

// 비교할 수 없거나 차이가 없으면 화면을 열지 않고 상태 메시지로 알린다.
func TestMenuCompareWithoutDiff(t *testing.T) {
	tests := []struct {
		name, remote, status string
	}{
		{"same", "same\n", "gitconfig 사본과 저장소 최신본이 같아요"},
		{"binary", "PNG\x00", "gitconfig 파일은 텍스트가 아니라서 비교할 수 없어요"},
		{"too large", strings.Repeat("a", 1<<20+1), "gitconfig 파일이 1MB보다 커서 비교할 수 없어요"},
	}
	for _, tt := range tests {
		m, cmd, _ := compareFixture(t, "same\n", tt.remote)
		m = runCmd(t, m, cmd)
		if m.diffScreen.name != "" || m.comparingName != "" || m.status != tt.status {
			t.Errorf("%s: diff = %q comparing = %q status = %q", tt.name, m.diffScreen.name, m.comparingName, m.status)
		}
	}
}

func TestMenuCompareFetchError(t *testing.T) {
	files := map[string]string{"files/git/.gitconfig": "v1"}
	m, _ := toggleFixture(t, files)
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)

	delete(files, "files/git/.gitconfig")
	m, _ = press(t, m, "enter")
	m, cmd = press(t, m, "2")
	m = runCmd(t, m, cmd)
	if m.diffScreen.name != "" || !strings.HasPrefix(m.status, "gitconfig 파일을 비교하지 못했어요 (저장소에 파일이 없어요)") {
		t.Fatalf("diff = %q status = %q", m.diffScreen.name, m.status)
	}
}

// 받는 사이에 다른 항목의 비교를 시작했으면 먼저 시작한 쪽의 결과는 버린다.
func TestCompareIgnoresStaleResult(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparingName = "zshrc"
	next, _ := m.Update(diffLoaded(t, "gitconfig", "a\n", "b\n"))
	m = next.(Model)
	if m.diffScreen.name != "" || m.comparingName != "zshrc" {
		t.Fatalf("diff = %q comparing = %q", m.diffScreen.name, m.comparingName)
	}
}

// 비교할 파일을 받는 동안에는 받는 중인 항목이 없어도 스피너가 계속 돈다.
func TestSpinnerTicksWhileComparing(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.spinner = spinner.New(spinner.WithSpinner(syncSpinner))
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Fatal("spinner should stop when nothing is loading")
	}
	m.comparingName = "zshrc"
	if _, cmd := m.Update(spinner.TickMsg{}); cmd == nil {
		t.Fatal("spinner should keep ticking while comparing")
	}
}

// diffLoaded 는 before와 after를 실제로 비교한 결과 메시지를 만든다.
func diffLoaded(t *testing.T, name, before, after string) diffLoadedMsg {
	t.Helper()
	hunks, err := diff.Compare([]byte(before), []byte(after))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	return diffLoadedMsg{name: name, hunks: hunks}
}

// conflictFixture 는 target 자리에 mine이라는 다른 파일이 있는 항목을 켜서 충돌 질문을 띄운다.
func conflictFixture(t *testing.T, mine, remote string) (Model, string) {
	t.Helper()
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": remote})
	if err := os.WriteFile(target, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "space")
	if m.confirmKind != confirmBackupLink || m.confirmingName != "gitconfig" {
		t.Fatalf("setup: kind = %d confirming = %q", m.confirmKind, m.confirmingName)
	}
	return m, target
}

// 충돌 질문에서 차이 보기를 고르면 질문을 닫지 않고 target 자리의 지금 파일과 저장소 파일을 비교한다.
// esc로 diff 화면을 닫으면 같은 질문으로 돌아오고, 거기서 백업 후 연결을 고를 수 있다.
func TestConflictPromptCompare(t *testing.T) {
	m, target := conflictFixture(t, "mine\n", "from repo\n")

	m, cmd := press(t, m, "2")
	if m.confirmingName != "gitconfig" || m.comparingName != "gitconfig" {
		t.Fatalf("prompt should stay while comparing: confirmingName = %q comparingName = %q", m.confirmingName, m.comparingName)
	}
	if v := viewText(m); !strings.Contains(v, "] gitconfig 저장소 파일과 비교하는 중이에요...") || !strings.Contains(v, "2. 차이 보기") {
		t.Errorf("progress should show under the prompt:\n%s", v)
	}

	m = runCmd(t, m, cmd)
	v := viewText(m)
	if m.diffScreen.name != "gitconfig" || !strings.Contains(v, "1    -  mine") || !strings.Contains(v, "  1  +  from repo") {
		t.Fatalf("diff screen should compare the current file:\n%s", v)
	}

	m, _ = press(t, m, "esc")
	if v := viewText(m); m.diffScreen.name != "" || m.confirmingName != "gitconfig" || !strings.Contains(v, "1. 백업 후 연결") {
		t.Fatalf("esc should go back to the prompt:\n%s", v)
	}
	assertContent(t, target, "mine\n")

	m, cmd = press(t, m, "1")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("state = %d, want synced after backup and link", m.items[0].state)
	}
	assertContent(t, target, "from repo\n")
	assertContent(t, target+".itsmydot.bak", "mine\n")
}

// 차이가 없으면 화면을 열지 않고 질문 아래에 알린다. 질문에 답하면 그 안내도 사라진다.
func TestConflictPromptCompareSame(t *testing.T) {
	m, _ := conflictFixture(t, "same\n", "same\n")
	m, cmd := press(t, m, "2")
	m = runCmd(t, m, cmd)
	if v := viewText(m); m.diffScreen.name != "" || m.confirmingName != "gitconfig" || !strings.Contains(v, "gitconfig 지금 파일과 저장소 파일이 같아요") {
		t.Fatalf("note should show under the prompt:\n%s", v)
	}

	m, _ = press(t, m, "esc")
	if m.confirmNote != "" || strings.Contains(viewText(m), "같아요") {
		t.Errorf("note should be cleared with the prompt:\n%s", viewText(m))
	}
}

// 비교 결과가 오기 전에 질문에 답했으면 결과를 버린다.
func TestConflictPromptCompareAfterAnswer(t *testing.T) {
	m, _ := conflictFixture(t, "mine\n", "from repo\n")
	m, cmd := press(t, m, "2")
	m, _ = press(t, m, "esc")
	if m.comparingName != "" {
		t.Fatalf("comparing = %q, want cleared with the prompt", m.comparingName)
	}
	m = runCmd(t, m, cmd)
	if m.diffScreen.name != "" || m.confirmNote != "" {
		t.Fatalf("late result should be dropped: diff = %q note = %q", m.diffScreen.name, m.confirmNote)
	}
}

// 하위 항목을 켜다가 그룹 파일만 걸렸으면 그룹 파일을 비교한다.
func TestConflictedIndexPrefersPromptItem(t *testing.T) {
	m := loadedModel(
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: conflict},
		item{entry: entry("env", "/home/me/.config/zsh/env.zsh"), parent: "zshrc"},
	)
	m.confirmingName, m.pendingNames = "env", []string{"zshrc", "env"}
	if i := m.conflictedIndex(); i != 0 {
		t.Errorf("conflictedIndex = %d, want the group file", i)
	}
	m.items[1].state = conflict
	if i := m.conflictedIndex(); i != 1 {
		t.Errorf("conflictedIndex = %d, want the prompt item", i)
	}
}
