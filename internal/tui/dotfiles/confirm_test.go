package dotfiles

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// 목록에는 사유 문장만 보이고 원래 에러는 오류 항목의 메뉴에서만 보인다.
func TestMenuShowsOriginalError(t *testing.T) {
	m := loadedModel(item{
		entry: entry("gitconfig", "/home/me/.gitconfig"),
		state: ui.Errored,
		err:   fmt.Errorf("fetch files/gitconfig: %w", github.ErrNotFound),
	})
	if v := viewText(m); strings.Contains(v, "fetch files/gitconfig") || !strings.Contains(v, "(저장소에 파일이 없어요)") {
		t.Errorf("list should show only the reason:\n%s", v)
	}
	m, _ = press(t, m, "enter")
	if v := viewText(m); !strings.Contains(v, "fetch files/gitconfig: not found") {
		t.Errorf("menu should show the original error:\n%s", v)
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

// 메뉴 제목 줄은 그룹 이름을 파랑, 파일 이름을 분홍으로 칠한다. 하위 파일은 그룹 줄 아래에 트리 가지로 매단다.
func TestRenderMenuTitle(t *testing.T) {
	blue, pink := "38;2;96;165;250", "38;2;244;114;182" // #60A5FA, #F472B6
	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig")},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: ui.Synced},
		item{entry: entry("env", "/home/me/.config/zsh/env.zsh"), parent: "zshrc", state: ui.Synced},
		item{entry: entry("git", "/home/me/.config/zsh/git.zsh"), parent: "zshrc"},
	)

	tests := []struct {
		cursor      int
		text        string
		colors      []string
		notExpected string
	}{
		{0, "gitconfig  ~/.gitconfig", []string{pink}, blue},
		{1, "zshrc  ~/.zshrc", []string{blue}, pink},
		{2, "zshrc  ~/.zshrc\n└─ env  ~/.config/zsh/env.zsh", []string{blue, pink}, ""},
	}
	for _, tt := range tests {
		mm := m
		mm.cursor = tt.cursor
		mm, _ = press(t, mm, "enter")
		title := mm.renderMenuTitle(ui.NewStyles(true))
		if got := ansi.Strip(title); got != tt.text {
			t.Errorf("cursor %d: title = %q, want %q", tt.cursor, got, tt.text)
		}
		for _, c := range tt.colors {
			if !strings.Contains(title, c) {
				t.Errorf("cursor %d: title should contain color %s: %q", tt.cursor, c, title)
			}
		}
		if tt.notExpected != "" && strings.Contains(title, tt.notExpected) {
			t.Errorf("cursor %d: title should not contain color %s: %q", tt.cursor, tt.notExpected, title)
		}
		if v := viewText(mm); !strings.Contains(v, strings.Split(tt.text, "\n")[0]) {
			t.Errorf("cursor %d: view missing %q:\n%s", tt.cursor, tt.text, v)
		}
	}
}
