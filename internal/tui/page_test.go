package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// 넓은 터미널에서도 게이지는 headerMaxWidth 칸에서 끝나고 상태 줄과 도움말 줄 사이는 한 줄 띄운다.
func TestViewHeaderMaxWidthAndFooterGap(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m = next.(Model)
	m.status, m.statusTone = "끝났어요", statusDone

	lines := strings.Split(viewText(m), "\n")
	if w := ansi.StringWidth(lines[1]); w != headerMaxWidth {
		t.Errorf("header is %d cells wide, want %d: %q", w, headerMaxWidth, lines[1])
	}
	if w := ansi.StringWidth(lines[3]); w != 200 {
		t.Errorf("rule is %d cells wide, want 200", w)
	}
	// 마지막 줄바꿈 뒤의 빈 줄, 도움말, 빈 줄, 상태 줄 순이다.
	n := len(lines)
	if !strings.HasPrefix(lines[n-4], " [+] 끝났어요") || strings.TrimSpace(lines[n-3]) != "" || !strings.HasPrefix(lines[n-2], " 방향키") {
		t.Errorf("status line and footer should be one line apart:\n%s", strings.Join(lines[n-4:], "\n"))
	}
}

// 버전을 알면 제목 아래 줄에 제목과 같은 칸부터 적는다.
func TestViewShowsVersion(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	m.version = "v1.2.3"
	lines := strings.Split(viewText(m), "\n")
	title, version := strings.Index(lines[1], "itsmydot"), strings.Index(lines[2], "v1.2.3")
	if title < 0 || version != title {
		t.Errorf("version should sit under the title:\n%s", strings.Join(lines[:3], "\n"))
	}
}

// 알림이 없으면 상태 줄 자리를 빈 줄로 남기고 배경은 칠하지 않는다.
func TestEmptyStatusLineHasNoBackground(t *testing.T) {
	st := newStyles(true)
	if got := st.renderStatusLine(40, "", st.muted.GetForeground(), ""); got != "" {
		t.Errorf("empty status line = %q, want no painted cells", got)
	}
}

// 아래 도움말 줄이 터미널보다 길면 키와 설명 묶음 사이에서 줄을 나눈다.
func TestHelpLineWraps(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = next.(Model)

	lines := strings.Split(strings.TrimRight(viewText(m), "\n"), "\n")
	footer := strings.Join(lines[len(lines)-3:], "\n")
	if want := " 방향키 이동   space 연결/연결 끊기\n enter 메뉴   ? 도움말   esc 종료"; !strings.HasSuffix(footer, want) {
		t.Errorf("footer = %q, want suffix %q", footer, want)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line wider than terminal (%d > 40): %q", w, line)
		}
	}
}
