package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github/githubtest"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// pressRoot 는 루트 모델에 키 하나를 누른 것처럼 Update를 호출한다.
func pressRoot(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyPressMsg
	switch key {
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		msg = tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// updateRoot 는 루트 모델에 메시지 하나를 넣는다.
func updateRoot(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func rootFixture(t *testing.T) Model {
	t.Helper()
	client := githubtest.NewClient(t, map[string]string{
		"manifest.yaml":        "entries:\n  - name: gitconfig\n    source: files/git/.gitconfig\n    target: ~/.gitconfig\n",
		"files/git/.gitconfig": "x",
	})
	return New(client, localcopy.New(t.TempDir()), "")
}

func TestEntryMenu(t *testing.T) {
	m := rootFixture(t)
	if msg := m.Init()(); msg == nil {
		t.Fatal("Init should ask for the background color")
	} else if _, ok := msg.(entriesLoadedMsg); ok {
		t.Fatal("manifest should not load before entering dotfiles")
	}

	v := ansi.Strip(m.View().Content)
	for _, want := range []string{"│ 1  dotfiles", "│ 2  brewfiles", "│    설정 파일을 이 컴퓨터에", "╭─", "╰─", "1/2 바로 선택", "esc 종료"} {
		if !strings.Contains(v, want) {
			t.Errorf("menu missing %q:\n%s", want, v)
		}
	}

	// 처음 dotfiles에 들어갈 때만 manifest를 받는다.
	m, cmd := pressRoot(t, m, "1")
	if m.screen != dotfilesScreen || cmd == nil {
		t.Fatalf("1 should enter dotfiles and load the manifest (screen=%d)", m.screen)
	}
	m = updateRoot(m, cmd())
	if !m.dotfiles.ready || len(m.dotfiles.items) != 1 {
		t.Fatalf("dotfiles should be loaded: ready=%v items=%d err=%v", m.dotfiles.ready, len(m.dotfiles.items), m.dotfiles.loadErr)
	}

	m, cmd = pressRoot(t, m, "esc")
	m = updateRoot(m, cmd())
	if m.screen != menuScreen {
		t.Fatalf("esc in dotfiles should go back to the menu, screen=%d", m.screen)
	}
	if m, cmd = pressRoot(t, m, "enter"); m.screen != dotfilesScreen || cmd != nil {
		t.Errorf("entering dotfiles again should not reload (screen=%d cmd=%v)", m.screen, cmd != nil)
	}
}

// 진입 메뉴의 커서는 마지막에 고른 메뉴에 남는다.
func TestEntryMenuRemembersChoice(t *testing.T) {
	m := rootFixture(t)
	m, _ = pressRoot(t, m, "down")
	m, _ = pressRoot(t, m, "enter")
	if m.screen != brewfilesScreen {
		t.Fatalf("enter on brewfiles should open it, screen=%d", m.screen)
	}
	m, cmd := pressRoot(t, m, "esc")
	m = updateRoot(m, cmd())
	if m.screen != menuScreen || m.choice != 1 {
		t.Errorf("back on menu: screen=%d choice=%d, want menu and 1", m.screen, m.choice)
	}
}

// 커서가 있는 카드만 테두리를 강조색으로 칠하고, 터미널 폭에 맞춰 카드 폭을 정한다.
func TestMenuCards(t *testing.T) {
	m := rootFixture(t)
	m = updateRoot(m, tea.WindowSizeMsg{Width: 70, Height: 24})
	st := ui.NewStyles(true)
	accent := lipgloss.NewStyle().Foreground(st.Palette.Accent).Render("╭")
	accentCode := accent[:strings.Index(accent, "╭")]

	first, second := m.renderMenuCard(st, 0, 68), m.renderMenuCard(st, 1, 68)
	if !strings.HasPrefix(first, accentCode) || strings.HasPrefix(second, accentCode) {
		t.Errorf("only the selected card should have an accent border:\n%q\n%q", first, second)
	}
	for line := range strings.SplitSeq(ansi.Strip(first), "\n") {
		if w := lipgloss.Width(line); w != 68 {
			t.Errorf("card line width = %d, want 68: %q", w, line)
		}
	}

	// dotfiles를 불러온 뒤에는 카드에 연결된 비율을 보여 준다.
	if strings.Contains(ansi.Strip(first), "연결됨") {
		t.Error("summary should not show before loading")
	}
	m = updateRoot(m, entriesLoadedMsg{items: []item{{entry: entry("a", "/home/me/.a"), state: ui.Synced}, {entry: entry("b", "/home/me/.b")}}})
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "연결됨 1/2") {
		t.Errorf("dotfiles card should show the synced count:\n%s", v)
	}
	for line := range strings.SplitSeq(ansi.Strip(m.View().Content), "\n") {
		if w := lipgloss.Width(line); w > 70 {
			t.Errorf("line wider than terminal (%d > 70): %q", w, line)
		}
	}
}

func TestEntryMenuQuits(t *testing.T) {
	_, cmd := pressRoot(t, rootFixture(t), "esc")
	if cmd == nil {
		t.Fatal("esc on the menu should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("esc on the menu should quit")
	}
}

// 진입 메뉴로 돌아간 뒤에 도착한 결과도 dotfiles 목록에 반영된다.
func TestRootForwardsResultsWhileOnMenu(t *testing.T) {
	m := rootFixture(t)
	m = updateRoot(m, tea.WindowSizeMsg{Width: 70, Height: 20})
	m = updateRoot(m, entriesLoadedMsg{items: []item{{entry: entry("gitconfig", "/home/me/.gitconfig")}}})
	// brewfiles 화면은 터미널 폭을 받아야 구분선을 그 폭만큼 긋는다.
	brewfiles := ansi.Strip(m.brewfiles.View().Content)
	if !m.dotfiles.ready || m.dotfiles.width != 70 || !strings.Contains(brewfiles, strings.Repeat("─", 70)) {
		t.Errorf("messages should reach both lists: ready=%v width=%d\n%s", m.dotfiles.ready, m.dotfiles.width, brewfiles)
	}
}

// 진입 메뉴에서 brewfiles에 처음 들어갈 때만 불러온다.
func TestRootLoadsBrewfilesOnce(t *testing.T) {
	m := rootFixture(t)
	m, cmd := pressRoot(t, m, "2")
	if m.screen != brewfilesScreen || cmd == nil {
		t.Fatalf("2 should enter brewfiles and load (screen=%d)", m.screen)
	}
	m, back := pressRoot(t, m, "esc")
	m = updateRoot(m, back())
	if _, cmd = pressRoot(t, m, "2"); cmd != nil {
		t.Error("entering brewfiles again should not reload")
	}
}
