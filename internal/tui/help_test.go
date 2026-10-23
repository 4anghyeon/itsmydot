package tui

import (
	"strings"
	"testing"
)

func TestHelpScreen(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	if v := viewText(m); !strings.Contains(v, "? 도움말") {
		t.Errorf("footer missing help key:\n%s", v)
	}

	m, _ = press(t, m, "?")
	v := viewText(m)
	for _, want := range []string{"동기화 상태", "[!] 다른 파일이 있음", ">   접힌 그룹", "사본 저장 경로: ~/.itsmydot"} {
		if !strings.Contains(v, want) {
			t.Errorf("help missing %q:\n%s", want, v)
		}
	}
	// macOS 터미널 기본 높이 24줄 안에 들어가야 한다.
	if lines := strings.Count(v, "\n"); lines > 24 {
		t.Errorf("help is %d lines tall:\n%s", lines, v)
	}

	// 도움말이 떠 있는 동안 목록 키는 먹지 않는다.
	m, cmd := press(t, m, "space")
	if cmd != nil || m.confirmingName != "" || !m.helpOpen {
		t.Fatalf("space acted while help was open")
	}

	for _, key := range []string{"?", "esc"} {
		m, cmd = press(t, m, key)
		if cmd != nil || m.helpOpen {
			t.Errorf("%s did not close help (cmd=%v)", key, cmd != nil)
		}
		m, _ = press(t, m, "?")
	}
}
