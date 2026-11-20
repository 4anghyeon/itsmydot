package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func TestCursorMovesWithinBounds(t *testing.T) {
	m := loadedModel(
		item{entry: entry("a", "/home/me/.a")},
		item{entry: entry("b", "/home/me/.b")},
	)

	m, _ = press(t, m, "up") // 맨 위라서 그대로
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "j")
	if m.cursor != 1 { // 맨 아래에서 더 내려가지 않음
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
	m, _ = press(t, m, "k")
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestQuitAndBackToMenu(t *testing.T) {
	_, cmd := press(t, loadedModel(), "q")
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit")
	}

	_, cmd = press(t, loadedModel(), "esc")
	if cmd == nil {
		t.Fatal("esc should return a command")
	}
	if _, ok := cmd().(backToMenuMsg); !ok {
		t.Error("esc should go back to the entry menu")
	}
}

// 받는 중인 항목은 체크박스 안에서 스피너가 돌고, 모두 끝나면 스피너가 멈춘다.
func TestSpinner(t *testing.T) {
	m, _ := toggleFixture(t, map[string]string{"files/git/.gitconfig": "x"})

	m, cmd := confirmToggle(t, m)
	if v := viewText(m); !strings.Contains(v, "[.] gitconfig") {
		t.Fatalf("syncing item should show the first spinner frame:\n%s", v)
	}

	// 스피너 tick이 오면 다음 모양으로 바뀌고, 다음 tick을 예약한다.
	var tick, done tea.Msg
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			tick = msg
		} else {
			done = msg
		}
	}
	next, tickCmd := m.Update(tick)
	m = next.(dotfileList)
	if tickCmd == nil {
		t.Error("spinner should keep ticking while syncing")
	}
	if v := viewText(m); !strings.Contains(v, "[o] gitconfig") {
		t.Errorf("spinner should advance:\n%s", v)
	}

	// 받기가 끝나면 다음 tick을 예약하지 않는다.
	next, _ = m.Update(done)
	m = next.(dotfileList)
	if _, tickCmd = m.Update(m.spinner.Tick()); tickCmd != nil {
		t.Error("spinner should stop when nothing is syncing")
	}
}
