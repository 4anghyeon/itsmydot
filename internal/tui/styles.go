package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// palette 는 화면에 쓰는 색 묶음이다.
// 터미널의 기본 16색은 테마에 따라 초록이 올리브색으로 보이기도 해서 색을 직접 지정한다.
// 배경 밝기에 따라 두 벌 중 하나를 고른다.
type palette struct {
	synced, conflict, errored, syncing color.Color
	muted, accent                      color.Color
	update                             color.Color // 업데이트 있음 표시. 충돌의 노란색과 헷갈리지 않게 주황 쪽으로 둔다
	group, file                        color.Color // 메뉴 제목의 그룹 이름과 파일 이름. 다른 상태 색과 겹치지 않는 파랑과 분홍
	onAccent                           color.Color // 강조색을 칠한 제목 배지 위의 글자
	selection, statusBar               color.Color // 선택한 줄과 맨 아래 상태 줄의 배경. diff 배경처럼 256색표에 있는 회색만 쓴다

	diffRemoved, diffAdded diffTones   // 비교 보기에서 지운 줄과 추가한 줄의 배경
	diffHunk               color.Color // 비교 보기에서 @@ 줄의 배경
}

// diffTones 는 비교 보기에서 지운 줄이나 추가한 줄 한쪽을 칠하는 배경색이다. 줄, 줄 번호 칸, 바뀐 단어 순으로 진하다.
// 256색 터미널은 색을 가장 가까운 칸으로 바꾸는데 옅은 분홍은 노랑으로, 옅은 초록은 하늘색으로 바뀐다.
// 그래서 256색표에 있는 색만 골라 어느 터미널에서나 빨강과 초록 계열로 보이게 한다.
type diffTones struct {
	line, number, word color.Color
}

var (
	darkPalette = palette{
		synced:   lipgloss.Color("#4ADE80"),
		conflict: lipgloss.Color("#FBBF24"),
		errored:  lipgloss.Color("#F87171"),
		syncing:  lipgloss.Color("#22D3EE"),
		muted:    lipgloss.Color("#8A8A8A"),
		accent:   lipgloss.Color("#C084FC"),
		update:   lipgloss.Color("#FB923C"),
		group:    lipgloss.Color("#60A5FA"),
		file:     lipgloss.Color("#F472B6"),

		onAccent:  lipgloss.Color("#1C1C1C"),
		selection: lipgloss.Color("#3A3A3A"),
		statusBar: lipgloss.Color("#303030"),

		diffRemoved: diffTones{lipgloss.Color("#5F0000"), lipgloss.Color("#870000"), lipgloss.Color("#AF0000")},
		diffAdded:   diffTones{lipgloss.Color("#005F00"), lipgloss.Color("#008700"), lipgloss.Color("#00AF00")},
		diffHunk:    lipgloss.Color("#303030"),
	}
	lightPalette = palette{
		synced:   lipgloss.Color("#15803D"),
		conflict: lipgloss.Color("#B45309"),
		errored:  lipgloss.Color("#DC2626"),
		syncing:  lipgloss.Color("#0E7490"),
		muted:    lipgloss.Color("#6B7280"),
		accent:   lipgloss.Color("#9333EA"),
		update:   lipgloss.Color("#C2410C"),
		group:    lipgloss.Color("#2563EB"),
		file:     lipgloss.Color("#DB2777"),

		onAccent:  lipgloss.Color("#FFFFFF"),
		selection: lipgloss.Color("#E4E4E4"),
		statusBar: lipgloss.Color("#EEEEEE"),

		diffRemoved: diffTones{lipgloss.Color("#FFD7D7"), lipgloss.Color("#FFAFAF"), lipgloss.Color("#FF8787")},
		diffAdded:   diffTones{lipgloss.Color("#D7FFD7"), lipgloss.Color("#AFFFAF"), lipgloss.Color("#87FF87")},
		diffHunk:    lipgloss.Color("#EEEEEE"),
	}
)

// styles 는 palette로 만든 스타일 묶음이다.
type styles struct {
	palette
	title, muted, cursor, selected, question, key lipgloss.Style
	update, groupName, fileName                   lipgloss.Style
	description, descriptionBar                   lipgloss.Style
	heading                                       lipgloss.Style // 도움말 화면의 소제목
	mascotFace, mascotMouth                       lipgloss.Style // 맨 위 캐릭터의 눈과 코, 입
	diffRemoved, diffAdded                        diffStyles
	diffHunk                                      lipgloss.Style
}

// diffStyles 는 비교 보기에서 지운 줄이나 추가한 줄 한쪽을 그리는 스타일이다.
// 글자 색은 터미널 기본색을 두고 배경만 칠한다. 빨간 배경에 빨간 글자는 읽기 어렵다.
type diffStyles struct {
	line, number, word, marker lipgloss.Style
}

// newDiffStyles 는 tones로 칠하는 diffStyles를 만든다. marker는 +나 - 표시라서 줄 배경 위에 mark 색 글자로 그린다.
func newDiffStyles(tones diffTones, mark, muted color.Color) diffStyles {
	return diffStyles{
		line:   lipgloss.NewStyle().Background(tones.line),
		number: lipgloss.NewStyle().Background(tones.number).Foreground(muted),
		word:   lipgloss.NewStyle().Background(tones.word),
		marker: lipgloss.NewStyle().Background(tones.line).Foreground(mark).Bold(true),
	}
}

// newStyles 는 배경 밝기에 맞는 스타일 묶음을 만든다.
func newStyles(dark bool) styles {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	return styles{
		palette:        p,
		title:          lipgloss.NewStyle().Bold(true).Foreground(p.onAccent).Background(p.accent).Padding(0, 1),
		muted:          lipgloss.NewStyle().Foreground(p.muted),
		cursor:         lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		selected:       lipgloss.NewStyle().Bold(true),
		question:       lipgloss.NewStyle().Bold(true).Foreground(p.conflict),
		key:            lipgloss.NewStyle().Bold(true),
		update:         lipgloss.NewStyle().Bold(true).Foreground(p.update),
		groupName:      lipgloss.NewStyle().Bold(true).Foreground(p.group),
		fileName:       lipgloss.NewStyle().Bold(true).Foreground(p.file),
		description:    lipgloss.NewStyle().Italic(true).Foreground(p.muted),
		descriptionBar: lipgloss.NewStyle().Foreground(p.accent),
		heading:        lipgloss.NewStyle().Bold(true),
		mascotFace:     lipgloss.NewStyle().Bold(true),
		mascotMouth:    lipgloss.NewStyle().Bold(true).Foreground(p.group),
		diffRemoved:    newDiffStyles(p.diffRemoved, p.errored, p.muted),
		diffAdded:      newDiffStyles(p.diffAdded, p.synced, p.muted),
		diffHunk:       lipgloss.NewStyle().Background(p.diffHunk).Foreground(p.muted),
	}
}

// syncSpinner 는 받는 중인 항목의 체크박스 안에서 . o O o 순서로 돈다.
// 기본 스피너의 점자 문자는 폰트에 따라 폭이 깨지고 막대가 도는 모양은 실패 표시 [-]와 헷갈려서 직접 정의한다.
var syncSpinner = spinner.Spinner{
	Frames: []string{".", "o", "O", "o"},
	FPS:    time.Second / 8,
}

// gaugeWidth 는 진행 게이지의 칸 수다.
const gaugeWidth = 16

// state 는 상태별 체크박스 색이다.
func (st styles) state(s itemState) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(st.stateColor(s))
}

// stateColor 는 상태별 색이다.
func (st styles) stateColor(s itemState) color.Color {
	switch s {
	case synced:
		return st.palette.synced
	case syncing:
		return st.palette.syncing
	case conflict:
		return st.palette.conflict
	case errored:
		return st.palette.errored
	default:
		return st.palette.muted
	}
}

// flashing 은 상태 색에 강조색을 flash만큼 섞은 스타일이다. flash가 1이면 강조색이고 0이면 상태 색 그대로다.
func (st styles) flashing(s itemState, flash float64) lipgloss.Style {
	if flash <= 0 {
		return st.state(s)
	}
	const steps = 101
	blend := lipgloss.Blend1D(steps, st.stateColor(s), st.palette.accent)
	i := int(math.Round(min(flash, 1) * (steps - 1)))
	return lipgloss.NewStyle().Foreground(blend[i])
}

// gauge 는 ratio만큼 채운 진행 막대를 그린다. ratio는 0과 1 사이로 자른다.
// 채운 칸은 목록의 연결 표시와 같은 초록이라 색만 봐도 연결된 비율인 걸 알 수 있다.
func (st styles) gauge(ratio float64) string {
	ratio = min(max(ratio, 0), 1)
	filled := int(math.Round(ratio * gaugeWidth))
	if ratio > animationSettle && filled == 0 {
		filled = 1 // 하나라도 연결돼 있으면 빈 막대로 보이지 않게 한다
	}

	return st.state(synced).Render(strings.Repeat("█", filled)) +
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

// headerMaxWidth 는 맨 위 줄에서 right를 놓는 가장 먼 칸이다.
// 넓은 터미널에서 게이지가 제목에서 너무 멀어지지 않게 한다. 구분선과 상태 줄은 그대로 터미널 끝까지 그린다.
const headerMaxWidth = 80

// mascotWidth 는 캐릭터가 차지하는 폭이다. 왼쪽 여백 한 칸과 두 눈 사이를 합쳤다.
const mascotWidth = 6

// mascot 은 맨 위 왼쪽에 세 줄로 그리는 itsmydot 캐릭터다. 두 눈 아래에 비스듬한 코와 물결 모양 입이 있다.
//
//	.   .
//	  /
//	  ~
func (st styles) mascot() [3]string {
	return [3]string{
		" " + st.mascotFace.Render(".   ."),
		"   " + st.mascotFace.Render("/") + "  ",
		"   " + st.mascotMouth.Render("~") + "  ",
	}
}

// page 는 모든 화면이 같이 쓰는 틀이다. 맨 위 세 줄 왼쪽에 캐릭터를 그리고 가운데 줄에 제목 배지와 right를 양 끝에 놓는다.
// 한 줄에 다 들어가지 않는 좁은 터미널에서는 right를 캐릭터 아랫줄로 내린다.
// 그 아래에 구분선을 긋고 lines를 폭 width에 맞춰 줄바꿈해 붙인 뒤 한 줄 띄워 맨 아래에 도움말 줄 footer를 붙인다.
func (st styles) page(width int, right string, lines []string, footer string) string {
	face := st.mascot()
	title := st.title.Render("itsmydot")
	header := []string{face[0], face[1] + "  " + title, face[2]}
	gap := min(width, headerMaxWidth) - mascotWidth - 2 - lipgloss.Width(title) - lipgloss.Width(right)
	if gap >= 2 {
		header[1] += strings.Repeat(" ", gap) + right
	} else if right != "" {
		header[2] += "  " + right
	}
	out := append(header[:], st.rule(width), "")
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			line = ansi.Wrap(line, width, " ")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n\n " + footer + "\n"
}

// rule 은 화면 폭만큼 긋는 가로 구분선이다.
func (st styles) rule(width int) string {
	return st.muted.Render(strings.Repeat("─", width))
}

// statusLine 은 맨 아래 상태 줄을 폭 width 전체에 배경을 칠해 그린다. marker가 있으면 글 앞에 color로 붙인다.
// 글이 폭보다 길면 marker 뒤 칸에 맞춰 줄을 나눈다.
// 글이 없으면 칠하지 않은 빈 줄을 돌려준다. 줄은 남겨 두어 메시지가 생길 때 화면 높이가 바뀌지 않게 한다.
func (st styles) statusLine(width int, marker string, color color.Color, text string) string {
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

// help 는 키와 설명을 번갈아 받아 상자 아래에 붙일 도움말 줄로 이어 붙인다.
// 터미널 폭 width를 넘으면 키와 설명 묶음 사이에서 줄을 나눈다. width가 0이면 나누지 않는다.
// 화면은 도움말 앞에 한 칸을 띄우므로 나눈 줄에도 같은 한 칸을 붙인다.
func (st styles) help(width int, pairs ...string) string {
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
