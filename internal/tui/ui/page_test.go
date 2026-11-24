package ui

import (
	"strings"
	"testing"

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
		g := ansi.Strip(NewStyles(true).RenderGauge(tt.ratio))
		if got := strings.Count(g, "█"); got != tt.filled {
			t.Errorf("gauge(%v) filled = %d, want %d", tt.ratio, got, tt.filled)
		}
		if w := lipgloss.Width(g); w != gaugeWidth {
			t.Errorf("gauge(%v) width = %d, want %d", tt.ratio, w, gaugeWidth)
		}
	}
}

// 알림이 없으면 상태 줄 자리를 빈 줄로 남기고 배경은 칠하지 않는다.
func TestEmptyStatusLineHasNoBackground(t *testing.T) {
	st := NewStyles(true)
	if got := st.RenderStatusLine(40, "", st.Muted.GetForeground(), ""); got != "" {
		t.Errorf("empty status line = %q, want no painted cells", got)
	}
}
