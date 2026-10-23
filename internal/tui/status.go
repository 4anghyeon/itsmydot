package tui

// statusTone 은 상태 메시지가 어떤 결과인지 나타낸다. 표시는 목록의 체크박스와 같은 뜻으로 쓴다.
type statusTone int

const (
	statusInfo     statusTone = iota // 표시 없이 글만 보여 준다
	statusDone                       // [+] 연결하거나 사본을 받은 일
	statusUnlinked                   // [ ] 연결을 끊은 일
	statusFailed                     // [-] 하지 못한 일
	statusBusy                       // 스피너. 받거나 비교하는 중
)

// statusKeyLimit 은 알림을 띄운 뒤 목록에서 키를 몇 번 누르면 알림을 지울지다.
// 결과를 읽을 틈은 주되 다른 일을 이어서 하는 동안 지난 결과가 계속 남아 있지 않게 한다.
const statusKeyLimit = 3

// notify 는 상태 줄에 text를 띄우고 키 수를 처음부터 다시 센 Model을 돌려준다. text가 비어 있으면 상태 줄을 비운다.
func (m Model) notify(tone statusTone, text string) Model {
	m.status, m.statusTone, m.keysSinceStatus = text, tone, 0
	return m
}

// countStatusKey 는 목록에서 누른 키를 세다가 statusKeyLimit에 닿으면 알림을 지운다.
// 받거나 비교하는 중이라는 알림은 끝나면 결과로 바뀌므로 세지 않는다.
// 이 키가 새 알림을 띄우면 notify가 수를 다시 센다.
func (m Model) countStatusKey() Model {
	if m.status != "" && m.statusTone != statusBusy {
		m.keysSinceStatus++
		if m.keysSinceStatus >= statusKeyLimit {
			m = m.notify(statusInfo, "")
		}
	}
	return m
}

// statusMarker 는 상태 줄 앞에 붙일 표시와 그 색을 고를 상태다. 목록의 체크박스와 같은 표시를 쓴다.
func (m Model) statusMarker() (marker string, tone itemState) {
	switch m.statusTone {
	case statusDone:
		return synced.marker(), synced
	case statusUnlinked:
		return notSynced.marker(), notSynced
	case statusFailed:
		return errored.marker(), errored
	case statusBusy:
		return m.renderMarker(syncing), syncing
	case statusInfo:
		// 결과가 아니라 알림이라 표시 없이 글만 보여 준다.
	}
	return "", notSynced
}
