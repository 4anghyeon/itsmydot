package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// handleHelpKey 는 도움말 화면의 키 입력을 처리한다. ?나 esc, q로 닫으면 목록으로 돌아가고 다른 키는 무시한다.
func (m dotfileList) handleHelpKey(key string) (dotfileList, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "?", "esc", "q":
		m.helpOpen = false
	}
	return m, nil
}

// helpView 는 목록 대신 전체 키와 상태 표시의 뜻, 로컬 사본이 놓이는 곳을 보여 준다.
func (m dotfileList) helpView(st ui.Styles) tea.View {
	pad := func(text string, width int) string {
		return text + strings.Repeat(" ", max(width-lipgloss.Width(text), 0))
	}
	rows := func(pairs [][2]string) []string {
		width := 0
		for _, p := range pairs {
			width = max(width, lipgloss.Width(p[0]))
		}
		lines := make([]string, len(pairs))
		for i, p := range pairs {
			lines[i] = "  " + pad(p[0], width) + "  " + p[1]
		}
		return lines
	}

	body := []string{" " + st.Heading.Render("키")}
	body = append(body, rows([][2]string{
		{st.Key.Render("방향키"), "항목을 옮기고 그룹을 접거나 펼쳐요"},
		{st.Key.Render("space"), "항목을 연결하거나 연결을 끊어요"},
		{st.Key.Render("enter"), "항목 메뉴를 열어요"},
		{st.Key.Render("tab"), "항목 파일 내용을 미리 봐요"},
		{st.Key.Render("esc"), "화면을 닫거나 진입 메뉴로 돌아가요"},
	})...)

	markers := [][2]string{
		{st.StateStyle(ui.Synced).Render(ui.Synced.Marker()), "연결됨"},
		{st.StateStyle(ui.Conflict).Render(ui.Conflict.Marker()), "다른 파일이 있음"},
		{st.StateStyle(ui.NotSynced).Render(ui.NotSynced.Marker()), "연결 안 됨"},
		{st.StateStyle(ui.Errored).Render(ui.Errored.Marker()), "확인하거나 받지 못함"},
		{st.StateStyle(ui.Syncing).Render("[o]"), "받는 중"},
		{st.Update.Render("^  "), "저장소에 업데이트 있음"},
		{st.Key.Render("v  "), "펼친 그룹"},
		{st.Key.Render(">  "), "접힌 그룹"},
	}
	cells := make([]string, len(markers))
	for i, p := range markers {
		cells[i] = p[0] + " " + p[1]
	}
	left := 0
	for i := 0; i < len(cells); i += 2 {
		left = max(left, lipgloss.Width(cells[i]))
	}
	body = append(body, "", " "+st.Heading.Render("동기화 상태"))
	for i := 0; i < len(cells); i += 2 {
		line := "  " + pad(cells[i], left)
		if i+1 < len(cells) {
			line += "    " + cells[i+1]
		}
		body = append(body, line)
	}

	body = append(body, "", " "+st.Heading.Render("파일 경로"))
	body = append(body, "  "+st.Key.Render("사본 저장 경로:")+" ~/"+localcopy.DirName)

	right := st.Muted.Render("도움말")
	footer := st.RenderHelp(m.width, "?/esc", "닫기")
	return tea.NewView(st.RenderPage(st.PageWidth(m.width, right, body...), m.version, right, body, footer))
}
