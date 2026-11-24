package dotfiles

import (
	"slices"

	"github.com/4anghyeon/itsmydot/internal/link"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// 그룹은 자기 파일을 가진 항목이고 하위 항목은 그룹 파일이 불러오는 파일이다.
// 그래서 하위 항목이 켜져 있으면 그룹 파일도 켜져 있어야 한다.
// 하위 항목을 켤 때 그룹 파일을 함께 켜고 그룹 파일을 끌 때 하위 항목도 함께 끈다.
// 폴더 그룹은 자기 파일 없이 하위 항목만 묶으므로 이 규칙이 없고 하위 항목을 하나씩 켜고 끈다.

// indexOf 는 이름이 name인 항목의 위치다. 없으면 -1이다.
func (m List) indexOf(name string) int {
	return slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == name })
}

func (it item) NodeName() string   { return it.entry.Name }
func (it item) ParentName() string { return it.parent }

// isFolder 는 i번째 항목이 자기 파일 없이 하위 항목만 묶는 폴더 그룹인지 알려 준다.
func (m List) isFolder(i int) bool {
	return m.items[i].entry.IsFolder()
}

// folderState 는 i번째 폴더 그룹 줄에 그릴 상태다. 받는 중인 하위 항목이 있으면 받는 중이고
// 하위 항목이 모두 연결돼 있으면 연결됨, 그 밖에는 연결 안 됨이다.
func (m List) folderState(i int) ui.ItemState {
	children := m.children(i)
	synced := 0
	for _, j := range children {
		switch m.items[j].state {
		case ui.Syncing:
			return ui.Syncing
		case ui.Synced:
			synced++
		case ui.NotSynced, ui.Conflict, ui.Errored:
		}
	}
	if len(children) > 0 && synced == len(children) {
		return ui.Synced
	}
	return ui.NotSynced
}

// notifyFolder 는 폴더 그룹 줄에서 연결이나 미리보기를 고르면 하위 항목을 고르라고 알린다.
func (m List) notifyFolder() List {
	return m.notify(ui.StatusInfo, m.items[m.cursor].entry.Name+" 폴더에는 연결할 파일이 없어요. 하위 항목을 골라 주세요")
}

// children 은 i번째 항목의 하위 항목 위치들이다. 그룹이 아니면 비어 있다.
func (m List) children(i int) []int {
	return ui.TreeChildren(m.items, i)
}

// syncedChildren 은 i번째 그룹의 하위 항목 중 연결된 것의 위치들이다.
func (m List) syncedChildren(i int) []int {
	var indexes []int
	for _, j := range m.children(i) {
		if m.items[j].state == ui.Synced {
			indexes = append(indexes, j)
		}
	}
	return indexes
}

// outdatedChildren 은 i번째 그룹에서 업데이트가 있는 하위 항목 수를 센다.
// 그룹을 접으면 하위 항목 줄이 안 보이니 그룹 줄에 이 수를 대신 보여 준다.
func (m List) outdatedChildren(i int) int {
	count := 0
	for _, j := range m.syncedChildren(i) {
		if m.items[j].outdated {
			count++
		}
	}
	return count
}

// visible 은 i번째 항목이 목록에 보이는지 알려 준다. 접힌 그룹의 하위 항목만 숨는다.
func (m List) visible(i int) bool {
	return ui.TreeVisible(m.items, m.collapsed, i)
}

// collapse 는 커서가 있는 그룹을 접는다. 하위 항목에서 누르면 그 그룹을 접고 커서를 그룹 줄로 옮긴다.
func (m List) collapse() List {
	m.cursor, m.collapsed = ui.TreeCollapse(m.items, m.collapsed, m.cursor)
	return m
}

// expand 는 커서가 있는 그룹을 펼친다.
func (m List) expand() List {
	m.collapsed = ui.TreeExpand(m.items, m.collapsed, m.cursor)
	return m
}

// unlinkChildren 은 i번째 그룹의 연결된 하위 항목을 모두 끄고 끈 개수를 돌려준다.
// 하위 항목은 그룹 파일 없이는 쓰이지 않으므로 백업이 없으면 묻지 않고 symlink만 지운다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m List) unlinkChildren(i int) (List, int) {
	count := 0
	for _, j := range m.syncedChildren(i) {
		child := &m.items[j]
		child.flash = 1
		restored, err := link.Unlink(child.entry.Target, child.copyPath)
		if err == nil {
			_, err = removeLoadLine(child.entry, m.home)
		}
		if err != nil {
			child.state, child.err = ui.Errored, err
			continue
		}
		child.state, child.err = ui.NotSynced, nil
		if restored != "" {
			child.state = ui.Conflict
		}
		count++
	}
	return m, count
}
