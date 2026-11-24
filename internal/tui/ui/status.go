package ui

// StatusTone 은 상태 메시지가 어떤 결과인지 나타낸다. 표시는 목록의 체크박스와 같은 뜻으로 쓴다.
type StatusTone int

const (
	StatusInfo     StatusTone = iota // 표시 없이 글만 보여 준다
	StatusDone                       // [+] 연결하거나 사본을 받은 일
	StatusUnlinked                   // [ ] 연결을 끊은 일
	StatusFailed                     // [-] 하지 못한 일
	StatusBusy                       // 스피너. 받거나 비교하는 중
)

// StatusKeyLimit 은 알림을 띄운 뒤 목록에서 키를 몇 번 누르면 알림을 지울지다.
// 결과를 읽을 틈은 주되 다른 일을 이어서 하는 동안 지난 결과가 계속 남아 있지 않게 한다.
const StatusKeyLimit = 3

// StatusLine 은 목록 화면 맨 아래 상태 줄에 띄운 알림이다. 목록 모델에 넣어 두고 필드를 그대로 읽는다.
type StatusLine struct {
	Status          string     // 마지막 결과나 안내. 비어 있으면 상태 줄을 칠하지 않는다
	StatusTone      StatusTone // Status가 어떤 결과인지. 상태 줄 앞의 표시를 고른다
	keysSinceStatus int        // Status를 띄운 뒤 목록에서 누른 키 수. StatusKeyLimit에 닿으면 Status를 지운다
}

// NewStatusLine 은 text를 띄우고 키 수를 처음부터 다시 세는 상태 줄이다. text가 비어 있으면 빈 상태 줄이다.
func NewStatusLine(tone StatusTone, text string) StatusLine {
	return StatusLine{Status: text, StatusTone: tone}
}

// CountKey 는 목록에서 누른 키 하나를 센 상태 줄이다. [StatusKeyLimit] 에 닿으면 알림을 지운다.
// 받거나 비교하는 중이라는 알림은 끝나면 결과로 바뀌므로 세지 않는다.
// 이 키가 새 알림을 띄우면 [NewStatusLine] 으로 만든 상태 줄이 수를 처음부터 다시 센다.
func (s StatusLine) CountKey() StatusLine {
	if s.Status == "" || s.StatusTone == StatusBusy {
		return s
	}
	s.keysSinceStatus++
	if s.keysSinceStatus >= StatusKeyLimit {
		return NewStatusLine(StatusInfo, "")
	}
	return s
}

// ToneMarker 는 tone에 맞는 상태 줄 표시와 그 색을 고를 상태다. busy는 진행 중일 때 쓸 표시다.
func ToneMarker(tone StatusTone, busy string) (marker string, color ItemState) {
	switch tone {
	case StatusDone:
		return Synced.Marker(), Synced
	case StatusUnlinked:
		return NotSynced.Marker(), NotSynced
	case StatusFailed:
		return Errored.Marker(), Errored
	case StatusBusy:
		return busy, Syncing
	case StatusInfo:
		// 결과가 아니라 알림이라 표시 없이 글만 보여 준다.
	}
	return "", NotSynced
}
