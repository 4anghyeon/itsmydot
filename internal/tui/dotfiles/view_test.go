package dotfiles

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/manifest"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

func TestView(t *testing.T) {
	loading := viewText(List{})
	if !strings.Contains(loading, "불러오는 중") {
		t.Errorf("loading view = %q", loading)
	}

	failed := loadedModel()
	failed.loadErr = errors.New("boom")
	if v := viewText(failed); !strings.Contains(v, "boom") {
		t.Errorf("error view = %q", v)
	}

	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: ui.Synced},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: ui.Conflict},
		item{entry: entry("vimrc", "/home/me/.vimrc"), state: ui.Errored, err: github.ErrNotFound},
		item{entry: entry("tmux", "/home/me/.tmux.conf"), state: ui.Syncing},
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
	st := ui.NewStyles(true)
	got := ansi.Strip(st.RenderDescription("one two three", 13))
	if want := "┃ one two\n  three"; got != want {
		t.Errorf("descriptionBlock = %q, want %q", got, want)
	}
	if got := st.RenderDescription("", 13); got != "" {
		t.Errorf("empty description = %q", got)
	}
}

// 커서가 있는 줄은 앞에 막대를 세우고 배경을 터미널 끝까지 칠한다.
func TestViewSelectedRowFillsWidth(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig")}, item{entry: entry("zshrc", "/home/me/.zshrc")})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = next.(List)

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

// manifest 형식이 잘못됐으면 사유 아래에 틀린 곳을 한 줄씩 보여 준다.
func TestViewShowsManifestProblems(t *testing.T) {
	m := loadedModel()
	m.loadErr = &manifest.InvalidError{Problems: []string{"a 항목에 source가 없어요", "a 항목에 target이 없어요"}}
	v := viewText(m)
	for _, want := range []string{"manifest를 불러오지 못했어요. manifest.yaml 형식이 잘못됐어요", "a 항목에 source가 없어요", "a 항목에 target이 없어요"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestHeaderShowsSyncedCount(t *testing.T) {
	m := loadedModel(
		item{entry: entry("a", "/home/me/.a"), state: ui.Synced},
		item{entry: entry("b", "/home/me/.b"), state: ui.Conflict},
		item{entry: entry("c", "/home/me/.c")},
	)
	if v := viewText(m); !strings.Contains(v, "연결됨 1/3") {
		t.Errorf("header missing synced count:\n%s", v)
	}
}

// 터미널이 좁으면 화면을 터미널 폭에 맞추고 긴 에러는 줄바꿈한다.
func TestViewFitsTerminalWidth(t *testing.T) {
	m := loadedModel(item{
		entry: entry("gitconfig", "/home/me/.gitconfig"),
		state: ui.Errored,
		err:   errors.New(strings.Repeat("very long network error ", 10)),
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = next.(List)

	for _, line := range strings.Split(viewText(m), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line wider than terminal (%d > 60): %q", w, line)
		}
	}
}

// 넓은 터미널에서도 게이지는 [ui.HeaderMaxWidth] 칸에서 끝나고 상태 줄과 도움말 줄 사이는 한 줄 띄운다.
func TestViewHeaderMaxWidthAndFooterGap(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: ui.Synced})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m = next.(List)
	m.Status, m.StatusTone = "끝났어요", ui.StatusDone

	lines := strings.Split(viewText(m), "\n")
	if w := ansi.StringWidth(lines[1]); w != ui.HeaderMaxWidth {
		t.Errorf("header is %d cells wide, want %d: %q", w, ui.HeaderMaxWidth, lines[1])
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

// 아래 도움말 줄이 터미널보다 길면 키와 설명 묶음 사이에서 줄을 나눈다.
func TestHelpLineWraps(t *testing.T) {
	m := loadedModel(item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: ui.Synced})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m = next.(List)

	lines := strings.Split(strings.TrimRight(viewText(m), "\n"), "\n")
	footer := strings.Join(lines[len(lines)-4:], "\n")
	if want := " 방향키 이동   space 연결/연결 끊기\n enter 메뉴   tab 미리보기   ? 도움말\n esc 진입 메뉴"; !strings.HasSuffix(footer, want) {
		t.Errorf("footer = %q, want suffix %q", footer, want)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line wider than terminal (%d > 40): %q", w, line)
		}
	}
}

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

// 업데이트 표시는 주황색 글자로 그린다. 배경 밝기에 따라 색도 바뀐다.
func TestUpdateMarkColors(t *testing.T) {
	m := loadedModel(item{entry: entry("a", "/home/me/.a"), state: ui.Synced, outdated: true})
	darkOrange := "38;2;251;146;60" // #FB923C
	lightOrange := "38;2;194;65;12" // #C2410C

	if v := m.View().Content; !strings.Contains(v, darkOrange) {
		t.Errorf("dark background: want the mark color %s", darkOrange)
	}
	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	if v := next.(List).View().Content; !strings.Contains(v, lightOrange) {
		t.Errorf("white background: want the mark color %s", lightOrange)
	}
}
