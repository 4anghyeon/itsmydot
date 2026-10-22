package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// View 는 항목 목록과 그 아래에 커서 항목의 설명이나 확인 질문, 상태 줄을 그린다.
// 도움말이나 비교 보기 화면이 열려 있으면 그 화면을 그린다.
func (m Model) View() tea.View {
	st := newStyles(!m.light)
	if m.helpOpen {
		return m.helpView(st)
	}
	if m.diffScreen.name != "" {
		return m.diffView(st)
	}

	right := ""
	if m.ready && m.loadErr == nil && len(m.items) > 0 {
		done := 0
		for _, it := range m.items {
			if it.state == synced {
				done++
			}
		}
		right = st.muted.Render(fmt.Sprintf("연결됨 %d/%d ", done, len(m.items))) + st.renderGauge(m.gauge)
	}

	var list, lower []string
	selected := -1
	switch {
	case !m.ready:
		list = append(list, " "+st.muted.Render("manifest 불러오는 중이에요..."))
	case m.loadErr != nil:
		list = append(list, " "+st.state(errored).Render("manifest를 불러오지 못했어요. "+errorReason(m.loadErr, m.home)))
		for line := range strings.SplitSeq(m.loadErr.Error(), "\n") {
			list = append(list, " "+st.state(errored).Render(line))
		}
	case len(m.items) == 0:
		list = append(list, " "+st.muted.Render("manifest에 항목이 없어요"))
	default:
		list, selected = m.renderItemLines(st)
		// 설명이 없어도 줄은 남겨서 커서를 옮길 때 화면 높이가 출렁이지 않게 한다.
		lower = append(lower, " "+m.renderDescription(st, m.items[m.cursor].entry.Description))
	}

	footer := st.renderHelp(m.width, "방향키", "이동", "space", "연결/연결 끊기", "enter", "메뉴", "?", "도움말", "esc", "종료")
	status := m.status
	if m.confirming != "" {
		var numbers []string
		lower, numbers = m.renderConfirmLines(st)
		footer = st.renderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
		// 질문이 떠 있는 동안 상태 줄에 남은 이전 결과는 질문과 상관없어서 비워 둔다.
		status = ""
	}

	marker, tone := "", notSynced
	if status != "" {
		switch m.statusTone {
		case statusDone:
			marker, tone = synced.marker(), synced
		case statusUnlinked:
			marker, tone = notSynced.marker(), notSynced
		case statusFailed:
			marker, tone = errored.marker(), errored
		case statusBusy:
			marker, tone = m.renderMarker(syncing), syncing
		case statusInfo:
			// 결과가 아니라 알림이라 표시 없이 글만 보여 준다.
		}
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

// renderConfirmLines 는 확인 질문이나 메뉴를 질문, 번호 선택지, 진행 상황 순으로 한 줄씩 만든다.
// 선택지 번호도 함께 돌려줘서 도움말 줄에 쓸 수 있게 한다.
func (m Model) renderConfirmLines(st styles) (lines, numbers []string) {
	question, options := m.confirmPrompt()
	title := st.question.Render(question)
	if m.confirmKind == confirmAction {
		title = m.renderMenuTitle(st)
		// 사유 문장만으로는 원인을 알기 어려울 때를 위해 원래 에러를 메뉴에서만 보여 준다.
		if i := m.indexOf(m.confirming); i >= 0 && m.items[i].state == errored && m.items[i].err != nil {
			title += "\n" + st.muted.Render(m.items[i].err.Error())
		}
	}
	for line := range strings.SplitSeq(title, "\n") {
		lines = append(lines, " "+line)
	}
	lines = append(lines, "")
	numbers = make([]string, len(options))
	for i, opt := range options {
		numbers[i] = strconv.Itoa(i + 1)
		cursor, label := " ", opt
		if i == m.choice {
			cursor, label = st.cursor.Render(">"), st.selected.Render(opt)
		}
		lines = append(lines, " "+cursor+" "+st.key.Render(numbers[i]+".")+" "+label)
	}
	switch {
	case m.comparing != "":
		lines = append(lines, "", " "+st.state(syncing).Render(m.renderMarker(syncing))+" "+m.confirmNote)
	case m.confirmNote != "":
		lines = append(lines, "", " "+m.confirmNote)
	}
	return lines, numbers
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
			width = max(width, lipgloss.Width(m.fold(i))+lipgloss.Width(m.branch(i))+lipgloss.Width(it.entry.Name))
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

		branch, fold := m.branch(i), m.fold(i)
		bar, name := " ", it.entry.Name+strings.Repeat(" ", width-lipgloss.Width(fold)-lipgloss.Width(branch)-lipgloss.Width(it.entry.Name))
		if current {
			bar, name = st.cursor.Render("▌"), paint(st.selected).Render(name)
			selected = len(lines)
		} else {
			name = plain.Render(name)
		}
		line := bar + plain.Render(" ") + paint(st.key).Render(fold) + paint(st.muted).Render(branch) + paint(st.flashing(it.state, it.flash)).Render(m.renderMarker(it.state)) +
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
			line += paint(st.state(it.state)).Render("   (다른 파일이 있음)")
		case errored:
			line += paint(st.state(it.state)).Render("   (" + errorReason(it.err, m.home) + ")")
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

// fold 는 맨 위 항목의 체크박스 앞에 붙이는 접기 표시다. 그룹은 펼쳤으면 v, 접었으면 >이고 나머지는 칸만 맞춘다.
// 체크박스 안의 +와 헷갈리지 않게 +와 -는 쓰지 않는다.
func (m Model) fold(i int) string {
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

// branch 는 i번째 하위 항목 앞에 붙이는 트리 가지다.
// 가지는 그룹 줄의 접기 표시 바로 아래에서 시작한다. 맨 위 항목이면 빈 문자열이다.
func (m Model) branch(i int) string {
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

// confirmPrompt 는 확인 질문 문구와 선택지를 만든다. handleConfirmKey이 마지막 선택지를 취소로 보므로 순서를 지켜야 한다.
func (m Model) confirmPrompt() (question string, options []string) {
	i := m.indexOf(m.confirming)
	if i < 0 {
		return "", []string{"취소"}
	}
	it := m.items[i]
	target := tildify(it.entry.Target, m.home)

	// 켜진 하위 항목이 있으면 그룹 파일을 끌 때 함께 꺼진다는 걸 미리 알린다.
	childrenNote := ""
	if children := m.syncedChildren(i); len(children) > 0 {
		names := make([]string, len(children))
		for k, j := range children {
			names[k] = m.items[j].entry.Name
		}
		childrenNote = fmt.Sprintf("하위 항목 %d개(%s)도 함께 연결이 끊겨요. ", len(children), strings.Join(names, ", "))
	}

	switch m.confirmKind {
	case confirmUnlinkWithoutBackup:
		return fmt.Sprintf("%s 백업이 없어서 되돌릴 원본 파일이 없어요. %s연결을 어떻게 끊을까요?", target, childrenNote),
			[]string{"지금 설정은 남겨 두고 연결만 끊기 (추천)", "연결을 끊고 파일도 지우기", "취소"}
	case confirmAction:
		question := it.entry.Name + "  " + target
		if it.state == synced {
			return question, []string{"최신으로 받기", "차이 보기", "연결 끊기", "닫기"}
		}
		return question, []string{"연결", "닫기"}
	case confirmGroupUnlink:
		return fmt.Sprintf("%s 연결을 끊으면 %s계속할까요?", it.entry.Name, childrenNote),
			[]string{"모두 연결 끊기", "취소"}
	case confirmTurnOff:
		return fmt.Sprintf("%s  %s 연결을 끊을까요? 백업해 둔 원본 파일을 되돌려요.", it.entry.Name, target),
			[]string{"연결 끊기", "취소"}
	case confirmTurnOn:
		question := fmt.Sprintf("%s  %s 파일을 연결할까요?", it.entry.Name, target)
		if it.parent != "" && slices.Contains(m.pending, it.parent) {
			parent := m.items[m.indexOf(it.parent)]
			question += fmt.Sprintf(" 그룹 파일 %s  %s 도 함께 연결해요.", parent.entry.Name, tildify(parent.entry.Target, m.home))
		}
		return question, []string{"연결", "취소"}
	default:
		var conflicted []string
		for _, name := range m.pending {
			if j := m.indexOf(name); j >= 0 && m.items[j].state == conflict {
				conflicted = append(conflicted, tildify(m.items[j].entry.Target, m.home))
			}
		}
		question := fmt.Sprintf("%s 에 이미 파일이 있어요. 기존 파일을 백업한 뒤 연결할까요?", strings.Join(conflicted, ", "))
		if it.parent != "" && slices.Contains(m.pending, it.parent) {
			question = fmt.Sprintf("그룹 파일 %s 도 함께 연결해요. ", it.parent) + question
		}
		return question, []string{"백업 후 연결", "차이 보기", "취소"}
	}
}

// renderMenuTitle 은 항목 메뉴의 제목이다.
func (m Model) renderMenuTitle(st styles) string {
	return m.renderItemTitle(st, m.indexOf(m.confirming))
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

// tildify 는 홈 디렉토리 아래 경로를 ~로 줄인다. /Users/me/.zshrc 는 ~/.zshrc 가 된다.
func tildify(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
