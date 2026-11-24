package ui

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
			if got := NormalizeKey(tt.msg); got != tt.want {
				t.Errorf("normalizeKey = %q, want %q", got, tt.want)
			}
		})
	}
}
