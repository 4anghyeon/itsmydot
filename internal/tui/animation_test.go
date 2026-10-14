package tui

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestToggleAnimation(t *testing.T) {
	m, _ := toggleFixture(t, map[string]string{"files/git/.gitconfig": "x"})

	// 켜고 받기 결과가 오면 애니메이션이 시작된다. 게이지는 아직 0에서 출발한다.
	m, cmd := press(t, m, "space")
	var done tea.Msg
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(spinner.TickMsg); !ok {
			done = msg
		}
	}
	next, animationCmd := m.Update(done)
	m = next.(Model)
	if animationCmd == nil || !m.animating {
		t.Fatal("animation should start when the state changes")
	}
	if m.items[0].flash != 1 || m.gauge != 0 {
		t.Fatalf("flash = %v gauge = %v, want 1 and 0", m.items[0].flash, m.gauge)
	}

	// 이미 움직이는 중이면 tick을 겹쳐 예약하지 않는다.
	if _, again := m.startAnimation(); again != nil {
		t.Error("startAnimation should not schedule a second tick while animating")
	}

	// 프레임을 진행하면 게이지는 중간값을 거쳐 1에 도착하고, 강조색은 0으로 사라진다.
	sawMiddle := false
	steps := 0
	for ; steps < 300; steps++ {
		next, animationCmd = m.Update(animationTickMsg{})
		m = next.(Model)
		if m.gauge > 0.1 && m.gauge < 0.9 {
			sawMiddle = true
		}
		if animationCmd == nil {
			break
		}
	}
	if !sawMiddle {
		t.Error("gauge should pass through intermediate values")
	}
	if steps >= 120 { // 60fps 기준 2초 이내에 끝나야 한다
		t.Errorf("animation took %d frames, want < 120", steps)
	}
	if m.animating || m.gauge != 1 || m.items[0].flash != 0 {
		t.Errorf("after settling: animating=%v gauge=%v flash=%v", m.animating, m.gauge, m.items[0].flash)
	}
}

func TestFlashingColor(t *testing.T) {
	st := newStyles(true)
	same := func(a, b lipgloss.Style) bool {
		ar, ag, ab, _ := a.GetForeground().RGBA()
		br, bg, bb, _ := b.GetForeground().RGBA()
		return ar == br && ag == bg && ab == bb
	}
	if !same(st.flashing(synced, 0), st.state(synced)) {
		t.Error("flash 0 should be the state color")
	}
	if !same(st.flashing(synced, 1), lipgloss.NewStyle().Foreground(st.palette.accent)) {
		t.Error("flash 1 should be the accent color")
	}
	if same(st.flashing(synced, 0.5), st.state(synced)) {
		t.Error("flash 0.5 should be in between")
	}
}
