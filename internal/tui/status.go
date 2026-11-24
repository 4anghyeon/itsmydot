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

// statusLine 은 목록 화면 맨 아래 상태 줄에 띄운 알림이다. 목록 모델에 넣어 두고 필드를 그대로 읽는다.
type statusLine struct {
	status          string     // 마지막 결과나 안내. 비어 있으면 상태 줄을 칠하지 않는다
	statusTone      statusTone // status가 어떤 결과인지. 상태 줄 앞의 표시를 고른다
	keysSinceStatus int        // status를 띄운 뒤 목록에서 누른 키 수. statusKeyLimit에 닿으면 status를 지운다
}

// newStatusLine 은 text를 띄우고 키 수를 처음부터 다시 세는 상태 줄이다. text가 비어 있으면 빈 상태 줄이다.
func newStatusLine(tone statusTone, text string) statusLine {
	return statusLine{status: text, statusTone: tone}
}

// countKey 는 목록에서 누른 키 하나를 센 상태 줄이다. statusKeyLimit에 닿으면 알림을 지운다.
// 받거나 비교하는 중이라는 알림은 끝나면 결과로 바뀌므로 세지 않는다.
// 이 키가 새 알림을 띄우면 notify가 수를 다시 센다.
func (s statusLine) countKey() statusLine {
	if s.status == "" || s.statusTone == statusBusy {
		return s
	}
	s.keysSinceStatus++
	if s.keysSinceStatus >= statusKeyLimit {
		return newStatusLine(statusInfo, "")
	}
	return s
}

// notify 는 상태 줄에 text를 띄운 목록 모델을 돌려준다. text가 비어 있으면 상태 줄을 비운다.
func (m dotfileList) notify(tone statusTone, text string) dotfileList {
	m.statusLine = newStatusLine(tone, text)
	return m
}

// countStatusKey 는 목록에서 누른 키를 상태 줄에 센 목록 모델을 돌려준다.
func (m dotfileList) countStatusKey() dotfileList {
	m.statusLine = m.statusLine.countKey()
	return m
}

// toneMarker 는 tone에 맞는 상태 줄 표시와 그 색을 고를 상태다. busy는 진행 중일 때 쓸 표시다.
func toneMarker(tone statusTone, busy string) (marker string, color itemState) {
	switch tone {
	case statusDone:
		return synced.marker(), synced
	case statusUnlinked:
		return notSynced.marker(), notSynced
	case statusFailed:
		return errored.marker(), errored
	case statusBusy:
		return busy, syncing
	case statusInfo:
		// 결과가 아니라 알림이라 표시 없이 글만 보여 준다.
	}
	return "", notSynced
}
