package tui

import (
	"image/color"
	"math"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// gaugeWidth 는 진행 게이지의 칸 수다.
const gaugeWidth = 16

// headerMaxWidth 는 맨 위 줄에서 right를 놓는 가장 먼 칸이다.
// 넓은 터미널에서 게이지가 제목에서 너무 멀어지지 않게 한다. 구분선과 상태 줄은 그대로 터미널 끝까지 그린다.
const headerMaxWidth = 80

// mascotWidth 는 캐릭터가 차지하는 폭이다. 왼쪽 여백 한 칸과 두 눈 사이를 합쳤다.
const mascotWidth = 6

// renderGauge 는 ratio만큼 채운 진행 막대를 그린다. ratio는 0과 1 사이로 자른다.
// 채운 칸은 목록의 연결 표시와 같은 초록이라 색만 봐도 연결된 비율인 걸 알 수 있다.
func (st styles) renderGauge(ratio float64) string {
	ratio = min(max(ratio, 0), 1)
	filled := int(math.Round(ratio * gaugeWidth))
	if ratio > animationSettleThreshold && filled == 0 {
		filled = 1 // 하나라도 연결돼 있으면 빈 막대로 보이지 않게 한다
	}

	return st.stateStyle(synced).Render(strings.Repeat("█", filled)) +
		st.muted.Render(strings.Repeat("░", gaugeWidth-filled))
}

// pageWidth 는 화면 폭이다. 터미널 폭을 알면 그 폭을 다 쓰고 모르면 제목 줄과 lines 중 가장 긴 줄에 맞춘다.
// 화면 폭을 터미널에 맞춰 고정해서 메뉴를 열거나 상태 메시지가 바뀌어도 구분선 길이가 출렁이지 않는다.
func (st styles) pageWidth(terminal int, right string, lines ...string) int {
	if terminal > 0 {
		return terminal
	}
	width := mascotWidth + 2 + lipgloss.Width(st.title.Render("itsmydot")) + 2 + lipgloss.Width(right)
	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}
	return width
}

// renderMascot 는 맨 위 왼쪽에 세 줄로 그리는 itsmydot 캐릭터다. 두 눈 아래에 비스듬한 코와 물결 모양 입이 있다.
//
//	.   .
//	  /
//	  ~
func (st styles) renderMascot() [3]string {
	return [3]string{
		" " + st.mascotFace.Render(".   ."),
		"   " + st.mascotFace.Render("/") + "  ",
		"   " + st.mascotMouth.Render("~") + "  ",
	}
}

// renderPage 는 모든 화면이 같이 쓰는 틀이다. 캐릭터와 제목, right를 놓은 머리 아래에 구분선과 lines, footer를 차례로 붙인다.
// 좁은 터미널에서는 right를 제목 아래 줄로 내리고 폭 width를 넘는 줄은 줄바꿈한다.
func (st styles) renderPage(width int, version, right string, lines []string, footer string) string {
	face := st.renderMascot()
	title := st.title.Render("itsmydot")
	header := []string{face[0], face[1] + "  " + title, face[2]}
	if version != "" {
		header[2] += "  " + st.muted.Render(version)
	}
	gap := min(width, headerMaxWidth) - mascotWidth - 2 - lipgloss.Width(title) - lipgloss.Width(right)
	if gap >= 2 {
		header[1] += strings.Repeat(" ", gap) + right
	} else if right != "" {
		header[2] += "  " + right
	}

	out := slices.Concat(header[:], []string{st.renderRule(width), ""}, lines)
	for i, line := range out {
		if lipgloss.Width(line) > width {
			out[i] = ansi.Wrap(line, width, " ")
		}
	}
	return strings.Join(out, "\n") + "\n\n " + footer + "\n"
}

// renderRule 은 화면 폭만큼 긋는 가로 구분선이다.
func (st styles) renderRule(width int) string {
	return st.muted.Render(strings.Repeat("─", width))
}

// renderStatusLine 은 맨 아래 상태 줄을 폭 width 전체에 배경을 칠해 그린다. marker가 있으면 글 앞에 color로 붙인다.
// 글이 없으면 칠하지 않은 빈 줄을 돌려준다. 자리는 남겨 두어 메시지가 생길 때 화면 높이가 바뀌지 않는다.
func (st styles) renderStatusLine(width int, marker string, color color.Color, text string) string {
	if text == "" {
		return ""
	}
	bar := lipgloss.NewStyle().Background(st.palette.statusBar)
	indent := 1
	if marker != "" {
		indent += lipgloss.Width(marker) + 1
	}
	lines := strings.Split(ansi.Wrap(text, max(width-indent, 1), " "), "\n")
	for i, line := range lines {
		head := bar.Render(strings.Repeat(" ", indent))
		if i == 0 && marker != "" {
			head = bar.Render(" ") + bar.Foreground(color).Bold(true).Render(marker) + bar.Render(" ")
		}
		line = head + bar.Render(line)
		lines[i] = line + bar.Render(strings.Repeat(" ", max(width-lipgloss.Width(line), 0)))
	}
	return strings.Join(lines, "\n")
}

// renderHelp 는 키와 설명을 번갈아 받아 맨 아래 도움말 줄로 이어 붙인다.
// 터미널 폭 width를 넘으면 키와 설명 묶음 사이에서 줄을 나눈다. width가 0이면 나누지 않는다.
func (st styles) renderHelp(width int, pairs ...string) string {
	const gap = "   "
	var lines []string
	line := ""
	for i := 0; i+1 < len(pairs); i += 2 {
		part := st.key.Render(pairs[i]) + " " + st.muted.Render(pairs[i+1])
		switch {
		case line == "":
			line = part
		case width > 0 && 1+lipgloss.Width(line+gap+part) > width:
			lines = append(lines, line)
			line = part
		default:
			line += gap + part
		}
	}
	return strings.Join(append(lines, line), "\n ")
}
