package tui

import (
	"maps"
	"slices"

	"github.com/4anghyeon/itsmydot/internal/link"
)

// 그룹은 자기 파일을 가진 항목이고 하위 항목은 그룹 파일이 불러오는 파일이다.
// 그래서 하위 항목이 켜져 있으면 그룹 파일도 켜져 있어야 한다.
// 하위 항목을 켤 때 그룹 파일을 함께 켜고 그룹 파일을 끌 때 하위 항목도 함께 끈다.

// indexOf 는 이름이 name인 항목의 위치다. 없으면 -1이다.
func (m Model) indexOf(name string) int {
	return slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == name })
}

// children 은 i번째 항목의 하위 항목 위치들이다. 그룹이 아니면 비어 있다.
func (m Model) children(i int) []int {
	var indexes []int
	for j := i + 1; j < len(m.items) && m.items[j].parent == m.items[i].entry.Name; j++ {
		indexes = append(indexes, j)
	}
	return indexes
}

// syncedChildren 은 i번째 그룹의 하위 항목 중 연결된 것의 위치들이다.
func (m Model) syncedChildren(i int) []int {
	var indexes []int
	for _, j := range m.children(i) {
		if m.items[j].state == synced {
			indexes = append(indexes, j)
		}
	}
	return indexes
}

// visible 은 i번째 항목이 목록에 보이는지 알려 준다. 접힌 그룹의 하위 항목만 숨는다.
func (m Model) visible(i int) bool {
	return m.items[i].parent == "" || !m.collapsed[m.items[i].parent]
}

// nextVisible 은 from에서 step 방향으로 가장 가까운 보이는 항목의 위치다. 없으면 from을 그대로 돌려준다.
func (m Model) nextVisible(from, step int) int {
	for i := from + step; i >= 0 && i < len(m.items); i += step {
		if m.visible(i) {
			return i
		}
	}
	return from
}

// collapse 는 커서가 있는 그룹을 접는다. 하위 항목에서 누르면 그 그룹을 접고 커서를 그룹 줄로 옮긴다.
func (m Model) collapse() Model {
	if len(m.items) == 0 {
		return m
	}
	group := m.cursor
	if parent := m.items[m.cursor].parent; parent != "" {
		group = m.indexOf(parent)
	}
	if len(m.children(group)) == 0 {
		return m
	}
	m.cursor = group
	return m.setCollapsed(m.items[group].entry.Name, true)
}

// expand 는 커서가 있는 그룹을 펼친다.
func (m Model) expand() Model {
	if len(m.items) == 0 || len(m.children(m.cursor)) == 0 {
		return m
	}
	return m.setCollapsed(m.items[m.cursor].entry.Name, false)
}

func (m Model) setCollapsed(name string, collapsed bool) Model {
	m.collapsed = maps.Clone(m.collapsed)
	if m.collapsed == nil {
		m.collapsed = make(map[string]bool)
	}
	m.collapsed[name] = collapsed
	return m
}

// unlinkChildren 은 i번째 그룹의 연결된 하위 항목을 모두 끄고 끈 개수를 돌려준다.
// 하위 항목은 그룹 파일 없이는 쓰이지 않으므로 백업이 없으면 묻지 않고 symlink만 지운다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) unlinkChildren(i int) (Model, int) {
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
