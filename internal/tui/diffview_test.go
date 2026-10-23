package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// 터미널 높이에 맞춰 본문을 자르고 방향키와 pgdn, end로 스크롤한다. 맨 위에서 더 올라가지는 않는다.
func TestDiffScroll(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	// 빈 파일에 30줄을 추가하면 머리줄까지 31줄이다.
	var text strings.Builder
	for i := 1; i <= 30; i++ {
		_, _ = fmt.Fprintf(&text, "line %d\n", i)
	}
	m.comparingName = "zshrc"
	next, _ := m.Update(diffLoaded(t, "zshrc", "", text.String()))
	next, _ = next.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m = next.(Model)

	// 높이 18에서 캐릭터와 구분선, 도움말 10줄을 빼면 본문은 8줄이다.
	// 아래에 더 있다는 걸 알 수 있게 오른쪽에 스크롤 막대가 붙고 맨 위에 있으니 막대 위쪽이 채워져 있다.
	v := viewText(m)
	if !strings.Contains(v, "8/31줄") || !strings.Contains(v, "line 7") || strings.Contains(v, "line 8") {
		t.Fatalf("first page:\n%s", v)
	}
	if bar := scrollbarColumn(v); bar != "██░░░░░░" {
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
	if m.diffScreen.yOffset != 9 {
		t.Errorf("yOffset = %d, want 9 after pgdn", m.diffScreen.yOffset)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(Model)
	v = viewText(m)
	if m.diffScreen.yOffset != 23 || !strings.Contains(v, "31/31줄") || !strings.Contains(v, "line 30") {
		t.Errorf("yOffset = %d after end:\n%s", m.diffScreen.yOffset, v)
	}
	if bar := scrollbarColumn(v); bar != "░░░░░░██" {
		t.Errorf("scrollbar = %q at the bottom", bar)
	}
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
	m.comparingName = "zshrc"
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

// 지운 줄과 추가한 줄은 줄 끝까지 배경을 칠하고 줄 번호 칸과 바뀐 단어는 더 진하게 칠한다. @@ 줄은 회색 띠로 그린다.
func TestDiffRowColors(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparingName = "zshrc"
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

	// 배경을 칠한 줄은 내용이 짧아도 가장 긴 줄 끝까지 이어져야 한다.
	text := viewText(m)
	if !strings.Contains(text, " 2    -  export EDITOR=vim \n") || !strings.Contains(text, "   2  +  export EDITOR=nvim\n") {
		t.Errorf("tinted rows should fill the box:\n%s", text)
	}
}

// 파일 끝 줄바꿈이 없는 줄 바로 아래에 git과 같은 안내 줄을 붙인다.
func TestDiffNoNewlineRow(t *testing.T) {
	m := loadedModel(item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced})
	m.comparingName = "zshrc"
	next, _ := m.Update(diffLoaded(t, "zshrc", "a\nlast", "a\nlast\n"))
	v := viewText(next.(Model))
	lines := strings.Split(v, "\n")
	i := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "2    -  last") })
	if i < 0 || i+2 >= len(lines) ||
		!strings.Contains(lines[i+1], "\\ No newline at end of file") || !strings.Contains(lines[i+2], "  2  +  last") {
		t.Errorf("no newline row should follow the removed line:\n%s", v)
	}
}
