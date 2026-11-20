package tui

import "maps"

// 두 목록은 모두 그룹과 하위 항목으로 이뤄진 트리다. dotfiles는 그룹 파일과 그 파일이 불러오는 파일이고
// brewfiles는 프로필과 그 안의 패키지다. 하위 항목은 목록에서 그룹 바로 뒤에 이어 붙고 그룹 이름으로 접고 펼친다.

// treeNode 는 트리 목록의 한 줄이다.
type treeNode interface {
	nodeName() string
	parentName() string // 하위 항목이면 그룹 이름이고 맨 위 항목이면 빈 문자열이다
}

// treeChildren 은 i번째 줄의 하위 항목 위치들이다. 그룹이 아니면 비어 있다.
func treeChildren[T treeNode](nodes []T, i int) []int {
	var indexes []int
	for j := i + 1; j < len(nodes) && nodes[j].parentName() == nodes[i].nodeName(); j++ {
		indexes = append(indexes, j)
	}
	return indexes
}

// treeVisible 은 i번째 줄이 목록에 보이는지 알려 준다. 접힌 그룹의 하위 항목만 숨는다.
func treeVisible[T treeNode](nodes []T, collapsed map[string]bool, i int) bool {
	return nodes[i].parentName() == "" || !collapsed[nodes[i].parentName()]
}

// treeNextVisible 은 from에서 step 방향으로 가장 가까운 보이는 줄의 위치다. 없으면 from을 그대로 돌려준다.
func treeNextVisible[T treeNode](nodes []T, collapsed map[string]bool, from, step int) int {
	for i := from + step; i >= 0 && i < len(nodes); i += step {
		if treeVisible(nodes, collapsed, i) {
			return i
		}
	}
	return from
}

// treeCollapse 는 cursor가 있는 그룹을 접고 옮긴 커서와 새 collapsed를 돌려준다.
// 하위 항목에서 누르면 그 그룹을 접고 커서를 그룹 줄로 옮긴다. 그룹이 아니면 그대로 돌려준다.
func treeCollapse[T treeNode](nodes []T, collapsed map[string]bool, cursor int) (int, map[string]bool) {
	if len(nodes) == 0 {
		return cursor, collapsed
	}
	group := cursor
	for group > 0 && nodes[group].parentName() != "" {
		group--
	}
	if len(treeChildren(nodes, group)) == 0 {
		return cursor, collapsed
	}
	return group, withCollapsed(collapsed, nodes[group].nodeName(), true)
}

// treeExpand 는 cursor가 있는 그룹을 펼친 새 collapsed를 돌려준다.
func treeExpand[T treeNode](nodes []T, collapsed map[string]bool, cursor int) map[string]bool {
	if len(nodes) == 0 || len(treeChildren(nodes, cursor)) == 0 {
		return collapsed
	}
	return withCollapsed(collapsed, nodes[cursor].nodeName(), false)
}

// withCollapsed 는 name 그룹의 접힘만 바꾼 새 map을 만든다. 모델이 복사돼도 map은 공유되므로 원래 map은 건드리지 않는다.
func withCollapsed(collapsed map[string]bool, name string, value bool) map[string]bool {
	next := maps.Clone(collapsed)
	if next == nil {
		next = make(map[string]bool)
	}
	next[name] = value
	return next
}

// treeFoldMarker 는 맨 위 줄의 체크박스 앞에 붙이는 접기 표시다. 그룹은 펼쳤으면 v, 접었으면 >이고 나머지는 칸만 맞춘다.
// 체크박스 안의 +와 헷갈리지 않게 +와 -는 쓰지 않는다. 하위 항목이 하나도 없는 목록이면 칸도 두지 않는다.
func treeFoldMarker[T treeNode](nodes []T, collapsed map[string]bool, i int) string {
	hasChildren := false
	for _, node := range nodes {
		if node.parentName() != "" {
			hasChildren = true
			break
		}
	}
	switch {
	case nodes[i].parentName() != "" || !hasChildren:
		return ""
	case len(treeChildren(nodes, i)) == 0:
		return "  "
	case collapsed[nodes[i].nodeName()]:
		return "> "
	default:
		return "v "
	}
}

// treeBranch 는 i번째 하위 항목 앞에 붙이는 트리 가지다.
// 가지는 그룹 줄의 접기 표시 바로 아래에서 시작한다. 맨 위 줄이면 빈 문자열이다.
func treeBranch[T treeNode](nodes []T, i int) string {
	parent := nodes[i].parentName()
	if parent == "" {
		return ""
	}
	if i+1 < len(nodes) && nodes[i+1].parentName() == parent {
		return "├── "
	}
	return "└── "
}
