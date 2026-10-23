package tui

import (
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// dubeolsik 은 두벌식 자판의 자모를 같은 자리의 영문 키로 바꾸는 표다.
// 한글 입력 상태에서 눌러도 단축키가 동작하게 한다.
var dubeolsik = map[rune]rune{
	'ㅂ': 'q', 'ㅈ': 'w', 'ㄷ': 'e', 'ㄱ': 'r', 'ㅅ': 't', 'ㅛ': 'y', 'ㅕ': 'u', 'ㅑ': 'i', 'ㅐ': 'o', 'ㅔ': 'p',
	'ㅁ': 'a', 'ㄴ': 's', 'ㅇ': 'd', 'ㄹ': 'f', 'ㅎ': 'g', 'ㅗ': 'h', 'ㅓ': 'j', 'ㅏ': 'k', 'ㅣ': 'l',
	'ㅋ': 'z', 'ㅌ': 'x', 'ㅊ': 'c', 'ㅍ': 'v', 'ㅠ': 'b', 'ㅜ': 'n', 'ㅡ': 'm',

	// shift를 누른 자모
	'ㅃ': 'Q', 'ㅉ': 'W', 'ㄸ': 'E', 'ㄲ': 'R', 'ㅆ': 'T', 'ㅒ': 'O', 'ㅖ': 'P',
}

// normalizeKey 는 키 입력을 단축키 이름("q", "y", "space" 등)으로 바꾼다.
// 두벌식 자모가 들어오면 같은 자리의 영문 키로 바꾼다.
func normalizeKey(msg tea.KeyPressMsg) string {
	if msg.Code == tea.KeyTab || msg.String() == "\t" {
		return "tab"
	}
	name := msg.String()
	r, size := utf8.DecodeRuneInString(name)
	if size == len(name) {
		if qwerty, ok := dubeolsik[r]; ok {
			return string(qwerty)
		}
	}
	return name
}
