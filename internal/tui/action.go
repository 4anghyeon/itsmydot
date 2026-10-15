package tui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

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
// target 자리에 다른 파일이 있거나 켜진 하위 항목까지 함께 꺼지는 경우에는 먼저 확인 질문을 띄운다.
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
		if len(m.syncedChildren(m.cursor)) > 0 {
			m.confirming, m.confirmKind, m.choice = it.entry.Name, confirmGroupUnlink, 0
			return m, nil
		}
		return m.unlink(m.cursor).startAnimation()
	}

	return m.turnOn(m.cursor, state)
}

// turnOn 은 i번째 항목을 켠다. 하위 항목이면 꺼져 있는 그룹 파일도 함께 켠다.
// state는 i번째 항목의 디스크 상태다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) turnOn(i int, state link.State) (Model, tea.Cmd) {
	indexes, states := []int{i}, []link.State{state}
	if parent := m.items[i].parent; parent != "" {
		p := m.indexOf(parent)
		group := &m.items[p]
		if group.state != syncing {
			groupState, err := link.Check(group.entry.Target, group.copyPath)
			if err != nil {
				group.state, group.err = errored, err
				m.status = group.entry.Name + ": 상태 확인 실패"
				return m, nil
			}
			if groupState != link.Synced {
				indexes, states = []int{p, i}, []link.State{groupState, state}
			}
		}
	}

	// 다른 파일이 있으면 백업이 일어나므로 먼저 묻는다. 백업은 건너뛸 수 없고 진행할지만 고른다.
	// 그룹 파일과 하위 파일이 둘 다 걸려도 질문은 한 번만 한다.
	conflicted := false
	for k, j := range indexes {
		if states[k] == link.Conflict {
			m.items[j].state, m.items[j].err = conflict, nil
			conflicted = true
		}
	}
	if conflicted {
		m.pending = make([]string, len(indexes))
		for k, j := range indexes {
			m.pending[k] = m.items[j].entry.Name
		}
		m.confirming, m.confirmKind, m.choice = m.items[i].entry.Name, confirmLink, 0
		return m, nil
	}
	return m.startSync(indexes...)
}

// unlink 는 i번째 항목의 연결을 끊고, 백업이 있으면 원래 파일을 되돌린다.
// 그룹이면 연결된 하위 항목도 함께 끈다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) unlink(i int) Model {
	m, childrenOff := m.unlinkChildren(i)
	it := &m.items[i]
	it.flash = 1
	restored, err := link.Unlink(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m.withChildrenOff(childrenOff)
	}
	if restored != "" {
		// 백업을 되돌렸으면 target 자리에 다시 일반 파일이 있다.
		it.state, it.err = conflict, nil
		m.status = it.entry.Name + ": 해제됨 · 원래 파일을 되돌림"
	} else {
		it.state, it.err = notSynced, nil
		m.status = it.entry.Name + ": 연결 끊음 · 내용은 " + tildify(it.copyPath, m.home) + " 에 남아 있음"
	}
	return m.withChildrenOff(childrenOff)
}

// detach 는 i번째 항목의 연결만 끊고, 지금 설정은 target에 일반 파일로 남긴다.
// 그룹이면 연결된 하위 항목도 함께 끈다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) detach(i int) Model {
	m, childrenOff := m.unlinkChildren(i)
	it := &m.items[i]
	it.flash = 1
	if err := link.Detach(it.entry.Target, it.copyPath); err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m.withChildrenOff(childrenOff)
	}
	// target 자리에 일반 파일이 남았으므로 디스크 기준으로는 conflict다.
	it.state, it.err = conflict, nil
	m.status = it.entry.Name + ": 연결 끊음 · 지금 설정은 " + tildify(it.entry.Target, m.home) + " 에 남겨 둠"
	return m.withChildrenOff(childrenOff)
}

// withChildrenOff 는 그룹을 끌 때 함께 끈 하위 항목 수를 상태 메시지에 덧붙인다.
func (m Model) withChildrenOff(count int) Model {
	if count > 0 {
		m.status += fmt.Sprintf(" · 하위 항목 %d개도 끔", count)
	}
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

	name, pending := m.confirming, m.pending
	m.confirming, m.pending = "", nil
	if key == cancel {
		// 메뉴는 아무것도 하지 않고 닫는 것이라 취소를 알릴 필요가 없다.
		if m.confirmKind != confirmAction {
			m.status = name + ": 취소됨"
		}
		return m, nil
	}

	i := m.indexOf(name)
	if i < 0 {
		return m, nil
	}
	m.items = slices.Clone(m.items)

	switch m.confirmKind {
	case confirmLink: // 1. 백업 후 연결
		indexes := make([]int, 0, len(pending))
		for _, p := range pending {
			if j := m.indexOf(p); j >= 0 {
				indexes = append(indexes, j)
			}
		}
		return m.startSync(indexes...)
	case confirmUnlink:
		if n == 1 { // 1. 지금 설정은 남겨 두고 연결만 끊기
			return m.detach(i).startAnimation()
		}
		// 2. 연결을 끊고 파일도 지우기
		return m.unlink(i).startAnimation()
	case confirmGroupUnlink: // 1. 모두 끄기
		return m.unlink(i).startAnimation()
	case confirmAction:
		// 메뉴를 여는 동안 커서는 움직이지 않으므로 커서가 곧 메뉴를 연 항목이다.
		if n == 1 && m.items[i].state == synced { // 1. 최신으로 받기
			return m.updateCopy(i)
		}
		return m.toggle() // 켜기 또는 끄기
	}
	return m, nil
}

// openMenu 는 커서가 있는 항목에서 할 수 있는 일을 번호 메뉴로 띄운다. 받는 중인 항목에서는 열지 않는다.
func (m Model) openMenu() Model {
	if !m.ready || len(m.items) == 0 || m.items[m.cursor].state == syncing {
		return m
	}
	m.confirming, m.confirmKind, m.choice = m.items[m.cursor].entry.Name, confirmAction, 0
	return m
}

// updateCopy 는 i번째 항목의 로컬 사본만 GitHub에서 다시 받는다. 연결은 건드리지 않는다.
// 메뉴를 연 뒤에 디스크가 바뀌었을 수 있어서 받기 전에 아직 연결돼 있는지 다시 본다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) updateCopy(i int) (Model, tea.Cmd) {
	it := &m.items[i]
	state, err := link.Check(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 상태 확인 실패"
		return m, nil
	}
	if state != link.Synced {
		it.state, it.err = fromLinkState(state), nil
		m.status = it.entry.Name + " 연결이 풀려 있어서 받지 않았어요"
		return m, nil
	}

	it.state, it.err = syncing, nil
	m.status = it.entry.Name + " 받는 중이에요..."
	entry, client, copies := it.entry, m.client, m.copies
	fetch := func() tea.Msg {
		return fetchCopy(context.Background(), client, copies, entry)
	}
	return m, tea.Batch(fetch, m.spinner.Tick)
}

// startSync 는 indexes의 항목들을 syncing으로 바꾸고, 받아서 연결하는 작업을 항목마다 백그라운드로 시작한다.
// 스피너도 함께 돌린다. 이미 돌고 있으면 스피너가 겹친 tick을 알아서 버린다.
// 끝나면 항목마다 entrySyncedMsg로 결과가 돌아온다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) startSync(indexes ...int) (Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(indexes)+1)
	names := make([]string, 0, len(indexes))
	for _, i := range indexes {
		it := &m.items[i]
		it.state, it.err = syncing, nil
		names = append(names, it.entry.Name)
		entry, client, copies := it.entry, m.client, m.copies
		cmds = append(cmds, func() tea.Msg {
			return syncEntry(context.Background(), client, copies, entry)
		})
	}
	m.status = strings.Join(names, ", ") + ": 받는 중..."
	return m, tea.Batch(append(cmds, m.spinner.Tick)...)
}

// applySynced 는 syncEntry의 결과를 해당 항목에 반영한다.
// 그룹 파일과 하위 파일을 함께 받으면 결과가 따로 도착한다. 먼저 온 결과의 백업 안내가 묻히지 않도록
// 모두 끝날 때까지 모아 두었다가 목록 순서대로 한 줄에 보여 준다.
func (m Model) applySynced(msg entrySyncedMsg) Model {
	i := m.indexOf(msg.name)
	if i < 0 {
		return m
	}
	m.items = slices.Clone(m.items)
	it := &m.items[i]
	it.flash = 1

	var result string
	switch {
	case msg.err != nil && msg.copyUpdated:
		// 받기에 실패해도 연결과 이전 사본은 그대로라 연결된 상태로 둔다.
		it.state, it.err = synced, nil
		result = it.entry.Name + " 최신본을 받지 못해서 이전 사본을 그대로 써요 (" + msg.err.Error() + ")"
	case msg.err != nil:
		it.state, it.err = errored, msg.err
		result = it.entry.Name + ": 연결 실패"
	case msg.copyUpdated:
		it.state, it.err = synced, nil
		result = it.entry.Name + " 사본을 최신으로 바꿨어요"
	default:
		it.state, it.err = synced, nil
		result = it.entry.Name + ": 연결됨"
		if msg.backup != "" {
			result += " · 기존 파일 백업: " + tildify(msg.backup, m.home)
		}
	}

	m.syncResults = append(slices.Clone(m.syncResults), syncResult{index: i, text: result})
	if slices.ContainsFunc(m.items, func(it item) bool { return it.state == syncing }) {
		return m
	}
	slices.SortFunc(m.syncResults, func(a, b syncResult) int { return a.index - b.index })
	texts := make([]string, len(m.syncResults))
	for k, r := range m.syncResults {
		texts[k] = r.text
	}
	m.status, m.syncResults = strings.Join(texts, " / "), nil
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

	// 하위 항목은 그룹 바로 뒤에 이어 붙여서 목록 순서가 화면 순서와 같게 한다.
	var items []item
	for _, e := range m.Entries {
		items = append(items, checkItem(e, "", copies))
		for _, child := range e.Children {
			items = append(items, checkItem(child, e.Name, copies))
		}
	}
	return entriesLoadedMsg{items: items}
}

// checkItem 은 항목 하나의 로컬 사본 경로와 지금 상태를 확인해 목록의 한 줄을 만든다.
// 확인에 실패해도 나머지 항목은 보여 주도록 그 항목만 errored로 표시한다.
func checkItem(e manifest.Entry, parent string, copies *localcopy.Dir) item {
	e.Children = nil
	it := item{entry: e, parent: parent}
	var err error
	it.copyPath, err = copies.Path(e.Source)
	if err == nil {
		var s link.State
		s, err = link.Check(e.Target, it.copyPath)
		it.state = fromLinkState(s)
	}
	if err != nil {
		it.state, it.err = errored, err
	}
	return it
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

// fetchCopy 는 연결된 항목의 최신본을 받아 로컬 사본만 바꾼다. symlink는 사본을 가리키고 있으므로 바로 반영된다.
func fetchCopy(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name, copyUpdated: true}
	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	_, msg.err = copies.Write(e.Source, data)
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
