package tui

// entriesLoadedMsg 는 manifest를 불러와 각 항목의 상태까지 확인한 결과다.
type entriesLoadedMsg struct {
	items []item
	err   error
}

// entrySyncedMsg 는 항목 하나를 받아서 연결하는 작업(syncEntry)의 결과다.
type entrySyncedMsg struct {
	name   string
	backup string // 기존 파일을 옮겨 둔 백업 경로. 백업하지 않았으면 빈 문자열
	err    error
}
