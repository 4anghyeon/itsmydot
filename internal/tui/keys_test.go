package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func hangul(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{"영문 그대로", tea.KeyPressMsg{Code: 'y', Text: "y"}, "y"},
		{"ㅂ → q", hangul('ㅂ'), "q"},
		{"ㅛ → y", hangul('ㅛ'), "y"},
		{"ㅜ → n", hangul('ㅜ'), "n"},
		{"ㅏ → k", hangul('ㅏ'), "k"},
		{"ㅓ → j", hangul('ㅓ'), "j"},
		{"ㄱ → r", hangul('ㄱ'), "r"},
		{"shift: ㅃ → Q", hangul('ㅃ'), "Q"},
		{"완성형 글자는 그대로", hangul('바'), "바"},
		{"특수 키는 그대로", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, "space"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeKey(tt.msg); got != tt.want {
				t.Errorf("normalizeKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHangulKeysInModel(t *testing.T) {
	m := loadedModel(
		item{entry: entry("a", "/home/me/.a")},
		item{entry: entry("b", "/home/me/.b")},
	)
	update := func(m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
		next, cmd := m.Update(msg)
		return next.(Model), cmd
	}

	m, _ = update(m, hangul('ㅓ'))
	if m.cursor != 1 {
		t.Errorf("ㅓ: cursor = %d, want 1", m.cursor)
	}
	m, _ = update(m, hangul('ㅏ'))
	if m.cursor != 0 {
		t.Errorf("ㅏ: cursor = %d, want 0", m.cursor)
	}
	_, cmd := update(m, hangul('ㅂ'))
	if cmd == nil {
		t.Fatal("ㅂ should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ㅂ should quit")
	}
}
