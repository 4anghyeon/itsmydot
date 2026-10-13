package tui

// entriesLoadedMsg 는 manifest를 불러와 각 항목의 상태까지 확인한 결과다.
type entriesLoadedMsg struct {
	items []item
	err   error
}
