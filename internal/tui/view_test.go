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
		"> [+] gitconfig  ~/.gitconfig",
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

	// 설명이 없는 항목으로 옮기면 설명 줄은 비지만 상자 높이는 그대로다.
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

func TestDescriptionBlockWrapsUnderBar(t *testing.T) {
	m := Model{width: 13}
	got := ansi.Strip(m.descriptionBlock(newStyles(true), "one two three"))
	if want := "┃ one two\n  three"; got != want {
		t.Errorf("descriptionBlock = %q, want %q", got, want)
	}
	if got := m.descriptionBlock(newStyles(true), ""); got != "" {
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
		if strings.HasPrefix(line, "▌> [ ] gitconfig") {
			if w := ansi.StringWidth(line); w != 60 {
				t.Errorf("selected row is %d cells wide, want 60: %q", w, line)
			}
			return
		}
	}
	t.Errorf("selected row not found:\n%s", viewText(m))
}

// 넓은 터미널에서도 게이지는 headerMaxWidth 칸에서 끝나고, 상태 줄과 도움말 줄 사이는 한 줄 띄운다.
func TestViewHeaderMaxWidthAndFooterGap(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m = next.(Model)
	m.status, m.statusTone = "끝났어요", statusDone

	lines := strings.Split(viewText(m), "\n")
	if w := ansi.StringWidth(lines[1]); w != headerMaxWidth {
		t.Errorf("header is %d cells wide, want %d: %q", w, headerMaxWidth, lines[1])
	}
	if w := ansi.StringWidth(lines[3]); w != 200 {
		t.Errorf("rule is %d cells wide, want 200", w)
	}
	// 마지막 줄바꿈 뒤의 빈 줄, 도움말, 빈 줄, 상태 줄 순이다.
	n := len(lines)
	if !strings.HasPrefix(lines[n-4], " [+] 끝났어요") || strings.TrimSpace(lines[n-3]) != "" || !strings.HasPrefix(lines[n-2], " 방향키") {
		t.Errorf("status line and footer should be one line apart:\n%s", strings.Join(lines[n-4:], "\n"))
	}
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
	if m.confirming != "" || m.status != "" {
		t.Errorf("confirming = %q status = %q", m.confirming, m.status)
	}
}

// 알림이 없으면 상태 줄 자리를 빈 줄로 남기고 배경은 칠하지 않는다.
func TestEmptyStatusLineHasNoBackground(t *testing.T) {
	st := newStyles(true)
	if got := st.statusLine(40, "", st.muted.GetForeground(), ""); got != "" {
		t.Errorf("empty status line = %q, want no painted cells", got)
	}
}

// 버전을 알면 제목 아래 줄에 제목과 같은 칸부터 적는다.
func TestViewShowsVersion(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")})
	m.version = "v1.2.3"
	lines := strings.Split(viewText(m), "\n")
	title, version := strings.Index(lines[1], "itsmydot"), strings.Index(lines[2], "v1.2.3")
	if title < 0 || version != title {
		t.Errorf("version should sit under the title:\n%s", strings.Join(lines[:3], "\n"))
	}
}
