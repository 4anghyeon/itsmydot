package tui

import (
	"strings"
	"testing"

	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// 상태 줄은 결과에 따라 [+]나 [-]를 앞에 붙이고 메시지가 없어도 자리를 지켜 화면 높이가 바뀌지 않는다.
func TestViewStatusLine(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	before := strings.Count(viewText(m), "\n")

	for tone, want := range map[ui.StatusTone]string{
		ui.StatusDone:     " [+] 끝났어요",
		ui.StatusUnlinked: " [ ] 끝났어요",
		ui.StatusFailed:   " [-] 끝났어요",
		ui.StatusInfo:     "\n 끝났어요",
	} {
		m.Status, m.StatusTone = "끝났어요", tone
		v := viewText(m)
		if !strings.Contains(v, want) {
			t.Errorf("tone %d: view missing %q:\n%s", tone, want, v)
		}
		if after := strings.Count(v, "\n"); after != before {
			t.Errorf("tone %d: view height changed from %d to %d lines", tone, before, after)
		}
	}
}

// 알림은 목록에서 키를 ui.StatusKeyLimit번 누르면 사라진다. 받는 중이라는 알림은 결과로 바뀔 때까지 남는다.
func TestStatusFadesAfterKeys(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")}, item{entry: entry("zshrc", "/home/me/.zshrc")})
	m = m.notify(ui.StatusDone, "끝났어요")
	for i := 1; i < ui.StatusKeyLimit; i++ {
		m, _ = press(t, m, "down")
		if m.Status == "" {
			t.Fatalf("status cleared after %d keys", i)
		}
	}
	m, _ = press(t, m, "up")
	if m.Status != "" {
		t.Errorf("status = %q after %d keys, want cleared", m.Status, ui.StatusKeyLimit)
	}

	m = m.notify(ui.StatusBusy, "받는 중이에요...")
	for range ui.StatusKeyLimit {
		m, _ = press(t, m, "down")
	}
	if m.Status == "" {
		t.Error("busy status should stay until the result arrives")
	}
}

// 메뉴를 닫거나 질문을 취소하면 알림을 띄우지 않고 질문 전의 알림도 지운다.
func TestCloseClearsStatus(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	m = m.notify(ui.StatusDone, "끝났어요")
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "esc")
	if m.confirmingName != "" || m.Status != "" {
		t.Errorf("confirming = %q status = %q", m.confirmingName, m.Status)
	}
}
