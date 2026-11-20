package tui

import (
	"slices"

	"github.com/4anghyeon/itsmydot/internal/link"
)

// 그룹은 자기 파일을 가진 항목이고 하위 항목은 그룹 파일이 불러오는 파일이다.
// 그래서 하위 항목이 켜져 있으면 그룹 파일도 켜져 있어야 한다.
// 하위 항목을 켤 때 그룹 파일을 함께 켜고 그룹 파일을 끌 때 하위 항목도 함께 끈다.

// indexOf 는 이름이 name인 항목의 위치다. 없으면 -1이다.
func (m dotfileList) indexOf(name string) int {
	return slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == name })
}

func (it item) nodeName() string   { return it.entry.Name }
func (it item) parentName() string { return it.parent }

// children 은 i번째 항목의 하위 항목 위치들이다. 그룹이 아니면 비어 있다.
func (m dotfileList) children(i int) []int {
	return treeChildren(m.items, i)
}

// syncedChildren 은 i번째 그룹의 하위 항목 중 연결된 것의 위치들이다.
func (m dotfileList) syncedChildren(i int) []int {
	var indexes []int
	for _, j := range m.children(i) {
		if m.items[j].state == synced {
			indexes = append(indexes, j)
		}
	}
	return indexes
}

// outdatedChildren 은 i번째 그룹에서 업데이트가 있는 하위 항목 수를 센다.
// 그룹을 접으면 하위 항목 줄이 안 보이니 그룹 줄에 이 수를 대신 보여 준다.
func (m dotfileList) outdatedChildren(i int) int {
	count := 0
	for _, j := range m.syncedChildren(i) {
		if m.items[j].outdated {
			count++
		}
	}
	return count
}

// visible 은 i번째 항목이 목록에 보이는지 알려 준다. 접힌 그룹의 하위 항목만 숨는다.
func (m dotfileList) visible(i int) bool {
	return treeVisible(m.items, m.collapsed, i)
}

// collapse 는 커서가 있는 그룹을 접는다. 하위 항목에서 누르면 그 그룹을 접고 커서를 그룹 줄로 옮긴다.
func (m dotfileList) collapse() dotfileList {
	m.cursor, m.collapsed = treeCollapse(m.items, m.collapsed, m.cursor)
	return m
}

// expand 는 커서가 있는 그룹을 펼친다.
func (m dotfileList) expand() dotfileList {
	m.collapsed = treeExpand(m.items, m.collapsed, m.cursor)
	return m
}

// unlinkChildren 은 i번째 그룹의 연결된 하위 항목을 모두 끄고 끈 개수를 돌려준다.
// 하위 항목은 그룹 파일 없이는 쓰이지 않으므로 백업이 없으면 묻지 않고 symlink만 지운다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m dotfileList) unlinkChildren(i int) (dotfileList, int) {
	count := 0
	for _, j := range m.syncedChildren(i) {
		child := &m.items[j]
		child.flash = 1
		restored, err := link.Unlink(child.entry.Target, child.copyPath)
		if err != nil {
			child.state, child.err = errored, err
			continue
		}
		child.state, child.err = notSynced, nil
		if restored != "" {
			child.state = conflict
		}
		count++
	}
	return m, count
}
