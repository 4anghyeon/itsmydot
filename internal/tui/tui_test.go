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
		"manifest.yaml": "entries:\n  - name: gitconfig\n    source: files/git/.gitconfig\n    target: ~/.gitconfig\n" +
			"agentfiles:\n  - name: claude\n    source: files/claude/CLAUDE.md\n    target: ~/.claude/CLAUDE.md\n" +
			"  - name: rules\n    children:\n      - name: go\n        source: files/claude/rules/go.md\n        target: ~/.claude/rules/go.md\n",
		"files/git/.gitconfig": "x",
	})
	return New(client, localcopy.New(t.TempDir()), "")
}

func TestEntryMenu(t *testing.T) {
	m := rootFixture(t)
	if msg := m.Init()(); msg == nil {
		t.Fatal("Init should ask for the background color")
	}

	v := ansi.Strip(m.View().Content)
	for _, want := range []string{"│ 1  dotfiles", "│ 2  brewfiles", "│ 3  agentfiles", "│    설정 파일을 이 컴퓨터에", "╭─", "╰─", "1/2/3 바로 선택", "esc 종료"} {
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
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "gitconfig") {
		t.Fatalf("dotfiles should be loaded:\n%s", v)
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
	m = loadDotfilesFromMenu(t, m)
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "연결됨 0/1") {
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
	m = loadDotfilesFromMenu(t, m)
	if v := ansi.Strip(m.View().Content); m.screen != menuScreen || !strings.Contains(v, "연결됨 0/1") {
		t.Errorf("result should reach dotfiles while on the menu (screen=%d):\n%s", m.screen, v)
	}
	// 모든 목록 화면이 터미널 폭을 받아야 구분선을 그 폭만큼 긋는다.
	for name, v := range map[string]string{"dotfiles": m.dotfiles.View().Content, "brewfiles": m.brewfiles.View().Content, "agentfiles": m.agentfiles.View().Content} {
		if !strings.Contains(ansi.Strip(v), strings.Repeat("─", 70)) {
			t.Errorf("%s should get the terminal width:\n%s", name, ansi.Strip(v))
		}
	}
}

// loadDotfilesFromMenu 는 dotfiles에 들어가 불러오기를 시작한 뒤 결과가 오기 전에 진입 메뉴로 돌아와 결과를 받는다.
func loadDotfilesFromMenu(t *testing.T, m Model) Model {
	t.Helper()
	m, load := pressRoot(t, m, "1")
	m, back := pressRoot(t, m, "esc")
	m = updateRoot(m, back())
	return updateRoot(m, load())
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

// agentfiles는 manifest의 agentfiles 섹션만 불러오고 카드에 그 섹션의 연결 비율을 보여 준다. 폴더 그룹은 세지 않는다.
func TestRootLoadsAgentfiles(t *testing.T) {
	m := rootFixture(t)
	m, load := pressRoot(t, m, "3")
	if m.screen != agentfilesScreen || load == nil {
		t.Fatalf("3 should enter agentfiles and load (screen=%d)", m.screen)
	}
	m = updateRoot(m, load())
	v := ansi.Strip(m.View().Content)
	for _, want := range []string{"claude", "rules", "go", "연결됨 0/2"} {
		if !strings.Contains(v, want) {
			t.Errorf("agentfiles missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "gitconfig") {
		t.Errorf("agentfiles should not show dotfiles entries:\n%s", v)
	}

	m, back := pressRoot(t, m, "esc")
	m = updateRoot(m, back())
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "연결됨 0/2") {
		t.Errorf("agentfiles card should show the synced count:\n%s", v)
	}
	if m.dotfiles.Summary(ui.NewStyles(true)) != "" {
		t.Error("dotfiles should stay unloaded")
	}
}
