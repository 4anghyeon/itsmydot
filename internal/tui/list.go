package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// listRow 는 트리 목록 한 줄을 이루는 조각들이다.
// 목록 그리기는 이름 칸 정렬과 선택한 줄 칠하기만 맡고 무엇을 보여 줄지는 각 목록이 정한다.
type listRow struct {
	fold, branch string // 접기 표시와 트리 가지. 그룹이 없는 목록이면 비어 있다
	marker       string // 체크박스
	markerStyle  lipgloss.Style
	name         string        // 가장 긴 이름에 맞춰 뒤를 공백으로 채운다
	detail       string        // 이름 뒤에 흐린 색으로 붙는 경로 같은 정보
	tail         []listSegment // detail 뒤에 차례로 붙는 표시
	current      bool          // 커서가 있는 줄이면 true
}

// listSegment 는 줄 끝에 붙는 표시 하나다. gap은 앞 칸과 띄울 공백이고 style은 text에만 입힌다.
type listSegment struct {
	gap, text string
	style     lipgloss.Style
}

// renderRows 는 rows를 한 줄씩 그리고 커서가 있는 줄의 위치를 함께 돌려준다. 커서가 있는 줄이 없으면 -1이다.
// detail 칸이 맞도록 접기 표시와 가지의 폭을 이름 폭에 넣어 센다.
func (st styles) renderRows(rows []listRow) (lines []string, selected int) {
	width := 0
	for _, row := range rows {
		width = max(width, lipgloss.Width(row.fold)+lipgloss.Width(row.branch)+lipgloss.Width(row.name))
	}

	selected = -1
	lines = make([]string, 0, len(rows))
	for _, row := range rows {
		// 선택한 줄은 조각마다 배경을 칠해야 조각 사이의 색 초기화에 배경이 끊기지 않는다.
		paint := func(s lipgloss.Style) lipgloss.Style {
			if row.current {
				return s.Background(st.selection)
			}
			return s
		}
		plain := paint(lipgloss.NewStyle())

		bar, name := " ", row.name+strings.Repeat(" ", width-lipgloss.Width(row.fold)-lipgloss.Width(row.branch)-lipgloss.Width(row.name))
		if row.current {
			bar, name = st.cursor.Render("▌"), paint(st.selected).Render(name)
			selected = len(lines)
		} else {
			name = plain.Render(name)
		}
		line := bar + plain.Render(" ") + paint(st.key).Render(row.fold) + paint(st.muted).Render(row.branch) + paint(row.markerStyle).Render(row.marker) +
			plain.Render(" ") + name + plain.Render("  ") + paint(st.muted).Render(row.detail)
		for _, segment := range row.tail {
			line += plain.Render(segment.gap) + paint(segment.style).Render(segment.text)
		}
		lines = append(lines, line)
	}
	return lines, selected
}

// renderSpinner 는 체크박스 안에 스피너의 현재 모양을 넣어 그린다. 받거나 설치하는 중인 줄에 쓴다.
func renderSpinner(s spinner.Model) string {
	return "[" + s.View() + "]"
}

// renderDescription 은 커서가 있는 항목의 설명을 인용문처럼 왼쪽 막대를 붙여 그린다. 폭 width를 넘으면 줄을 바꾸고 0이면 바꾸지 않는다.
func (st styles) renderDescription(text string, width int) string {
	if text == "" {
		return ""
	}
	const indent = 2
	if width > 0 {
		// 설명 앞에는 화면 왼쪽 여백 한 칸이 더 붙는다.
		text = ansi.Wrap(text, max(width-1-indent, 1), " ")
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
