package tui

import (
	"context"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/link"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// toggle 은 커서가 있는 항목을 켜거나 끈다.
//
// 화면에 표시된 상태가 아니라 디스크를 다시 확인해서 결정한다.
// errored 항목도 실제로 연결돼 있으면 끄고, 아니면 다시 받아서 연결을 시도한다.
// target 자리에 다른 파일이 있으면 바로 연결하지 않고 확인 질문을 띄운다.
func (m Model) toggle() (Model, tea.Cmd) {
	if !m.ready || len(m.items) == 0 {
		return m, nil
	}
	// m.items는 이전 Model과 같은 배열을 쓴다. 복사하지 않고 고치면 이전 Model까지 바뀐다.
	m.items = slices.Clone(m.items)
	it := &m.items[m.cursor]
	if it.state == syncing {
		return m, nil
	}

	state, err := link.Check(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 상태 확인 실패"
		return m, nil
	}

	// 끄는 건 네트워크가 필요 없어서 바로 처리한다.
	if state == link.Synced {
		// 되돌릴 백업이 없으면 target 자리가 비게 되므로 먼저 묻는다.
		// 처음부터 파일이 없었던 건지, 백업이 사고로 사라진 건지 디스크만으로는 구분할 수 없다.
		has, err := link.HasBackup(it.entry.Target)
		if err != nil {
			it.state, it.err = errored, err
			m.status = it.entry.Name + ": 백업 확인 실패"
			return m, nil
		}
		if !has {
			m.confirming, m.confirmKind, m.choice = it.entry.Name, confirmUnlink, 0
			return m, nil
		}
		return m.unlink(m.cursor).startAnimation()
	}

	// 다른 파일이 있으면 백업이 일어나므로 먼저 묻는다. 백업은 건너뛸 수 없고 진행할지만 고른다.
	if state == link.Conflict {
		it.state, it.err = conflict, nil
		m.confirming, m.confirmKind, m.choice = it.entry.Name, confirmLink, 0
		return m, nil
	}

	return m.startSync(m.cursor)
}

// unlink 는 i번째 항목의 연결을 끊고, 백업이 있으면 원래 파일을 되돌린다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) unlink(i int) Model {
	it := &m.items[i]
	it.flash = 1
	restored, err := link.Unlink(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m
	}
	it.state, it.err = notSynced, nil
	m.status = it.entry.Name + ": 해제됨"
	if restored != "" {
		// 백업을 되돌렸으면 target 자리에 다시 일반 파일이 있다.
		it.state = conflict
		m.status += " · 원래 파일을 되돌림"
	}
	return m
}

// detach 는 i번째 항목의 연결만 끊고, 지금 설정은 target에 일반 파일로 남긴다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) detach(i int) Model {
	it := &m.items[i]
	it.flash = 1
	if err := link.Detach(it.entry.Target, it.copyPath); err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m
	}
	// target 자리에 일반 파일이 남았으므로 디스크 기준으로는 conflict다.
	it.state, it.err = conflict, nil
	m.status = it.entry.Name + ": 연결 끊음 · 지금 설정은 " + tildify(it.entry.Target, m.home) + " 에 남겨 둠"
	return m
}

// answerConfirm 은 확인 질문에 대한 키 입력을 처리한다.
// 방향키로 커서를 옮겨 enter나 space로 고르거나 번호를 눌러 바로 고른다. esc는 취소다.
// y와 n 대신 숫자를 쓰는 건 한글 입력기가 자모를 조합 대기로 붙잡아서 늦게 도착하기 때문이다.
// 그 밖의 키는 무시해서 답하기 전에 다른 항목을 건드리지 않게 한다.
func (m Model) answerConfirm(key string) (Model, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	_, options := m.confirmPrompt()
	cancel := strconv.Itoa(len(options)) // 마지막 선택지는 항상 취소다
	switch key {
	case "up", "k":
		m.choice = max(m.choice-1, 0)
		return m, nil
	case "down", "j":
		m.choice = min(m.choice+1, len(options)-1)
		return m, nil
	case "enter", "space":
		key = strconv.Itoa(m.choice + 1)
	case "esc":
		key = cancel
	}
	n, err := strconv.Atoi(key)
	if err != nil || n < 1 || n > len(options) {
		return m, nil
	}

	name := m.confirming
	m.confirming = ""
	if key == cancel {
		m.status = name + ": 취소됨"
		return m, nil
	}

	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == name })
	if i < 0 {
		return m, nil
	}
	m.items = slices.Clone(m.items)

	switch m.confirmKind {
	case confirmLink: // 1. 백업 후 연결
		return m.startSync(i)
	case confirmUnlink:
		if n == 1 { // 1. 지금 설정은 남겨 두고 연결만 끊기
			return m.detach(i).startAnimation()
		}
		// 2. 연결을 끊고 파일도 지우기
		m = m.unlink(i)
		if m.items[i].state == notSynced {
			m.status = name + ": 연결 끊음 · 내용은 " + tildify(m.items[i].copyPath, m.home) + " 에 남아 있음"
		}
		return m.startAnimation()
	}
	return m, nil
}

// startSync 는 i번째 항목을 syncing으로 바꾸고, 받아서 연결하는 작업을 백그라운드로 시작한다.
// 스피너도 함께 돌린다. 이미 돌고 있으면 스피너가 겹친 tick을 알아서 버린다.
// 끝나면 entrySyncedMsg로 결과가 돌아온다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) startSync(i int) (Model, tea.Cmd) {
	it := &m.items[i]
	it.state, it.err = syncing, nil
	m.status = it.entry.Name + ": 받는 중..."
	entry, client, copies := it.entry, m.client, m.copies
	sync := func() tea.Msg {
		return syncEntry(context.Background(), client, copies, entry)
	}
	return m, tea.Batch(sync, m.spinner.Tick)
}

// applySynced 는 syncEntry의 결과를 해당 항목에 반영한다.
func (m Model) applySynced(msg entrySyncedMsg) Model {
	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == msg.name })
	if i < 0 {
		return m
	}
	m.items = slices.Clone(m.items)
	it := &m.items[i]
	it.flash = 1

	if msg.err != nil {
		it.state, it.err = errored, msg.err
		m.status = it.entry.Name + ": 연결 실패"
		return m
	}
	it.state, it.err = synced, nil
	m.status = it.entry.Name + ": 연결됨"
	if msg.backup != "" {
		m.status += " · 기존 파일 백업: " + tildify(msg.backup, m.home)
	}
	return m
}

// loadEntries 는 GitHub에서 manifest를 받아 파싱하고, 각 항목의 현재 상태를 확인한다.
// 디스크는 바꾸지 않는다. Init의 백그라운드 작업에서 호출된다.
func loadEntries(ctx context.Context, client *github.Client, copies *localcopy.Dir) entriesLoadedMsg {
	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return entriesLoadedMsg{err: err}
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return entriesLoadedMsg{err: err}
	}

	// 한 항목이 실패해도 나머지는 보여 주도록 그 항목만 errored로 표시한다.
	items := make([]item, 0, len(m.Entries))
	for _, e := range m.Entries {
		it := item{entry: e}
		it.copyPath, err = copies.Path(e.Source)
		if err == nil {
			var s link.State
			s, err = link.Check(e.Target, it.copyPath)
			it.state = fromLinkState(s)
		}
		if err != nil {
			it.state, it.err = errored, err
		}
		items = append(items, it)
	}
	return entriesLoadedMsg{items: items}
}

// syncEntry 는 항목 하나를 GitHub에서 최신본으로 받아 로컬 사본에 저장하고 target에 연결한다.
// 이미 연결된 항목이면 로컬 사본만 최신으로 바뀐다.
func syncEntry(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name}

	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	copyPath, err := copies.Write(e.Source, data)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.backup, msg.err = link.Link(e.Target, copyPath)
	return msg
}

// fromLinkState 는 link 패키지의 디스크 상태를 화면 상태로 바꾼다.
func fromLinkState(s link.State) itemState {
	switch s {
	case link.Synced:
		return synced
	case link.Conflict:
		return conflict
	default:
		return notSynced
	}
}
