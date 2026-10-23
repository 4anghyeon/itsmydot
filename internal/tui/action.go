package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/link"
)

// toggle 은 커서가 있는 항목을 켜거나 끈다.
//
// 화면에 표시된 상태가 아니라 디스크를 다시 확인해서 결정한다.
// errored 항목도 실제로 연결돼 있으면 끄고, 아니면 다시 받아서 연결을 시도한다.
// 언제나 확인 질문을 먼저 띄운다.
func (m Model) toggle() (Model, tea.Cmd) {
	if !m.ready || len(m.items) == 0 {
		return m, nil
	}
	m.items = slices.Clone(m.items)
	it := &m.items[m.cursor]
	if it.state == syncing {
		return m, nil
	}

	state, err := link.Check(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m = m.notify(statusFailed, it.entry.Name+" 파일 상태를 확인하지 못했어요 ("+errorReason(err, m.home)+")")
		return m, nil
	}

	if state == link.Synced {
		// 되돌릴 백업이 없으면 target 자리가 비게 되므로 먼저 묻는다.
		// 처음부터 파일이 없었던 건지, 백업이 사고로 사라진 건지 디스크만으로는 구분할 수 없다.
		hasBackup, err := link.HasBackup(it.entry.Target)
		if err != nil {
			it.state, it.err = errored, err
			m = m.notify(statusFailed, it.entry.Name+" 백업을 확인하지 못했어요 ("+errorReason(err, m.home)+")")
			return m, nil
		}
		if !hasBackup {
			m.confirmingName, m.confirmKind, m.choice = it.entry.Name, confirmUnlinkWithoutBackup, 0
			return m, nil
		}
		if len(m.syncedChildren(m.cursor)) > 0 {
			m.confirmingName, m.confirmKind, m.choice = it.entry.Name, confirmGroupUnlink, 0
			return m, nil
		}
		m.confirmingName, m.confirmKind, m.choice = it.entry.Name, confirmTurnOff, 0
		return m, nil
	}

	return m.turnOn(m.cursor, state)
}

// turnOn 은 i번째 항목을 켠다. 하위 항목이면 꺼져 있는 그룹 파일도 함께 켠다.
// state는 i번째 항목의 디스크 상태다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) turnOn(i int, state link.State) (Model, tea.Cmd) {
	indexes, states := []int{i}, []link.State{state}
	if parent := m.items[i].parent; parent != "" {
		groupIndex := m.indexOf(parent)
		group := &m.items[groupIndex]
		if group.state != syncing {
			groupState, err := link.Check(group.entry.Target, group.copyPath)
			if err != nil {
				group.state, group.err = errored, err
				m = m.notify(statusFailed, group.entry.Name+" 파일 상태를 확인하지 못했어요 ("+errorReason(err, m.home)+")")
				return m, nil
			}
			if groupState != link.Synced {
				indexes, states = []int{groupIndex, i}, []link.State{groupState, state}
			}
		}
	}

	// 다른 파일이 있으면 백업이 일어나므로 백업 질문으로 묻는다. 백업은 건너뛸 수 없고 진행할지만 고른다.
	// 그룹 파일과 하위 파일이 둘 다 걸려도 질문은 한 번만 한다. 걸린 파일이 없으면 켤지만 묻는다.
	conflicted := false
	for k, j := range indexes {
		if states[k] == link.Conflict {
			m.items[j].state, m.items[j].err = conflict, nil
			conflicted = true
		}
	}
	m.pendingNames = make([]string, len(indexes))
	for k, j := range indexes {
		m.pendingNames[k] = m.items[j].entry.Name
	}
	m.confirmingName, m.confirmKind, m.choice = m.items[i].entry.Name, confirmTurnOn, 0
	if conflicted {
		m.confirmKind = confirmBackupLink
	}
	return m, nil
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
		m = m.notify(statusFailed, it.entry.Name+" 연결을 끊지 못했어요 ("+errorReason(err, m.home)+")")
		return m.withChildrenOff(childrenOff)
	}
	if restored != "" {
		// 백업을 되돌렸으면 target 자리에 다시 일반 파일이 있다.
		it.state, it.err = conflict, nil
		m = m.notify(statusUnlinked, it.entry.Name+" 연결을 끊고 원본 파일을 되돌렸어요")
	} else {
		it.state, it.err = notSynced, nil
		m = m.notify(statusUnlinked, it.entry.Name+" 연결을 끊었어요. 사본은 "+tildify(it.copyPath, m.home)+" 에 남아 있어요")
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
		m = m.notify(statusFailed, it.entry.Name+" 연결을 끊지 못했어요 ("+errorReason(err, m.home)+")")
		return m.withChildrenOff(childrenOff)
	}
	// target 자리에 일반 파일이 남았으므로 디스크 기준으로는 conflict다.
	it.state, it.err = conflict, nil
	m = m.notify(statusUnlinked, it.entry.Name+" 연결을 끊었어요. 지금 설정은 "+tildify(it.entry.Target, m.home)+" 에 남겨 뒀어요")
	return m.withChildrenOff(childrenOff)
}

// withChildrenOff 는 그룹을 끌 때 함께 끈 하위 항목 수를 상태 메시지에 덧붙인다.
func (m Model) withChildrenOff(count int) Model {
	if count > 0 {
		m.status += fmt.Sprintf(". 하위 항목 %d개도 함께 연결을 끊었어요", count)
	}
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
		m = m.notify(statusFailed, it.entry.Name+" 파일 상태를 확인하지 못했어요 ("+errorReason(err, m.home)+")")
		return m, nil
	}
	if state != link.Synced {
		it.state, it.err = fromLinkState(state), nil
		m = m.notify(statusInfo, it.entry.Name+" 연결이 풀려 있어서 받지 않았어요")
		return m, nil
	}

	it.state, it.err = syncing, nil
	m = m.notify(statusBusy, it.entry.Name+" 최신본을 받는 중이에요...")
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
	m = m.notify(statusBusy, strings.Join(names, ", ")+" 받는 중이에요...")
	return m, tea.Batch(append(cmds, m.spinner.Tick)...)
}

// syncResult 는 받기가 끝난 항목 하나의 결과 문구다. index는 목록에서의 위치로 정렬에 쓴다.
type syncResult struct {
	index  int
	text   string
	failed bool
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
	failed := msg.err != nil
	switch {
	case msg.err != nil && msg.copyUpdated:
		// 받기에 실패해도 연결과 이전 사본은 그대로라 연결된 상태로 둔다.
		it.state, it.err = synced, nil
		result = it.entry.Name + " 최신본을 받지 못해서 이전 사본을 그대로 써요 (" + errorReason(msg.err, m.home) + ")"
	case msg.err != nil:
		it.state, it.err = errored, msg.err
		result = it.entry.Name + " 파일을 연결하지 못했어요 (" + errorReason(msg.err, m.home) + ")"
	case msg.copyUpdated:
		it.state, it.err, it.outdated = synced, nil, false
		result = it.entry.Name + " 사본을 최신으로 바꿨어요"
	default:
		it.state, it.err, it.outdated = synced, nil, false
		result = it.entry.Name + " 파일을 연결했어요"
		if msg.backup != "" {
			result += ". 기존 파일은 " + tildify(msg.backup, m.home) + " 에 백업했어요"
		}
	}

	m.syncResults = append(slices.Clone(m.syncResults), syncResult{index: i, text: result, failed: failed})
	if slices.ContainsFunc(m.items, func(it item) bool { return it.state == syncing }) {
		return m
	}
	slices.SortFunc(m.syncResults, func(a, b syncResult) int { return a.index - b.index })
	texts := make([]string, len(m.syncResults))
	tone := statusDone
	for k, r := range m.syncResults {
		texts[k] = r.text
		if r.failed {
			tone = statusFailed
		}
	}
	m = m.notify(tone, strings.Join(texts, " / "))
	m.syncResults = nil
	return m
}
