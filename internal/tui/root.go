// Package tui 는 itsmydot의 터미널 화면이다. 진입 메뉴에서 dotfiles나 brewfiles를 고르면 그 목록 화면으로 들어간다.
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
)

// screen 은 지금 보이는 화면이다.
type screen int

const (
	menuScreen screen = iota
	dotfilesScreen
	brewfilesScreen
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
}

// backToMenuMsg 는 목록에서 esc를 눌러 진입 메뉴로 돌아가라는 메시지다.
type backToMenuMsg struct{}

func backToMenu() tea.Msg { return backToMenuMsg{} }

// Model 은 진입 메뉴와 두 목록 화면을 오가는 루트 모델이다. 메뉴와 화면 전환만 맡고 목록의 일은 각 목록 모델에 넘긴다.
type Model struct {
	dotfiles  dotfileList
	brewfiles brewfileList

	screen screen
	choice int // 진입 메뉴에서 커서가 있는 선택지. 목록에서 돌아오면 마지막에 고른 메뉴에 그대로 있다

	// dotfilesStarted 는 dotfiles 목록의 manifest 불러오기를 시작했으면 true다. 다시 들어가도 새로 받지 않는다.
	dotfilesStarted bool

	version string
	width   int  // 터미널 폭. 모르면 0이다
	light   bool // 터미널 배경이 밝으면 true
}

// New 는 진입 메뉴부터 보여 주는 처음 화면 상태를 만든다. version은 제목 옆에 보여 줄 버전이다.
func New(client *github.Client, copies *localcopy.Dir, version string) Model {
	return Model{
		dotfiles:  newDotfileList(client, copies, version),
		brewfiles: brewfileList{version: version},
		version:   version,
	}
}

// Init 은 색을 고르기 위해 터미널 배경색을 물어본다. manifest는 dotfiles 메뉴에 들어갈 때 받는다.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update 는 키 입력을 지금 보이는 화면에 넘기고 그 밖의 메시지는 두 목록에 모두 넘긴다.
// 진입 메뉴로 돌아간 뒤에도 받는 중이던 작업의 결과가 목록에 반영돼야 하기 때문이다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.screen {
		case dotfilesScreen:
			next, cmd := m.dotfiles.Update(msg)
			m.dotfiles = next.(dotfileList)
			return m, cmd
		case brewfilesScreen:
			next, cmd := m.brewfiles.Update(msg)
			m.brewfiles = next.(brewfileList)
			return m, cmd
		case menuScreen:
			return m.handleMenuKey(normalizeKey(msg))
		}
	case backToMenuMsg:
		m.screen = menuScreen
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
	}

	next, dotfilesCmd := m.dotfiles.Update(msg)
	m.dotfiles = next.(dotfileList)
	next, brewfilesCmd := m.brewfiles.Update(msg)
	m.brewfiles = next.(brewfileList)
	return m, tea.Batch(dotfilesCmd, brewfilesCmd)
}

// handleMenuKey 는 진입 메뉴의 키 입력을 처리한다. 고른 메뉴의 목록 화면으로 들어가고 esc는 종료다.
func (m Model) handleMenuKey(key string) (Model, tea.Cmd) {
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	}
	var picked int
	m.choice, picked = moveChoice(key, m.choice, len(menuOptions))
	if picked == 0 {
		return m, nil
	}
	m.choice = picked - 1
	m.screen = menuOptions[m.choice].screen
	if m.screen == dotfilesScreen && !m.dotfilesStarted {
		m.dotfilesStarted = true
		return m, m.dotfiles.Init()
	}
	return m, nil
}

// View 는 진입 메뉴나 고른 목록 화면을 그린다.
func (m Model) View() tea.View {
	switch m.screen {
	case dotfilesScreen:
		return m.dotfiles.View()
	case brewfilesScreen:
		return m.brewfiles.View()
	case menuScreen:
	}

	st := newStyles(!m.light)
	labels := make([]string, len(menuOptions))
	for i, option := range menuOptions {
		labels[i] = option.label
	}
	options, numbers := st.renderOptions(labels, m.choice)
	lines := append([]string{" " + st.heading.Render("무엇을 관리할까요?"), ""}, options...)
	description := " " + st.renderDescription(menuOptions[m.choice].description, m.width)

	width := st.pageWidth(m.width, "", append(lines, description)...)
	lines = append(lines, "", st.renderRule(width), description)
	footer := st.renderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "종료")
	return tea.NewView(st.renderPage(width, m.version, "", lines, footer))
}
