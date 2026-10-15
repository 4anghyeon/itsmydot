package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View 는 상자 안에 제목과 진행 게이지, 항목 목록, 상태 메시지나 확인 질문을 그리고
// 상자 아래에 도움말을 붙인다. 목록을 불러오는 중이거나 실패했으면 목록 대신 그 안내를 보여 준다.
func (m Model) View() tea.View {
	st := newStyles(!m.light)

	var body []string
	switch {
	case !m.ready:
		body = append(body, st.muted.Render("manifest 불러오는 중..."))
	case m.loadErr != nil:
		body = append(body, st.state(errored).Render("manifest를 불러오지 못했습니다: "+m.loadErr.Error()))
	case len(m.items) == 0:
		body = append(body, st.muted.Render("항목이 없습니다."))
	default:
		body = append(body, m.itemLines(st)...)
	}

	move := "이동"
	if slices.ContainsFunc(m.items, func(it item) bool { return it.parent != "" }) {
		move = "이동/접기"
	}
	footer := st.help("방향키", move, "space", "토글", "enter", "메뉴", "esc", "종료")
	if m.confirming != "" {
		question, options := m.confirmPrompt()
		body = append(body, "", st.question.Render(question), "")
		numbers := make([]string, len(options))
		for i, opt := range options {
			numbers[i] = strconv.Itoa(i + 1)
			cursor, label := " ", opt
			if i == m.choice {
				cursor, label = st.cursor.Render(">"), st.selected.Render(opt)
			}
			body = append(body, cursor+" "+st.key.Render(numbers[i]+".")+" "+label)
		}
		footer = st.help("방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
	} else if m.status != "" {
		body = append(body, "", m.status)
	}

	title := st.title.Render("itsmydot")
	right := ""
	if m.ready && m.loadErr == nil && len(m.items) > 0 {
		done := 0
		for _, it := range m.items {
			if it.state == synced {
				done++
			}
		}
		right = st.muted.Render(fmt.Sprintf("연결됨 %d/%d ", done, len(m.items))) + st.gauge(m.gauge)
	}
	// 상자 안쪽 폭은 가장 긴 줄에 맞춘다. 터미널보다 넓어지면 터미널 폭에 맞추고 긴 줄은 상자 안에서 줄바꿈한다.
	inner := lipgloss.Width(title) + 4 + lipgloss.Width(right)
	for _, line := range body {
		inner = max(inner, lipgloss.Width(line))
	}
	if m.width > 0 && inner+frameExtra > m.width {
		inner = max(m.width-frameExtra, 1)
	}

	header := title + strings.Repeat(" ", max(inner-lipgloss.Width(title)-lipgloss.Width(right), 1)) + right
	content := header + "\n\n" + strings.Join(body, "\n")
	box := st.frame.Width(inner + frameExtra).Render(content)
	return tea.NewView(box + "\n " + footer + "\n")
}

// itemLines 는 항목 목록을 한 줄씩 만든다. 하위 항목은 들여 쓰고 접힌 그룹의 하위 항목은 뺀다.
// 그룹 줄의 체크박스는 그룹 파일의 상태이고 하위 항목이 몇 개 켜졌는지는 줄 끝에 따로 적는다.
//
//	  [+] gitconfig  ~/.gitconfig
//	> [+] zshrc      ~/.zshrc  1/2
//	      [+] env    ~/.config/zsh/00-env.zsh
//	      [!] git    ~/.config/zsh/20-git.zsh   (다른 파일이 있음)
func (m Model) itemLines(st styles) []string {
	// 들여 쓴 하위 항목도 경로 칸이 맞도록 들여쓰기를 이름 폭에 넣어 센다.
	width := 0
	for i, it := range m.items {
		if m.visible(i) {
			width = max(width, len(indent(it))+lipgloss.Width(it.entry.Name))
		}
	}

	lines := make([]string, 0, len(m.items))
	for i, it := range m.items {
		if !m.visible(i) {
			continue
		}
		pad := indent(it)
		cursor, name := " ", it.entry.Name+strings.Repeat(" ", width-len(pad)-lipgloss.Width(it.entry.Name))
		if i == m.cursor {
			cursor, name = st.cursor.Render(">"), st.selected.Render(name)
		}
		line := cursor + " " + pad + st.flashing(it.state, it.flash).Render(m.marker(it.state)) + " " + name + "  " + st.muted.Render(tildify(it.entry.Target, m.home))

		if children := m.children(i); len(children) > 0 {
			count := fmt.Sprintf("  %d/%d", len(m.syncedChildren(i)), len(children))
			if m.collapsed[it.entry.Name] {
				count += " 접힘"
			}
			line += st.muted.Render(count)
		}

		switch it.state {
		case conflict:
			line += st.state(it.state).Render("   (다른 파일이 있음)")
		case errored:
			line += st.state(it.state).Render(fmt.Sprintf("   (%v)", it.err))
		case notSynced, synced, syncing:
			// 체크박스만으로 충분해서 덧붙일 설명이 없다.
		}
		lines = append(lines, line)
	}
	return lines
}

// indent 는 하위 항목 앞에 넣는 들여쓰기다.
func indent(it item) string {
	if it.parent == "" {
		return ""
	}
	return "    "
}

// marker 는 체크박스를 그린다. 받는 중이면 ~ 자리에 스피너의 현재 모양을 넣는다.
func (m Model) marker(s itemState) string {
	if s == syncing && len(m.spinner.Spinner.Frames) > 0 {
		return "[" + m.spinner.View() + "]"
	}
	return s.marker()
}

// confirmPrompt 는 확인 질문 문구와 선택지를 만든다. answerConfirm이 마지막 선택지를 취소로 보므로 순서를 지켜야 한다.
func (m Model) confirmPrompt() (question string, options []string) {
	i := m.indexOf(m.confirming)
	if i < 0 {
		return "", []string{"취소"}
	}
	it := m.items[i]
	target := tildify(it.entry.Target, m.home)

	// 켜진 하위 항목이 있으면 그룹 파일을 끌 때 함께 꺼진다는 걸 미리 알린다.
	childrenNote := ""
	if n := len(m.syncedChildren(i)); n > 0 {
		childrenNote = fmt.Sprintf("하위 항목 %d개도 함께 꺼집니다. ", n)
	}

	switch m.confirmKind {
	case confirmUnlink:
		return fmt.Sprintf("%s 의 원래 파일(백업)을 찾을 수 없습니다. %s연결을 어떻게 끊을까요?", target, childrenNote),
			[]string{"지금 설정은 남겨 두고 연결만 끊기 (추천)", "연결을 끊고 파일도 지우기", "취소"}
	case confirmAction:
		question := it.entry.Name + "  " + target
		if it.state == synced {
			return question, []string{"최신으로 받기", "끄기", "닫기"}
		}
		return question, []string{"켜기", "닫기"}
	case confirmGroupUnlink:
		return fmt.Sprintf("%s 를 끄면 %s계속할까요?", it.entry.Name, childrenNote),
			[]string{"모두 끄기", "취소"}
	default:
		var conflicted []string
		for _, name := range m.pending {
			if j := m.indexOf(name); j >= 0 && m.items[j].state == conflict {
				conflicted = append(conflicted, tildify(m.items[j].entry.Target, m.home))
			}
		}
		question := fmt.Sprintf("%s 에 이미 파일이 있습니다. 기존 파일을 백업한 뒤 연결할까요?", strings.Join(conflicted, ", "))
		if it.parent != "" && slices.Contains(m.pending, it.parent) {
			question = fmt.Sprintf("%s 는 %s 가 불러오는 파일이라 %s 도 함께 켭니다. ", it.entry.Name, it.parent, it.parent) + question
		}
		return question, []string{"백업 후 연결", "취소"}
	}
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
