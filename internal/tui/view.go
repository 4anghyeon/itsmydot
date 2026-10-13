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

// View 는 상자 안에 제목·진행 게이지, 항목 목록(또는 불러오는 중/에러), 상태 메시지나 확인 질문을 그리고,
// 상자 아래에 도움말을 붙인다.
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

	footer := st.help("방향키", "이동", "space", "토글", "esc", "종료")
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

	// 상자 안쪽 폭: 가장 긴 줄에 맞추되, 터미널보다 넓으면 터미널 폭에 맞추고 긴 줄은 상자 안에서 줄바꿈한다.
	title := st.title.Render("itsmydot")
	right := ""
	if m.ready && m.loadErr == nil && len(m.items) > 0 {
		done := 0
		for _, it := range m.items {
			if it.state == synced {
				done++
			}
		}
		right = st.muted.Render(fmt.Sprintf("연결됨 %d/%d ", done, len(m.items))) + st.gauge(done, len(m.items))
	}
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

// itemLines 는 항목 목록을 한 줄씩 만든다.
//
//	> [+] gitconfig  ~/.gitconfig
//	  [!] zshrc      ~/.zshrc   (다른 파일이 있음)
func (m Model) itemLines(st styles) []string {
	// 이름 칸 너비를 가장 긴 이름에 맞춘다.
	width := 0
	for _, it := range m.items {
		width = max(width, lipgloss.Width(it.entry.Name))
	}

	lines := make([]string, 0, len(m.items))
	for i, it := range m.items {
		cursor, name := " ", it.entry.Name+strings.Repeat(" ", width-lipgloss.Width(it.entry.Name))
		if i == m.cursor {
			cursor, name = st.cursor.Render(">"), st.selected.Render(name)
		}
		line := cursor + " " + st.state(it.state).Render(m.marker(it.state)) + " " + name + "  " + st.muted.Render(tildify(it.entry.Target, m.home))

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

// marker 는 체크박스를 그린다. 받는 중이면 ~ 자리에 스피너의 현재 모양을 넣는다.
func (m Model) marker(s itemState) string {
	if s == syncing && len(m.spinner.Spinner.Frames) > 0 {
		return "[" + m.spinner.View() + "]"
	}
	return s.marker()
}

// confirmPrompt 는 확인 질문 문구와 선택지를 만든다. 마지막 선택지는 항상 "취소"다.
func (m Model) confirmPrompt() (question string, options []string) {
	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == m.confirming })
	if i < 0 {
		return "", []string{"취소"}
	}
	target := tildify(m.items[i].entry.Target, m.home)

	switch m.confirmKind {
	case confirmUnlink:
		return fmt.Sprintf("%s 의 원래 파일(백업)을 찾을 수 없습니다. 연결을 어떻게 끊을까요?", target),
			[]string{"지금 설정은 남겨 두고 연결만 끊기 (추천)", "연결을 끊고 파일도 지우기", "취소"}
	default:
		return fmt.Sprintf("%s 에 이미 파일이 있습니다. 기존 파일을 백업한 뒤 연결할까요?", target),
			[]string{"백업 후 연결", "취소"}
	}
}

// tildify 는 홈 디렉토리로 시작하는 경로를 ~로 줄인다. "/Users/me/.zshrc" → "~/.zshrc"
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
