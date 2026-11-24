package tui

// itemState 는 화면에 보여 주는 항목 하나의 상태다.
// 디스크 상태인 [link.State] 에 화면에서만 쓰는 syncing과 errored를 더했다.
type itemState int

const (
	notSynced itemState = iota // [ ] symlink 없음
	synced                     // [+] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [~] GitHub에서 받는 중. 화면에서는 ~ 대신 스피너가 돈다
	conflict                   // [!] target 자리에 다른 파일이 있음
	errored                    // [-] 상태 확인이나 동기화 실패
)

// marker 는 상태를 체크박스 모양으로 바꾼다.
// 체크 표시 같은 기호는 폰트에 없으면 다른 폰트로 대신 그리는데 이때 두 칸을 차지해 정렬이 깨진다. 그래서 ASCII만 쓴다.
func (s itemState) marker() string {
	switch s {
	case synced:
		return "[+]"
	case syncing:
		return "[~]"
	case conflict:
		return "[!]"
	case errored:
		return "[-]"
	default:
		return "[ ]"
	}
}
