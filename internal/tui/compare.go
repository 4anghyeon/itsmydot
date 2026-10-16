package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/diff"
	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// diffScreen 은 비교 보기 화면의 내용과 스크롤 위치다.
// 터미널 크기가 바뀌면 보이는 범위도 달라지므로 위치는 그릴 때마다 다시 맞춘다.
type diffScreen struct {
	name             string
	lines            []diffLine
	yOffset, xOffset int
}

// diffLine 은 diff 한 줄이다. kind는 줄 맨 앞 글자로 + 와 - 와 공백, @ 와 \ 중 하나다.
// 줄 번호가 없는 쪽은 0이다. 지운 줄은 사본에만, 추가한 줄은 리포에만 번호가 있다.
type diffLine struct {
	kind                      byte
	beforeNumber, afterNumber int
	text                      string
	changes                   []diff.Range // 짝지은 줄과 비교해 바뀐 단어의 위치. 짝이 없으면 nil이다
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

// diffLoadedMsg 는 로컬 사본과 리포 최신본을 비교한 결과다.
type diffLoadedMsg struct {
	name string
	text string // unified diff. 두 파일이 같으면 빈 문자열이다
	err  error
}

// diffHorizontalStep 은 좌우 방향키 한 번에 옆으로 미는 칸 수다.
const diffHorizontalStep = 8

// diffMaxHeight 는 터미널 높이를 모를 때 diff 본문을 보여 줄 최대 줄 수다.
const diffMaxHeight = 20

// startCompare 는 i번째 항목의 로컬 사본과 비교할 리포 최신본을 백그라운드로 받는다.
// 업데이트 표시에 쓰는 해시만으로는 내용을 알 수 없어서 비교할 때마다 새로 받는다.
func (m Model) startCompare(i int) (Model, tea.Cmd) {
	it := m.items[i]
	m.comparing = it.entry.Name
	m.status = it.entry.Name + " 리포 최신본과 비교하는 중이에요..."
	entry, copyPath, client := it.entry, it.copyPath, m.client
	load := func() tea.Msg {
		return compareCopy(context.Background(), client, copyPath, entry)
	}
	return m, tea.Batch(load, m.spinner.Tick)
}

// compareCopy 는 리포 최신본을 받아 로컬 사본과의 diff를 만든다. 디스크는 바꾸지 않는다.
func compareCopy(ctx context.Context, client *github.Client, copyPath string, e manifest.Entry) diffLoadedMsg {
	msg := diffLoadedMsg{name: e.Name}
	remote, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	local, err := os.ReadFile(copyPath)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.text, msg.err = diff.Unified("사본", "리포", local, remote)
	return msg
}

// applyDiff 는 비교 결과로 비교 보기 화면을 연다. 차이가 없거나 비교할 수 없으면 화면 대신 상태 메시지로 알린다.
// 받는 사이에 다른 항목의 비교를 시작했으면 늦게 온 결과는 버린다.
func (m Model) applyDiff(msg diffLoadedMsg) Model {
	if msg.name != m.comparing {
		return m
	}
	m.comparing = ""
	switch {
	case errors.Is(msg.err, diff.ErrBinary):
		m.status = msg.name + " 파일은 텍스트가 아니라서 비교할 수 없어요"
	case errors.Is(msg.err, diff.ErrTooLarge):
		m.status = fmt.Sprintf("%s 파일이 %dMB보다 커서 비교할 수 없어요", msg.name, diff.MaxSize>>20)
	case msg.err != nil:
		m.status = msg.name + " 비교하지 못했어요 (" + msg.err.Error() + ")"
	case msg.text == "":
		m.status = msg.name + " 사본과 리포 최신본이 같아요"
	default:
		m.status = ""
		m.diff = diffScreen{name: msg.name, lines: diffLines(msg.text)}
	}
	return m
}

// hunkHeader 는 @@ -1,5 +1,6 @@ 에서 사본과 리포의 시작 줄 번호를 꺼낸다. 줄 수가 1이면 쉼표 부분이 생략된다.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// diffLines 는 unified diff를 줄 번호가 붙은 화면 줄로 나눈다. 파일 이름 머리줄은 화면 제목이 대신하므로 뺀다.
// 탭은 터미널마다 폭이 달라 상자 테두리가 어긋나므로 공백으로 바꾼다.
func diffLines(text string) []diffLine {
	raw := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(raw) >= 2 && strings.HasPrefix(raw[0], "--- ") && strings.HasPrefix(raw[1], "+++ ") {
		raw = raw[2:]
	}

	lines := make([]diffLine, 0, len(raw))
	var before, after int
	for _, line := range raw {
		line = strings.ReplaceAll(line, "\t", "    ")
		if line == "" {
			line = " " // 빈 문맥 줄은 앞의 공백까지 지워져서 올 수 있다
		}
		switch kind := line[0]; kind {
		case '@':
			if match := hunkHeader.FindStringSubmatch(line); match != nil {
				before, _ = strconv.Atoi(match[1])
				after, _ = strconv.Atoi(match[2])
			}
			lines = append(lines, diffLine{kind: kind, text: line})
		case '\\':
			lines = append(lines, diffLine{kind: kind, text: line})
		case '-':
			lines = append(lines, diffLine{kind: kind, beforeNumber: before, text: line[1:]})
			before++
		case '+':
			lines = append(lines, diffLine{kind: kind, afterNumber: after, text: line[1:]})
			after++
		default:
			lines = append(lines, diffLine{kind: ' ', beforeNumber: before, afterNumber: after, text: line[1:]})
			before++
			after++
		}
	}
	markWordChanges(lines)
	return lines
}

// markWordChanges 는 연달아 지운 줄 뒤에 바로 추가한 줄이 오면 순서대로 한 줄씩 짝지어 바뀐 단어를 표시한다.
// 사이에 끼는 \ No newline at end of file 줄은 건너뛴다. 짝이 모자란 줄은 줄 전체가 바뀐 것으로 둔다.
func markWordChanges(lines []diffLine) {
	for i := 0; i < len(lines); {
		removedEnd := i
		for removedEnd < len(lines) && lines[removedEnd].kind == '-' {
			removedEnd++
		}
		if removedEnd == i {
			i++
			continue
		}
		addedStart := removedEnd
		for addedStart < len(lines) && lines[addedStart].kind == '\\' {
			addedStart++
		}
		addedEnd := addedStart
		for addedEnd < len(lines) && lines[addedEnd].kind == '+' {
			addedEnd++
		}
		for k := 0; k < min(removedEnd-i, addedEnd-addedStart); k++ {
			removed, added := &lines[i+k], &lines[addedStart+k]
			removed.changes, added.changes = diff.Words(removed.text, added.text)
		}
		i = max(addedEnd, removedEnd)
	}
}

// scrollDiff 는 비교 보기 화면의 키 입력을 처리한다. esc나 q로 닫으면 화면을 열기 전으로 돌아간다.
func (m Model) scrollDiff(key string) (Model, tea.Cmd) {
	layout := m.diffLayout(newStyles(!m.light))
	y, x := m.diff.yOffset, m.diff.xOffset
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.diff = diffScreen{}
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
		y = len(m.diff.lines)
	case "left", "h":
		x -= diffHorizontalStep
	case "right", "l":
		x += diffHorizontalStep
	}
	m.diff.yOffset, m.diff.xOffset = m.clampDiffOffsets(layout, y, x)
	return m, nil
}

// clampDiffOffsets 는 스크롤 위치를 내용이 있는 범위 안으로 자른다.
func (m Model) clampDiffOffsets(layout diffLayout, y, x int) (int, int) {
	y = min(max(y, 0), max(len(m.diff.lines)-layout.height, 0))
	x = min(max(x, 0), layout.maxXOffset)
	return y, x
}

// diffLayout 은 본문 폭과 높이를 정한다. 폭은 가장 긴 줄에 맞추되 터미널을 넘지 않게 하고
// 높이는 줄 수에 맞추되 상자 테두리와 제목, 도움말 줄을 빼고 남는 만큼을 넘지 않는다.
func (m Model) diffLayout(st styles) diffLayout {
	title := m.diffTitle(st)
	layout := diffLayout{height: min(len(m.diff.lines), diffMaxHeight)}
	if m.height > 0 {
		// 위아래 테두리, 맨 위 줄과 빈 줄, 제목과 빈 줄, 상자 아래 도움말, 마지막 줄바꿈 뒤의 빈 줄로 8줄이다.
		// 하위 항목은 제목이 그룹 줄까지 두 줄이다.
		chrome := 8 + strings.Count(title, "\n")
		layout.height = min(len(m.diff.lines), max(m.height-chrome, 3))
	}
	layout.scrollbar = len(m.diff.lines) > layout.height

	// @@ 줄은 +/- 표시 자리부터 그리므로 내용 폭과 따로 센다.
	largest, longest, hunk := 1, 0, 0
	for _, line := range m.diff.lines {
		largest = max(largest, line.beforeNumber, line.afterNumber)
		if line.kind == '@' {
			hunk = max(hunk, lipgloss.Width(line.text))
		} else {
			longest = max(longest, lipgloss.Width(line.text))
		}
	}
	layout.numberWidth = len(strconv.Itoa(largest))

	scrollbarWidth := 0
	if layout.scrollbar {
		scrollbarWidth = 2
	}
	prefix := diffPrefixWidth(layout.numberWidth)
	hunk += diffHunkIndent(layout.numberWidth)
	layout.width = max(lipgloss.Width(title), prefix+longest+scrollbarWidth, hunk+scrollbarWidth)
	if m.width > 0 && layout.width+frameExtra > m.width {
		layout.width = max(m.width-frameExtra, 1)
	}
	layout.rowWidth = layout.width - scrollbarWidth
	layout.contentWidth = max(layout.rowWidth-prefix, 1)
	layout.maxXOffset = max(longest-layout.contentWidth, 0)
	return layout
}

// diffPrefixWidth 는 내용 앞에 붙는 칸의 폭이다. 사본 번호와 리포 번호, 두 칸 띈 +/- 표시, 다시 두 칸이다.
func diffPrefixWidth(numberWidth int) int {
	return diffHunkIndent(numberWidth) + 1 + 2
}

// diffHunkIndent 는 +/- 표시가 놓이는 위치다. @@ 줄도 여기서부터 쓴다.
func diffHunkIndent(numberWidth int) int {
	return numberWidth*2 + 1 + 2
}

// diffTitle 은 비교 보기 화면에서 어떤 항목을 비교하는지 보여 주는 제목이다.
func (m Model) diffTitle(st styles) string {
	return m.itemTitle(st, m.indexOf(m.diff.name))
}

// diffView 는 목록 대신 비교 보기 화면을 그린다.
func (m Model) diffView(st styles) tea.View {
	layout := m.diffLayout(st)
	y, x := m.clampDiffOffsets(layout, m.diff.yOffset, m.diff.xOffset)
	visible := m.diff.lines[y : y+layout.height]

	body := make([]string, len(visible))
	for i, line := range visible {
		body[i] = st.diffRow(line, layout, x)
		if layout.scrollbar {
			padding := max(layout.width-2-lipgloss.Width(body[i]), 0)
			body[i] += strings.Repeat(" ", padding) + " " + st.scrollbarCell(i, layout.height, y, len(m.diff.lines))
		}
	}

	title := st.title.Render("itsmydot")
	right := st.muted.Render(fmt.Sprintf("%d/%d줄", y+layout.height, len(m.diff.lines)))
	header := title + strings.Repeat(" ", max(layout.width-lipgloss.Width(title)-lipgloss.Width(right), 1)) + right
	content := header + "\n\n" + m.diffTitle(st) + "\n\n" + strings.Join(body, "\n")
	box := st.frame.Width(layout.width + frameExtra).Render(content)
	footer := st.help("방향키", "스크롤", "pgup/pgdn", "한 쪽씩", "esc", "닫기")
	return tea.NewView(box + "\n " + footer + "\n")
}

// diffRow 는 diff 한 줄을 줄 번호와 함께 그린다. 가로로 밀 때는 내용만 밀고 줄 번호는 그 자리에 둔다.
// 지운 줄과 추가한 줄은 줄 끝까지 배경을 칠하고 줄 번호 칸은 조금 더 진하게, 바뀐 단어는 가장 진하게 칠한다.
//
//	1 1     [user]
//	2    -      name = me
//	  2  +      name = you
func (st styles) diffRow(line diffLine, layout diffLayout, x int) string {
	indent := diffHunkIndent(layout.numberWidth)
	if line.kind == '@' {
		text := ansi.Truncate(strings.Repeat(" ", indent)+line.text, layout.rowWidth, "")
		return st.diffHunk.Render(text + strings.Repeat(" ", max(layout.rowWidth-lipgloss.Width(text), 0)))
	}

	number := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", layout.numberWidth)
		}
		return fmt.Sprintf("%*d", layout.numberWidth, n)
	}
	numbers := number(line.beforeNumber) + " " + number(line.afterNumber)

	var side diffStyles
	switch line.kind {
	case '-':
		side = st.diffRemoved
	case '+':
		side = st.diffAdded
	case '\\':
		return st.muted.Render(numbers) + strings.Repeat(" ", indent-len(numbers)+3) + st.muted.Render(ansi.Cut(line.text, x, x+layout.contentWidth))
	default:
		return st.muted.Render(numbers) + strings.Repeat(" ", indent-len(numbers)+3) + ansi.Cut(line.text, x, x+layout.contentWidth)
	}

	row := side.number.Render(numbers+" ") + side.line.Render(" ") + side.marker.Render(string(line.kind)) + side.line.Render("  ") +
		side.content(line, x, layout.contentWidth)
	return row + side.line.Render(strings.Repeat(" ", max(layout.rowWidth-lipgloss.Width(row), 0)))
}

// content 는 내용 칸에 보이는 부분만 잘라 그린다. 바뀐 단어는 word 배경으로, 나머지는 line 배경으로 칠한다.
// 폭이 두 칸인 글자가 칸 경계에 걸치면 반쪽만 그릴 수 없어서 뺀다.
func (side diffStyles) content(line diffLine, x, width int) string {
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
	for i, r := range line.text {
		w := ansi.StringWidth(string(r))
		if column >= x && column+w <= x+width {
			changed := slices.ContainsFunc(line.changes, func(c diff.Range) bool { return i >= c.Start && i < c.End })
			if changed != highlighted {
				flush()
				highlighted = changed
			}
			segment.WriteRune(r)
		}
		column += w
	}
	flush()
	return out.String()
}

// scrollbarCell 은 본문 오른쪽 스크롤 막대의 row번째 칸이다. 지금 보이는 범위는 █, 나머지는 ░로 그린다.
func (st styles) scrollbarCell(row, height, offset, total int) string {
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
