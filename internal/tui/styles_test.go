package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestGauge(t *testing.T) {
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
		g := ansi.Strip(newStyles(true).gauge(tt.ratio))
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

// 터미널이 좁으면 상자를 터미널 폭에 맞추고, 긴 에러는 상자 안에서 줄바꿈한다.
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
