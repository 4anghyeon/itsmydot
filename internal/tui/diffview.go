package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/diff"
)

// diffScreen 은 비교 보기 화면의 내용과 스크롤 위치다.
// 터미널 크기가 바뀌면 보이는 범위도 달라지므로 위치는 그릴 때마다 다시 맞춘다.
type diffScreen struct {
	name             string
	rows             []diffRow
	yOffset, xOffset int
}

// diffRowKind 는 비교 보기 화면 한 줄의 종류다.
type diffRowKind int

const (
	hunkRow      diffRowKind = iota // @@ 머리줄
	lineRow                         // diff 한 줄
	noNewlineRow                    // 바로 위 줄 끝에 줄바꿈이 없다는 안내
)

// diffRow 는 비교 보기 화면의 한 줄이다. header는 hunkRow에서, line은 lineRow에서만 쓴다.
type diffRow struct {
	kind   diffRowKind
	header string
	line   diff.Line
}

// diffLayout 은 지금 터미널 크기에서 diff 본문을 어떻게 나눠 그릴지다.
type diffLayout struct {
	width, height int  // 본문 폭과 높이. 폭에는 스크롤 막대 칸이 들어 있다
	rowWidth      int  // 스크롤 막대 칸을 뺀 폭. 지운 줄과 추가한 줄은 여기까지 배경을 칠한다
	numberWidth   int  // 줄 번호 한 칸의 폭
	contentWidth  int  // 줄 번호와 +/- 표시를 뺀 내용 칸의 폭
	maxXOffset    int  // 가장 긴 내용이 끝까지 보이는 가로 위치
	scrollbar     bool // 한 화면에 다 들어가지 않으면 true
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

// diffHorizontalStep 은 좌우 방향키 한 번에 옆으로 미는 칸 수다.
const diffHorizontalStep = 8

// diffMaxHeight 는 터미널 높이를 모를 때 diff 본문을 보여 줄 최대 줄 수다.
const diffMaxHeight = 20

// tabWidth 는 탭 하나를 펼칠 칸 수다. 터미널마다 탭 폭이 달라 그대로 두면 줄 번호와 배경 칸이 어긋난다.
const tabWidth = 4

// noNewlineText 는 파일 끝에 줄바꿈이 없는 줄 아래에 붙이는 안내다. git diff와 같은 문구를 쓴다.
const noNewlineText = `\ No newline at end of file`

// handleDiffKey 는 비교 보기 화면의 키 입력을 처리한다. esc나 q로 닫으면 화면을 열기 전으로 돌아간다.
func (m Model) handleDiffKey(key string) (Model, tea.Cmd) {
	layout := m.computeDiffLayout(newStyles(!m.light))
	y, x := m.diffScreen.yOffset, m.diffScreen.xOffset
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.diffScreen = diffScreen{}
		return m, nil
	case "up", "k":
		y--
	case "down", "j":
		y++
	case "pgup":
		y -= layout.height
	case "pgdown", "space":
		y += layout.height
	case "home":
		y = 0
	case "end":
		y = len(m.diffScreen.rows)
	case "left", "h":
		x -= diffHorizontalStep
	case "right", "l":
		x += diffHorizontalStep
	}
	m.diffScreen.yOffset, m.diffScreen.xOffset = m.clampDiffOffsets(layout, y, x)
	return m, nil
}

// clampDiffOffsets 는 스크롤 위치를 내용이 있는 범위 안으로 자른다.
func (m Model) clampDiffOffsets(layout diffLayout, y, x int) (int, int) {
	y = min(max(y, 0), max(len(m.diffScreen.rows)-layout.height, 0))
	x = min(max(x, 0), layout.maxXOffset)
	return y, x
}

// computeDiffLayout 은 본문 폭과 높이를 정한다. 터미널 크기를 알면 머리와 제목, 도움말 줄을 빼고 남는 만큼을 쓴다.
func (m Model) computeDiffLayout(st styles) diffLayout {
	rows := m.diffScreen.rows
	title := m.renderDiffTitle(st)
	layout := diffLayout{height: min(len(rows), diffMaxHeight)}
	if m.height > 0 {
		// 맨 위 캐릭터 세 줄과 구분선, 빈 줄, 제목과 빈 줄, 도움말과 그 위 빈 줄, 마지막 줄바꿈 뒤의 빈 줄로 10줄이다.
		// 하위 항목은 제목이 그룹 줄까지 두 줄이다.
		reservedLines := 10 + strings.Count(title, "\n")
		layout.height = min(len(rows), max(m.height-reservedLines, 3))
	}
	layout.scrollbar = len(rows) > layout.height

	// @@ 줄은 +/- 표시 자리부터 그리므로 내용 폭과 따로 센다.
	largestLineNumber, longestContent, widestHunkHeader := 1, 0, 0
	for _, row := range rows {
		switch row.kind {
		case hunkRow:
			widestHunkHeader = max(widestHunkHeader, lipgloss.Width(row.header))
		case lineRow:
			largestLineNumber = max(largestLineNumber, row.line.BeforeNumber, row.line.AfterNumber)
			longestContent = max(longestContent, lipgloss.Width(expandTabs(row.line.Text)))
		case noNewlineRow:
			longestContent = max(longestContent, lipgloss.Width(noNewlineText))
		}
	}
	layout.numberWidth = len(strconv.Itoa(largestLineNumber))

	scrollbarWidth := 0
	if layout.scrollbar {
		scrollbarWidth = 2
	}
	prefix := diffPrefixWidth(layout.numberWidth)
	widestHunkHeader += diffHunkIndent(layout.numberWidth)
	layout.width = max(lipgloss.Width(title), prefix+longestContent+scrollbarWidth, widestHunkHeader+scrollbarWidth)
	if m.width > 0 {
		// 본문 앞에 화면 왼쪽 여백 한 칸을 둔다.
		layout.width = max(m.width-1, 1)
	}
	layout.rowWidth = layout.width - scrollbarWidth
	layout.contentWidth = max(layout.rowWidth-prefix, 1)
	layout.maxXOffset = max(longestContent-layout.contentWidth, 0)
	return layout
}

// diffPrefixWidth 는 내용 앞에 붙는 칸의 폭이다. 사본 번호와 저장소 번호, 두 칸 띈 +/- 표시, 다시 두 칸이다.
func diffPrefixWidth(numberWidth int) int {
	return diffHunkIndent(numberWidth) + 1 + 2
}

// diffHunkIndent 는 +/- 표시가 놓이는 위치다. @@ 줄도 여기서부터 쓴다.
func diffHunkIndent(numberWidth int) int {
	return numberWidth*2 + 1 + 2
}

// expandTabs 는 탭을 tabWidth칸 공백으로 펼친다.
func expandTabs(text string) string {
	return strings.ReplaceAll(text, "\t", strings.Repeat(" ", tabWidth))
}

// renderDiffTitle 은 비교 보기 화면에서 어떤 항목을 비교하는지 보여 주는 제목이다.
func (m Model) renderDiffTitle(st styles) string {
	return m.renderItemTitle(st, m.indexOf(m.diffScreen.name))
}

// diffView 는 목록 대신 비교 보기 화면을 그린다.
func (m Model) diffView(st styles) tea.View {
	rows := m.diffScreen.rows
	layout := m.computeDiffLayout(st)
	y, x := m.clampDiffOffsets(layout, m.diffScreen.yOffset, m.diffScreen.xOffset)

	body := make([]string, 0, layout.height)
	for i, row := range rows[y : y+layout.height] {
		line := st.renderDiffRow(row, layout, x)
		if layout.scrollbar {
			padding := max(layout.width-2-lipgloss.Width(line), 0)
			line += strings.Repeat(" ", padding) + " " + st.renderScrollbarCell(i, layout.height, y, len(rows))
		}
		body = append(body, line)
	}

	right := st.muted.Render(fmt.Sprintf("%d/%d줄", y+layout.height, len(rows)))
	lines := make([]string, 0, len(body)+3)
	for line := range strings.SplitSeq(m.renderDiffTitle(st), "\n") {
		lines = append(lines, " "+line)
	}
	lines = append(lines, "")
	for _, line := range body {
		lines = append(lines, " "+line)
	}
	footer := st.renderHelp(m.width, "방향키", "스크롤", "pgup/pgdn", "한 쪽씩", "esc", "닫기")
	return tea.NewView(st.renderPage(st.pageWidth(m.width, right, lines...), m.version, right, lines, footer))
}

// renderDiffRow 는 화면 한 줄을 줄 번호와 함께 그린다. 가로로 밀 때는 내용만 밀고 줄 번호는 그 자리에 둔다.
//
//	1 1     [user]
//	2    -      name = me
//	  2  +      name = you
func (st styles) renderDiffRow(row diffRow, layout diffLayout, x int) string {
	switch row.kind {
	case hunkRow:
		text := ansi.Truncate(strings.Repeat(" ", diffHunkIndent(layout.numberWidth))+row.header, layout.rowWidth, "")
		return st.diffHunk.Render(text + strings.Repeat(" ", max(layout.rowWidth-lipgloss.Width(text), 0)))
	case noNewlineRow:
		return strings.Repeat(" ", diffPrefixWidth(layout.numberWidth)) + st.muted.Render(ansi.Cut(noNewlineText, x, x+layout.contentWidth))
	default:
		return st.renderDiffLine(row.line, layout, x)
	}
}

// renderDiffLine 은 diff 한 줄을 그린다. 문맥 줄은 배경 없이 그리고 지운 줄과 추가한 줄은 각자의 배경으로 칠한다.
func (st styles) renderDiffLine(line diff.Line, layout diffLayout, x int) string {
	number := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", layout.numberWidth)
		}
		return fmt.Sprintf("%*d", layout.numberWidth, n)
	}
	numbers := number(line.BeforeNumber) + " " + number(line.AfterNumber)

	var side diffStyles
	var marker string
	switch line.Kind {
	case diff.Removed:
		side, marker = st.diffRemoved, "-"
	case diff.Added:
		side, marker = st.diffAdded, "+"
	default:
		return st.muted.Render(numbers) + "     " + ansi.Cut(expandTabs(line.Text), x, x+layout.contentWidth)
	}
	rendered := side.number.Render(numbers+" ") + side.line.Render(" ") + side.marker.Render(marker) + side.line.Render("  ") +
		side.renderContent(line, x, layout.contentWidth)
	return rendered + side.line.Render(strings.Repeat(" ", max(layout.rowWidth-lipgloss.Width(rendered), 0)))
}

// renderContent 는 내용 칸에 보이는 부분만 잘라 그린다. 바뀐 단어는 word 배경으로, 나머지는 line 배경으로 칠한다.
// 탭은 펼친 칸마다 원래 탭과 같은 배경을 칠한다. 폭이 두 칸인 글자가 칸 경계에 걸치면 반쪽만 그릴 수 없어서 뺀다.
func (side diffStyles) renderContent(line diff.Line, x, width int) string {
	var out, segment strings.Builder
	highlighted := false
	flush := func() {
		if segment.Len() == 0 {
			return
		}
		style := side.line
		if highlighted {
			style = side.word
		}
		out.WriteString(style.Render(segment.String()))
		segment.Reset()
	}

	column := 0
	for i, r := range line.Text {
		cell, cells := string(r), 1
		if r == '\t' {
			cell, cells = " ", tabWidth
		}
		cellWidth := ansi.StringWidth(cell)
		changed := slices.ContainsFunc(line.Changes, func(c diff.Range) bool { return i >= c.Start && i < c.End })
		for range cells {
			if column >= x && column+cellWidth <= x+width {
				if changed != highlighted {
					flush()
					highlighted = changed
				}
				segment.WriteString(cell)
			}
			column += cellWidth
		}
	}
	flush()
	return out.String()
}

// renderScrollbarCell 은 본문 오른쪽 스크롤 막대의 row번째 칸이다. 지금 보이는 범위는 █, 나머지는 ░로 그린다.
func (st styles) renderScrollbarCell(row, height, offset, total int) string {
	thumb := max(height*height/total, 1)
	start := 0
	if total > height {
		start = (offset*(height-thumb) + (total-height)/2) / (total - height)
	}
	if row >= start && row < start+thumb {
		return st.muted.Render("█")
	}
	return st.muted.Render("░")
}
