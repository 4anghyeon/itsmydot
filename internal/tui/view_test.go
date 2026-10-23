package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
)

func TestView(t *testing.T) {
	loading := viewText(Model{})
	if !strings.Contains(loading, "불러오는 중") {
		t.Errorf("loading view = %q", loading)
	}

	failed := loadedModel()
	failed.loadErr = errors.New("boom")
	if v := viewText(failed); !strings.Contains(v, "boom") {
		t.Errorf("error view = %q", v)
	}

	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: conflict},
		item{entry: entry("vimrc", "/home/me/.vimrc"), state: errored, err: github.ErrNotFound},
		item{entry: entry("tmux", "/home/me/.tmux.conf"), state: syncing},
	)
	v := viewText(m)
	for _, want := range []string{
		"▌ [+] gitconfig  ~/.gitconfig",
		"  [!] zshrc      ~/.zshrc   (다른 파일이 있음)",
		"  [-] vimrc      ~/.vimrc   (저장소에 파일이 없어요)",
		"  [~] tmux       ~/.tmux.conf",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestTildify(t *testing.T) {
	tests := map[string]string{
		"/home/me":          "~",
		"/home/me/.zshrc":   "~/.zshrc",
		"/home/me/a/b":      "~/a/b",
		"/home/meow/.zshrc": "/home/meow/.zshrc",
		"/etc/x":            "/etc/x",
	}
	for in, want := range tests {
		if got := tildify(in, "/home/me"); got != want {
			t.Errorf("tildify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestViewShowsCursorDescription(t *testing.T) {
	git := entry("gitconfig", "/home/me/.gitconfig")
	git.Description = "git 전역 설정"
	m := loadedModel(item{entry: git}, item{entry: entry("zshrc", "/home/me/.zshrc")})
	if v := viewText(m); !strings.Contains(v, "┃ git 전역 설정") {
		t.Errorf("view missing description of the cursor item:\n%s", v)
	}

	// 설명이 없는 항목으로 옮기면 설명 줄은 비지만 화면 높이는 그대로다.
	before := strings.Count(viewText(m), "\n")
	m, _ = press(t, m, "down")
	v := viewText(m)
	if strings.Contains(v, "git 전역 설정") {
		t.Errorf("description of a non-cursor item is shown:\n%s", v)
	}
	if after := strings.Count(v, "\n"); after != before {
		t.Errorf("view height changed from %d to %d lines", before, after)
	}
}

func TestRenderDescriptionWrapsUnderBar(t *testing.T) {
	m := Model{width: 13}
	got := ansi.Strip(m.renderDescription(newStyles(true), "one two three"))
	if want := "┃ one two\n  three"; got != want {
		t.Errorf("descriptionBlock = %q, want %q", got, want)
	}
	if got := m.renderDescription(newStyles(true), ""); got != "" {
		t.Errorf("empty description = %q", got)
	}
}

// 상태 줄은 결과에 따라 [+]나 [-]를 앞에 붙이고, 메시지가 없어도 자리를 지켜 화면 높이가 바뀌지 않는다.
func TestViewStatusLine(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	before := strings.Count(viewText(m), "\n")

	for tone, want := range map[statusTone]string{
		statusDone:     " [+] 끝났어요",
		statusUnlinked: " [ ] 끝났어요",
		statusFailed:   " [-] 끝났어요",
		statusInfo:     "\n 끝났어요",
	} {
		m.status, m.statusTone = "끝났어요", tone
		v := viewText(m)
		if !strings.Contains(v, want) {
			t.Errorf("tone %d: view missing %q:\n%s", tone, want, v)
		}
		if after := strings.Count(v, "\n"); after != before {
			t.Errorf("tone %d: view height changed from %d to %d lines", tone, before, after)
		}
	}
}

// 커서가 있는 줄은 앞에 막대를 세우고 배경을 터미널 끝까지 칠한다.
func TestViewSelectedRowFillsWidth(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")}, item{entry: entry("zshrc", "/home/me/.zshrc")})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = next.(Model)

	for line := range strings.SplitSeq(viewText(m), "\n") {
		if strings.HasPrefix(line, "▌ [ ] gitconfig") {
			if w := ansi.StringWidth(line); w != 60 {
				t.Errorf("selected row is %d cells wide, want 60: %q", w, line)
			}
			return
		}
	}
	t.Errorf("selected row not found:\n%s", viewText(m))
}

// 알림은 목록에서 키를 statusKeyLimit번 누르면 사라진다. 받는 중이라는 알림은 결과로 바뀔 때까지 남는다.
func TestStatusFadesAfterKeys(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")}, item{entry: entry("zshrc", "/home/me/.zshrc")})
	m = m.notify(statusDone, "끝났어요")
	for i := 1; i < statusKeyLimit; i++ {
		m, _ = press(t, m, "down")
		if m.status == "" {
			t.Fatalf("status cleared after %d keys", i)
		}
	}
	m, _ = press(t, m, "up")
	if m.status != "" {
		t.Errorf("status = %q after %d keys, want cleared", m.status, statusKeyLimit)
	}

	m = m.notify(statusBusy, "받는 중이에요...")
	for range statusKeyLimit {
		m, _ = press(t, m, "down")
	}
	if m.status == "" {
		t.Error("busy status should stay until the result arrives")
	}
}

// 메뉴를 닫거나 질문을 취소하면 알림을 띄우지 않고 질문 전의 알림도 지운다.
func TestCloseClearsStatus(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	m = m.notify(statusDone, "끝났어요")
	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "esc")
	if m.confirmingName != "" || m.status != "" {
		t.Errorf("confirming = %q status = %q", m.confirmingName, m.status)
	}
}
