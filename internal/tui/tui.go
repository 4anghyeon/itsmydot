// Package tui 는 itsmydot의 터미널 화면이다. 진입 메뉴에서 dotfiles, brewfiles, agentfiles 중 하나를 고르면 그 목록 화면으로 들어간다.
package tui

import (
	"maps"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/tui/brewfiles"
	"github.com/4anghyeon/itsmydot/internal/tui/dotfiles"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// screen 은 지금 보이는 화면이다.
type screen int

const (
	menuScreen screen = iota
	dotfilesScreen
	brewfilesScreen
	agentfilesScreen
)

// menuOption 은 진입 메뉴의 선택지 하나다.
type menuOption struct {
	label       string
	description string
	screen      screen
}

// menuOptions 는 진입 메뉴의 선택지를 번호 순서대로 담는다.
var menuOptions = []menuOption{
	{"dotfiles", "설정 파일을 이 컴퓨터에 연결하거나 연결을 끊어요", dotfilesScreen},
	{"brewfiles", "Homebrew 패키지를 골라 설치해요", brewfilesScreen},
	{"agentfiles", "코딩 에이전트의 지침과 규칙, 스킬을 연결해요", agentfilesScreen},
}

// Model 은 진입 메뉴와 목록 화면을 오가는 루트 모델이다. 메뉴와 화면 전환만 맡고 목록의 일은 각 목록 모델에 넘긴다.
type Model struct {
	dotfiles   dotfiles.List
	brewfiles  brewfiles.List
	agentfiles dotfiles.List

	screen screen
	choice int // 진입 메뉴에서 커서가 있는 선택지. 목록에서 돌아오면 마지막에 고른 메뉴에 그대로 있다

	// started 는 불러오기를 시작한 목록 화면이다. 다시 들어가도 새로 받지 않는다.
	started map[screen]bool

	version string
	width   int  // 터미널 폭. 모르면 0이다
	light   bool // 터미널 배경이 밝으면 true
}

// New 는 진입 메뉴부터 보여 주는 처음 화면 상태를 만든다. version은 제목 옆에 보여 줄 버전이다.
func New(client *github.Client, copies *localcopy.Dir, version string) Model {
	return Model{
		dotfiles:   dotfiles.New(client, copies, version, dotfiles.EntriesSection),
		brewfiles:  brewfiles.New(client, version),
		agentfiles: dotfiles.New(client, copies, version, dotfiles.AgentfilesSection),
		version:    version,
	}
}

// Init 은 색을 고르기 위해 터미널 배경색을 물어본다. manifest는 각 목록 메뉴에 처음 들어갈 때 받는다.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update 는 키 입력을 지금 보이는 화면에 넘기고 그 밖의 메시지는 모든 목록에 넘긴다.
// 진입 메뉴로 돌아간 뒤에도 받는 중이던 작업의 결과가 목록에 반영돼야 하기 때문이다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.screen {
		case dotfilesScreen:
			next, cmd := m.dotfiles.Update(msg)
			m.dotfiles = next.(dotfiles.List)
			return m, cmd
		case brewfilesScreen:
			next, cmd := m.brewfiles.Update(msg)
			m.brewfiles = next.(brewfiles.List)
			return m, cmd
		case agentfilesScreen:
			next, cmd := m.agentfiles.Update(msg)
			m.agentfiles = next.(dotfiles.List)
			return m, cmd
		case menuScreen:
			return m.handleMenuKey(ui.NormalizeKey(msg))
		}
	case ui.BackToMenuMsg:
		m.screen = menuScreen
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
	}

	next, dotfilesCmd := m.dotfiles.Update(msg)
	m.dotfiles = next.(dotfiles.List)
	next, brewfilesCmd := m.brewfiles.Update(msg)
	m.brewfiles = next.(brewfiles.List)
	next, agentfilesCmd := m.agentfiles.Update(msg)
	m.agentfiles = next.(dotfiles.List)
	return m, tea.Batch(dotfilesCmd, brewfilesCmd, agentfilesCmd)
}

// handleMenuKey 는 진입 메뉴의 키 입력을 처리한다. 고른 메뉴의 목록 화면으로 들어가고 esc는 종료다.
func (m Model) handleMenuKey(key string) (Model, tea.Cmd) {
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	}
	var picked int
	m.choice, picked = ui.MoveChoice(key, m.choice, len(menuOptions))
	if picked == 0 {
		return m, nil
	}
	m.choice = picked - 1
	m.screen = menuOptions[m.choice].screen
	if m.started[m.screen] {
		return m, nil
	}
	m.started = maps.Clone(m.started)
	if m.started == nil {
		m.started = make(map[screen]bool)
	}
	m.started[m.screen] = true
	switch m.screen {
	case brewfilesScreen:
		return m, m.brewfiles.Init()
	case agentfilesScreen:
		return m, m.agentfiles.Init()
	case dotfilesScreen, menuScreen:
	}
	return m, m.dotfiles.Init()
}

// View 는 진입 메뉴나 고른 목록 화면을 그린다.
func (m Model) View() tea.View {
	switch m.screen {
	case dotfilesScreen:
		return m.dotfiles.View()
	case brewfilesScreen:
		return m.brewfiles.View()
	case agentfilesScreen:
		return m.agentfiles.View()
	case menuScreen:
	}

	st := ui.NewStyles(!m.light)
	cardWidth := menuCardWidth
	if m.width > 0 {
		cardWidth = min(m.width, ui.HeaderMaxWidth) - 2
	}
	lines := []string{" " + st.Heading.Render("무엇을 관리할까요?"), ""}
	numbers := make([]string, len(menuOptions))
	for i := range menuOptions {
		numbers[i] = strconv.Itoa(i + 1)
		for line := range strings.SplitSeq(m.renderMenuCard(st, i, cardWidth), "\n") {
			lines = append(lines, " "+line)
		}
	}

	width := st.PageWidth(m.width, "", lines...)
	footer := st.RenderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "종료")
	return tea.NewView(st.RenderPage(width, m.version, "", lines, footer))
}

// menuCardWidth 는 터미널 폭을 모를 때 쓰는 진입 메뉴 카드의 폭이다.
const menuCardWidth = 60

// renderMenuCard 는 진입 메뉴의 i번째 선택지를 둥근 테두리 카드로 그린다. width는 테두리를 넣은 카드 전체 폭이다.
// 첫 줄은 번호와 이름이고 둘째 줄은 설명이다. 커서가 있는 카드는 테두리와 이름을 강조색으로 칠한다.
//
//	╭──────────────────────────────────────────────╮
//	│ 1  dotfiles          연결됨 3/12 ████░░░░░░░ │
//	│    설정 파일을 이 컴퓨터에 연결하거나 끊어요 │
//	╰──────────────────────────────────────────────╯
func (m Model) renderMenuCard(st ui.Styles, i, width int) string {
	option := menuOptions[i]
	border, number, label := st.Muted, st.Key, st.Key
	if i == m.choice {
		border, number, label = st.Cursor, st.Cursor, st.Title
	}

	// 테두리 두 칸과 양옆 여백 두 칸을 뺀 안쪽 폭이다.
	inner := max(width-4, 1)
	title := number.Render(strconv.Itoa(i+1)) + "  " + label.Render(option.label)
	if summary := m.renderMenuSummary(st, option.screen); summary != "" {
		if gap := inner - lipgloss.Width(title) - lipgloss.Width(summary); gap >= 2 {
			title += strings.Repeat(" ", gap) + summary
		} else {
			title += "\n   " + summary
		}
	}
	// 설명은 이름 칸에 맞춰 들여 쓰고, 좁은 터미널에서 줄이 바뀌어도 들여쓰기를 지킨다.
	body := title
	for line := range strings.SplitSeq(ansi.Wrap(option.description, max(inner-3, 1), " "), "\n") {
		body += "\n   " + st.Muted.Render(line)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border.GetForeground()).
		Padding(0, 1).
		Width(width).
		Render(body)
}

// renderMenuSummary 는 카드 첫 줄 오른쪽에 붙이는 목록 요약이다. 목록을 불러온 뒤에만 연결되거나 설치된 비율을 보여 준다.
func (m Model) renderMenuSummary(st ui.Styles, target screen) string {
	switch target {
	case dotfilesScreen:
		return m.dotfiles.Summary(st)
	case brewfilesScreen:
		return m.brewfiles.Summary(st)
	case agentfilesScreen:
		return m.agentfiles.Summary(st)
	case menuScreen:
	}
	return ""
}
