package ui

import tea "charm.land/bubbletea/v2"

// BackToMenuMsg 는 목록에서 esc를 눌러 진입 메뉴로 돌아가라는 메시지다.
type BackToMenuMsg struct{}

// BackToMenu 는 [BackToMenuMsg] 를 보내는 명령이다.
func BackToMenu() tea.Msg { return BackToMenuMsg{} }
