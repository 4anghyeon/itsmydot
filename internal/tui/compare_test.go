package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/diff"
)

// compareFixture 는 사본 내용이 local인 연결된 항목을 만들고 리포 내용을 remote로 바꾼 뒤 메뉴에서 비교 보기를 고른다.
func compareFixture(t *testing.T, local, remote string) (Model, tea.Cmd, string) {
	t.Helper()
	files := map[string]string{"files/git/.gitconfig": local}
	m, target := toggleFixture(t, files)
	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)
	if m.items[0].state != synced {
		t.Fatalf("setup: state = %d (err %v)", m.items[0].state, m.items[0].err)
	}

	files["files/git/.gitconfig"] = remote
	m, _ = press(t, m, "enter")
	m, cmd = press(t, m, "2")
	return m, cmd, target
}

// 메뉴에서 비교 보기를 고르면 리포 최신본을 받는 동안 상태 줄에 스피너가 돌고, 다 받으면 diff 화면이 열린다.
// esc로 닫으면 목록으로 돌아가고 사본은 바뀌지 않는다.
func TestMenuCompareOpensDiff(t *testing.T) {
	m, cmd, target := compareFixture(t, "[user]\n\tname = me\n", "[user]\n\tname = you\n")
	if m.comparing != "gitconfig" || !strings.Contains(viewText(m), "] gitconfig 리포 최신본과 비교하는 중이에요...") {
		t.Fatalf("comparing = %q view:\n%s", m.comparing, viewText(m))
	}

	m = runCmd(t, m, cmd)
	if m.diffScreen.name != "gitconfig" || m.comparing != "" {
		t.Fatalf("diff = %q comparing = %q status = %q", m.diffScreen.name, m.comparing, m.status)
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
	if m.diffScreen.name != "" || !strings.Contains(viewText(m), "> [+] gitconfig") {
		t.Fatalf("esc should go back to the list:\n%s", viewText(m))
	}
	assertContent(t, target, "[user]\n\tname = me\n")
}

// 비교할 수 없거나 차이가 없으면 화면을 열지 않고 상태 메시지로 알린다.
func TestMenuCompareWithoutDiff(t *testing.T) {
	tests := []struct {
		name, remote, status string
	}{
		{"same", "same\n", "gitconfig 사본과 리포 최신본이 같아요"},
		{"binary", "PNG\x00", "gitconfig 파일은 텍스트가 아니라서 비교할 수 없어요"},
		{"too large", strings.Repeat("a", 1<<20+1), "gitconfig 파일이 1MB보다 커서 비교할 수 없어요"},
	}
	for _, tt := range tests {
		m, cmd, _ := compareFixture(t, "same\n", tt.remote)
		m = runCmd(t, m, cmd)
		if m.diffScreen.name != "" || m.comparing != "" || m.status != tt.status {
			t.Errorf("%s: diff = %q comparing = %q status = %q", tt.name, m.diffScreen.name, m.comparing, m.status)
		}
	}
}

func TestMenuCompareFetchError(t *testing.T) {
	files := map[string]string{"files/git/.gitconfig": "v1"}
	m, _ := toggleFixture(t, files)
	m, cmd := press(t, m, "space")
	m = runCmd(t, m, cmd)

	delete(files, "files/git/.gitconfig")
	m, _ = press(t, m, "enter")
	m, cmd = press(t, m, "2")
	m = runCmd(t, m, cmd)
	if m.diffScreen.name != "" || !strings.HasPrefix(m.status, "gitconfig 비교하지 못했어요 (") {
		t.Fatalf("diff = %q status = %q", m.diffScreen.name, m.status)
	}
}

// 받는 사이에 다른 항목의 비교를 시작했으면 먼저 시작한 쪽의 결과는 버린다.
func TestCompareIgnoresStaleResult(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparing = "zshrc"
	next, _ := m.Update(diffLoaded(t, "gitconfig", "a\n", "b\n"))
	m = next.(Model)
	if m.diffScreen.name != "" || m.comparing != "zshrc" {
		t.Fatalf("diff = %q comparing = %q", m.diffScreen.name, m.comparing)
	}
}

// 비교할 파일을 받는 동안에는 받는 중인 항목이 없어도 스피너가 계속 돈다.
func TestSpinnerTicksWhileComparing(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.spinner = spinner.New(spinner.WithSpinner(syncSpinner))
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Fatal("spinner should stop when nothing is loading")
	}
	m.comparing = "zshrc"
	if _, cmd := m.Update(spinner.TickMsg{}); cmd == nil {
		t.Fatal("spinner should keep ticking while comparing")
	}
}

// 터미널 높이에 맞춰 본문을 자르고 방향키와 pgdn, end로 스크롤한다. 맨 위에서 더 올라가지는 않는다.
func TestDiffScroll(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	// 빈 파일에 30줄을 추가하면 머리줄까지 31줄이다.
	var text strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&text, "line %d\n", i)
	}
	m.comparing = "zshrc"
	next, _ := m.Update(diffLoaded(t, "zshrc", "", text.String()))
	next, _ = next.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m = next.(Model)

	// 높이 18에서 테두리와 제목, 도움말 8줄을 빼면 본문은 10줄이다.
	// 아래에 더 있다는 걸 알 수 있게 오른쪽에 스크롤 막대가 붙고 맨 위에 있으니 막대 위쪽이 채워져 있다.
	v := viewText(m)
	if !strings.Contains(v, "10/31줄") || !strings.Contains(v, "line 9") || strings.Contains(v, "line 10") {
		t.Fatalf("first page:\n%s", v)
	}
	if bar := scrollbarColumn(v); bar != "███░░░░░░░" {
		t.Errorf("scrollbar = %q at the top", bar)
	}

	m, _ = press(t, m, "up")
	if m.diffScreen.yOffset != 0 {
		t.Errorf("yOffset = %d, want 0 at the top", m.diffScreen.yOffset)
	}
	m, _ = press(t, m, "down")
	if m.diffScreen.yOffset != 1 {
		t.Errorf("yOffset = %d, want 1", m.diffScreen.yOffset)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = next.(Model)
	if m.diffScreen.yOffset != 11 {
		t.Errorf("yOffset = %d, want 11 after pgdn", m.diffScreen.yOffset)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(Model)
	v = viewText(m)
	if m.diffScreen.yOffset != 21 || !strings.Contains(v, "31/31줄") || !strings.Contains(v, "line 30") {
		t.Errorf("yOffset = %d after end:\n%s", m.diffScreen.yOffset, v)
	}
	if bar := scrollbarColumn(v); bar != "░░░░░░░███" {
		t.Errorf("scrollbar = %q at the bottom", bar)
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

// scrollbarColumn 은 화면에서 스크롤 막대 글자만 위에서부터 이어 붙인다.
func scrollbarColumn(view string) string {
	var bar strings.Builder
	for _, line := range strings.Split(view, "\n") {
		for _, r := range line {
			if r == '█' || r == '░' {
				bar.WriteRune(r)
			}
		}
	}
	return bar.String()
}

// 좌우 방향키로 긴 줄을 밀면 내용만 밀리고 줄 번호는 그 자리에 남는다. 끝을 넘어서 밀리지는 않는다.
func TestDiffHorizontalScroll(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparing = "zshrc"
	long := "export PATH=" + strings.Repeat("/usr/local/bin:", 4) + "END"
	next, _ := m.Update(diffLoaded(t, "zshrc", "short\n", long+"\n"))
	next, _ = next.(Model).Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	m = next.(Model)

	if v := viewText(m); !strings.Contains(v, "  1  +  export PATH=/usr") || strings.Contains(v, "END") {
		t.Fatalf("first view:\n%s", v)
	}
	m, _ = press(t, m, "right")
	if v := viewText(m); m.diffScreen.xOffset != 8 || !strings.Contains(v, "  1  +  ATH=/usr") {
		t.Errorf("xOffset = %d after right:\n%s", m.diffScreen.xOffset, v)
	}
	for range 10 {
		m, _ = press(t, m, "right")
	}
	if v := viewText(m); !strings.Contains(v, "END") || !strings.Contains(v, "1    -  ") {
		t.Errorf("xOffset = %d at the end:\n%s", m.diffScreen.xOffset, v)
	}
}

// 지운 줄과 추가한 줄은 줄 끝까지 배경을 칠하고, 줄 번호 칸과 바뀐 단어는 더 진하게 칠한다. @@ 줄은 회색 띠로 그린다.
func TestDiffRowColors(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparing = "zshrc"
	next, _ := m.Update(diffLoaded(t, "zshrc", "same\nexport EDITOR=vim\n", "same\nexport EDITOR=nvim\n"))
	m = next.(Model)

	background := func(hex string) string {
		var r, g, b int
		_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
		return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	}
	v := m.View().Content
	for name, c := range map[string]string{
		"removed line":   background("#5F0000"),
		"removed number": background("#870000"),
		"removed word":   background("#AF0000"),
		"added line":     background("#005F00"),
		"added number":   background("#008700"),
		"added word":     background("#00AF00"),
		"hunk":           background("#303030"),
	} {
		if !strings.Contains(v, c) {
			t.Errorf("%s background %s missing", name, c)
		}
	}

	// 배경을 칠한 줄은 내용이 짧아도 상자 안쪽 끝까지 이어져야 한다.
	text := viewText(m)
	if !strings.Contains(text, "│ 2    -  export EDITOR=vim  │") || !strings.Contains(text, "│   2  +  export EDITOR=nvim │") {
		t.Errorf("tinted rows should fill the box:\n%s", text)
	}
}

// 파일 끝 줄바꿈이 없는 줄 바로 아래에 git과 같은 안내 줄을 붙인다.
func TestDiffNoNewlineRow(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparing = "zshrc"
	next, _ := m.Update(diffLoaded(t, "zshrc", "a\nlast", "a\nlast\n"))
	v := viewText(next.(Model))
	lines := strings.Split(v, "\n")
	i := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "2    -  last") })
	if i < 0 || i+2 >= len(lines) ||
		!strings.Contains(lines[i+1], "\\ No newline at end of file") || !strings.Contains(lines[i+2], "  2  +  last") {
		t.Errorf("no newline row should follow the removed line:\n%s", v)
	}
}
