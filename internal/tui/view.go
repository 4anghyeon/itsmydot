package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// View 는 항목 목록과 그 아래에 커서 항목의 설명이나 확인 질문, 상태 줄을 그린다.
// 도움말이나 비교 보기 화면, 미리보기 창이 열려 있으면 그 화면을 그린다.
func (m Model) View() tea.View {
	st := newStyles(!m.light)
	if m.helpOpen {
		return m.helpView(st)
	}
	if m.diffScreen.name != "" {
		return m.diffView(st)
	}
	if m.preview.name != "" {
		return m.previewView(st)
	}

	right := ""
	if m.ready && m.loadErr == nil && len(m.items) > 0 {
		right = st.muted.Render(fmt.Sprintf("연결됨 %d/%d ", m.syncedCount(), len(m.items))) + st.renderGauge(m.gauge)
	}

	var list, lower []string
	selected := -1
	switch {
	case !m.ready:
		list = append(list, " "+st.muted.Render("manifest 불러오는 중이에요..."))
	case m.loadErr != nil:
		list = append(list, " "+st.stateStyle(errored).Render("manifest를 불러오지 못했어요. "+errorReason(m.loadErr, m.home)))
		for line := range strings.SplitSeq(m.loadErr.Error(), "\n") {
			list = append(list, " "+st.stateStyle(errored).Render(line))
		}
	case len(m.items) == 0:
		list = append(list, " "+st.muted.Render("manifest에 항목이 없어요"))
	default:
		list, selected = m.renderItemLines(st)
		// 설명이 없어도 줄은 남겨서 커서를 옮길 때 화면 높이가 출렁이지 않게 한다.
		lower = append(lower, " "+m.renderDescription(st, m.items[m.cursor].entry.Description))
	}

	footer := st.renderHelp(m.width, "방향키", "이동", "space", "연결/연결 끊기", "enter", "메뉴", "tab", "미리보기", "?", "도움말", "esc", "종료")
	status := m.status
	if m.confirmingName != "" {
		var numbers []string
		lower, numbers = m.renderConfirmLines(st)
		footer = st.renderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
		// 질문이 떠 있는 동안 상태 줄에 남은 이전 결과는 질문과 상관없어서 비워 둔다.
		status = ""
	}

	marker, tone := "", notSynced
	if status != "" {
		marker, tone = m.statusMarker()
	}

	width := st.pageWidth(m.width, right, append(slices.Concat(list, lower), " "+marker+" "+status)...)
	if selected >= 0 {
		// 선택한 줄은 배경을 화면 끝까지 칠한다.
		list[selected] += lipgloss.NewStyle().Background(st.selection).Render(strings.Repeat(" ", max(width-lipgloss.Width(list[selected]), 0)))
	}
	// 설명이나 질문은 구분선 바로 아래에 붙여서 목록과 멀어지지 않게 한다.
	lines := append(append(list, "", st.renderRule(width)), lower...)
	lines = append(lines, "", st.renderStatusLine(width, marker, st.stateColor(tone), status))
	return tea.NewView(st.renderPage(width, m.version, right, lines, footer))
}

// renderItemLines 는 항목 목록을 한 줄씩 만들고 커서가 있는 줄의 위치를 함께 돌려준다. 접힌 그룹의 하위 항목은 뺀다.
// 그룹 줄의 체크박스는 그룹 파일의 상태이고 켜진 하위 항목 수는 줄 끝에 따로 적는다.
//
//	    [+] gitconfig  ~/.gitconfig
//	▌ v [+] zshrc      ~/.zshrc  1/2
//	  ├── [+] env      ~/.config/zsh/env.zsh
//	  └── [!] git      ~/.config/zsh/git.zsh   (다른 파일이 있음)
//	  > [ ] claude     ~/.claude/CLAUDE.md  0/3
func (m Model) renderItemLines(st styles) (lines []string, selected int) {
	// 경로 칸이 맞도록 접기 표시와 가지의 폭을 이름 폭에 넣어 센다.
	width := 0
	for i, it := range m.items {
		if m.visible(i) {
			width = max(width, lipgloss.Width(m.foldMarker(i))+lipgloss.Width(m.treeBranch(i))+lipgloss.Width(it.entry.Name))
		}
	}

	lines = make([]string, 0, len(m.items))
	for i, it := range m.items {
		if !m.visible(i) {
			continue
		}
		current := i == m.cursor
		// 선택한 줄은 조각마다 배경을 칠해야 조각 사이의 색 초기화에 배경이 끊기지 않는다.
		paint := func(s lipgloss.Style) lipgloss.Style {
			if current {
				return s.Background(st.selection)
			}
			return s
		}
		plain := paint(lipgloss.NewStyle())

		branch, fold := m.treeBranch(i), m.foldMarker(i)
		bar, name := " ", it.entry.Name+strings.Repeat(" ", width-lipgloss.Width(fold)-lipgloss.Width(branch)-lipgloss.Width(it.entry.Name))
		if current {
			bar, name = st.cursor.Render("▌"), paint(st.selected).Render(name)
			selected = len(lines)
		} else {
			name = plain.Render(name)
		}
		line := bar + plain.Render(" ") + paint(st.key).Render(fold) + paint(st.muted).Render(branch) + paint(st.flashStyle(it.state, it.flash)).Render(m.renderMarker(it.state)) +
			plain.Render(" ") + name + plain.Render("  ") + paint(st.muted).Render(tildify(it.entry.Target, m.home))

		if children := m.children(i); len(children) > 0 {
			line += plain.Render("  ") + paint(st.count).Render(fmt.Sprintf("%d/%d", len(m.syncedChildren(i)), len(children)))
		}
		if it.state == synced && it.outdated {
			line += plain.Render("  ") + paint(st.update).Render("^ 업데이트 있음")
		}
		// 접힌 그룹은 하위 항목 줄이 안 보이므로 하위 항목의 업데이트를 그룹 줄에 모아 보여 준다.
		if n := m.outdatedChildren(i); n > 0 && m.collapsed[it.entry.Name] {
			line += plain.Render("  ") + paint(st.update).Render(fmt.Sprintf("^ 하위 %d개 업데이트 있음", n))
		}

		switch it.state {
		case conflict:
			line += paint(st.stateStyle(it.state)).Render("   (다른 파일이 있음)")
		case errored:
			line += paint(st.stateStyle(it.state)).Render("   (" + errorReason(it.err, m.home) + ")")
		case notSynced, synced, syncing:
			// 체크박스만으로 충분해서 덧붙일 설명이 없다.
		}
		lines = append(lines, line)
	}
	return lines, selected
}

// renderDescription 은 커서가 있는 항목의 설명을 인용문처럼 왼쪽 막대를 붙여 그린다.
func (m Model) renderDescription(st styles, text string) string {
	if text == "" {
		return ""
	}
	const indent = 2
	if m.width > 0 {
		// 설명 앞에는 화면 왼쪽 여백 한 칸이 더 붙는다.
		text = ansi.Wrap(text, max(m.width-1-indent, 1), " ")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		prefix := strings.Repeat(" ", indent)
		if i == 0 {
			prefix = st.descriptionBar.Render("┃") + " "
		}
		lines[i] = prefix + st.description.Render(line)
	}
	return strings.Join(lines, "\n")
}

// foldMarker 는 맨 위 항목의 체크박스 앞에 붙이는 접기 표시다. 그룹은 펼쳤으면 v, 접었으면 >이고 나머지는 칸만 맞춘다.
// 체크박스 안의 +와 헷갈리지 않게 +와 -는 쓰지 않는다.
func (m Model) foldMarker(i int) string {
	switch {
	case m.items[i].parent != "" || !slices.ContainsFunc(m.items, func(it item) bool { return it.parent != "" }):
		return ""
	case len(m.children(i)) == 0:
		return "  "
	case m.collapsed[m.items[i].entry.Name]:
		return "> "
	default:
		return "v "
	}
}

// treeBranch 는 i번째 하위 항목 앞에 붙이는 트리 가지다.
// 가지는 그룹 줄의 접기 표시 바로 아래에서 시작한다. 맨 위 항목이면 빈 문자열이다.
func (m Model) treeBranch(i int) string {
	parent := m.items[i].parent
	if parent == "" {
		return ""
	}
	if i+1 < len(m.items) && m.items[i+1].parent == parent {
		return "├── "
	}
	return "└── "
}

// renderMarker 는 체크박스를 그린다. 받는 중이면 ~ 자리에 스피너의 현재 모양을 넣는다.
func (m Model) renderMarker(s itemState) string {
	if s == syncing && len(m.spinner.Spinner.Frames) > 0 {
		return "[" + m.spinner.View() + "]"
	}
	return s.marker()
}

// renderItemTitle 은 i번째 항목의 제목이다. 메뉴와 비교 보기 화면이 같이 쓰고 하위 항목은 그룹 줄 아래에 매단다.
//
//	zshrc  ~/.zshrc
//	└─ env  ~/.config/zsh/env.zsh
func (m Model) renderItemTitle(st styles, i int) string {
	if i < 0 {
		return ""
	}
	it := m.items[i]
	target := tildify(it.entry.Target, m.home)

	if len(m.children(i)) > 0 {
		return m.renderGroupTitle(st, i)
	}
	line := st.fileName.Render(it.entry.Name) + "  " + target
	if it.parent != "" {
		return m.renderGroupTitle(st, m.indexOf(it.parent)) + "\n" + st.muted.Render("└─ ") + line
	}
	return line
}

// renderGroupTitle 은 메뉴 제목에 쓰는 그룹 한 줄이다.
func (m Model) renderGroupTitle(st styles, i int) string {
	return st.groupName.Render(m.items[i].entry.Name) + "  " + tildify(m.items[i].entry.Target, m.home)
}
