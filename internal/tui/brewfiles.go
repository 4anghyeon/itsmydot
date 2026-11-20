package tui

import (
	tea "charm.land/bubbletea/v2"
)

// brewfileList 는 brewfiles 메뉴의 목록 화면이다.
type brewfileList struct {
	version string
	width   int  // 터미널 폭. 모르면 0이다
	light   bool // 터미널 배경이 밝으면 true
}

// Update 는 터미널 크기와 배경색을 기억하고 esc를 누르면 진입 메뉴로 돌아간다.
func (m brewfileList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
	case tea.KeyPressMsg:
		switch normalizeKey(msg) {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, backToMenu
		}
	}
	return m, nil
}

// Init 은 할 일이 없다. [tea.Model] 을 채우려고 둔다.
func (m brewfileList) Init() tea.Cmd { return nil }

// View 는 brewfiles 목록 자리에 안내 한 줄을 그린다.
func (m brewfileList) View() tea.View {
	st := newStyles(!m.light)
	lines := []string{" " + st.muted.Render("brewfiles 목록은 준비 중이에요")}
	footer := st.renderHelp(m.width, "esc", "진입 메뉴")
	return tea.NewView(st.renderPage(st.pageWidth(m.width, "", lines...), m.version, "", lines, footer))
}
