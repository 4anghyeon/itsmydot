package tui

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderGauge(t *testing.T) {
	tests := []struct {
		ratio  float64
		filled int
	}{
		{0, 0},
		{0.5, gaugeWidth / 2},
		{1, gaugeWidth},
		{0.01, 1},         // 하나라도 연결돼 있으면 최소 한 칸
		{1.2, gaugeWidth}, // 범위를 넘어도 막대 길이는 그대로
		{-0.1, 0},
	}
	for _, tt := range tests {
		g := ansi.Strip(newStyles(true).renderGauge(tt.ratio))
		if got := strings.Count(g, "█"); got != tt.filled {
			t.Errorf("gauge(%v) filled = %d, want %d", tt.ratio, got, tt.filled)
		}
		if w := lipgloss.Width(g); w != gaugeWidth {
			t.Errorf("gauge(%v) width = %d, want %d", tt.ratio, w, gaugeWidth)
		}
	}
}

func TestHeaderShowsSyncedCount(t *testing.T) {
	m := loadedModel(
		item{entry: entry("a", "/home/me/.a"), state: synced},
		item{entry: entry("b", "/home/me/.b"), state: conflict},
		item{entry: entry("c", "/home/me/.c")},
	)
	if v := viewText(m); !strings.Contains(v, "연결됨 1/3") {
		t.Errorf("header missing synced count:\n%s", v)
	}
}

// 터미널이 좁으면 화면을 터미널 폭에 맞추고 긴 에러는 줄바꿈한다.
func TestViewFitsTerminalWidth(t *testing.T) {
	m := loadedModel(item{
		entry: entry("gitconfig", "/home/me/.gitconfig"),
		state: errored,
		err:   errors.New(strings.Repeat("very long network error ", 10)),
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(Model)

	for _, line := range strings.Split(viewText(m), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line wider than terminal (%d > 60): %q", w, line)
		}
	}
}

// 배경색을 감지한 결과에 따라 어두운 배경용이나 밝은 배경용 색을 쓴다. 감지 전에는 어두운 배경용을 쓴다.
func TestBackgroundSelectsPalette(t *testing.T) {
	m := loadedModel(item{entry: entry("a", "/home/me/.a"), state: synced})
	darkGreen, lightGreen := "38;2;74;222;128", "38;2;21;128;61" // #4ADE80, #15803D

	if v := m.View().Content; !strings.Contains(v, darkGreen) {
		t.Errorf("before detection: want dark palette")
	}

	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	m = next.(Model)
	if v := m.View().Content; !strings.Contains(v, lightGreen) || strings.Contains(v, darkGreen) {
		t.Errorf("white background: want light palette")
	}
}

// 업데이트 표시는 주황색 글자로 그린다. 배경 밝기에 따라 색도 바뀐다.
func TestUpdateMarkColors(t *testing.T) {
	m := loadedModel(item{entry: entry("a", "/home/me/.a"), state: synced, outdated: true})
	darkOrange := "38;2;251;146;60" // #FB923C
	lightOrange := "38;2;194;65;12" // #C2410C

	if v := m.View().Content; !strings.Contains(v, darkOrange) {
		t.Errorf("dark background: want the mark color %s", darkOrange)
	}
	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	if v := next.(Model).View().Content; !strings.Contains(v, lightOrange) {
		t.Errorf("white background: want the mark color %s", lightOrange)
	}
}

// 메뉴 제목 줄은 그룹 이름을 파랑, 파일 이름을 분홍으로 칠한다. 하위 파일은 그룹 줄 아래에 트리 가지로 매단다.
func TestRenderMenuTitle(t *testing.T) {
	blue, pink := "38;2;96;165;250", "38;2;244;114;182" // #60A5FA, #F472B6
	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig")},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: synced},
		item{entry: entry("env", "/home/me/.config/zsh/env.zsh"), parent: "zshrc", state: synced},
		item{entry: entry("git", "/home/me/.config/zsh/git.zsh"), parent: "zshrc"},
	)

	tests := []struct {
		cursor      int
		text        string
		colors      []string
		notExpected string
	}{
		{0, "gitconfig  ~/.gitconfig", []string{pink}, blue},
		{1, "zshrc  ~/.zshrc", []string{blue}, pink},
		{2, "zshrc  ~/.zshrc\n└─ env  ~/.config/zsh/env.zsh", []string{blue, pink}, ""},
	}
	for _, tt := range tests {
		mm := m
		mm.cursor = tt.cursor
		mm, _ = press(t, mm, "enter")
		title := mm.renderMenuTitle(newStyles(true))
		if got := ansi.Strip(title); got != tt.text {
			t.Errorf("cursor %d: title = %q, want %q", tt.cursor, got, tt.text)
		}
		for _, c := range tt.colors {
			if !strings.Contains(title, c) {
				t.Errorf("cursor %d: title should contain color %s: %q", tt.cursor, c, title)
			}
		}
		if tt.notExpected != "" && strings.Contains(title, tt.notExpected) {
			t.Errorf("cursor %d: title should not contain color %s: %q", tt.cursor, tt.notExpected, title)
		}
		if v := viewText(mm); !strings.Contains(v, strings.Split(tt.text, "\n")[0]) {
			t.Errorf("cursor %d: view missing %q:\n%s", tt.cursor, tt.text, v)
		}
	}
}

// 비교 보기 배경은 256색 터미널에서도 바뀌지 않도록 256색표에 있는 색만 쓴다.
// 옅은 분홍이나 초록은 256색으로 바뀌면서 노랑이나 하늘색이 된다.
func TestDiffTonesSurviveANSI256(t *testing.T) {
	for name, p := range map[string]palette{"dark": darkPalette, "light": lightPalette} {
		tones := []color.Color{
			p.diffRemoved.line, p.diffRemoved.number, p.diffRemoved.word,
			p.diffAdded.line, p.diffAdded.number, p.diffAdded.word,
			p.diffHunk,
		}
		for _, c := range tones {
			converted := colorprofile.ANSI256.Convert(c)
			r1, g1, b1, _ := c.RGBA()
			r2, g2, b2, _ := converted.RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				t.Errorf("%s: %v becomes %v in 256 colors", name, c, converted)
			}
		}
	}
}
