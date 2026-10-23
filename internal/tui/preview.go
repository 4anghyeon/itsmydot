package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/diff"
)

// previewScreen 은 파일 내용 미리보기 화면의 내용과 스크롤 위치다.
type previewScreen struct {
	name             string
	source           string
	lines            []string
	err              string
	loading          bool
	yOffset, xOffset int
}

// previewLoadedMsg 는 저장소에서 파일 내용을 받아 온 결과다.
type previewLoadedMsg struct {
	name   string
	source string
	data   []byte
	err    error
}

// previewLayout 은 지금 터미널 크기에서 미리보기 창을 어떻게 나눠 그릴지다.
type previewLayout struct {
	width, height int
	rowWidth      int
	numberWidth   int
	contentWidth  int
	maxXOffset    int
	scrollbar     bool
	split         bool
}

const (
	previewMinSplitWidth = 100
	previewMaxHeight     = 20
	readOnlyNotice       = "읽기 전용이에요. 고치려면 GitHub 저장소에서 편집해 주세요"
)

// openPreview 는 커서가 있는 항목의 파일 내용 미리보기를 열거나 이미 열려 있으면 닫는다.
func (m Model) openPreview() (Model, tea.Cmd) {
	if !m.ready || len(m.items) == 0 {
		return m, nil
	}
	if m.previewScreen.name != "" {
		m.previewScreen = previewScreen{}
		return m, nil
	}

	it := m.items[m.cursor]

	// 로컬 사본이 있고 저장소와 해시가 같으면 네트워크 요청 없이 사본을 바로 읽는다.
	hasCopy := false
	if it.copyPath != "" {
		if _, err := os.Stat(it.copyPath); err == nil {
			hasCopy = true
		}
	}

	if hasCopy && !it.outdated && !differsFromRepo(it.copyPath, it.entry.Source, m.fileHashes) {
		data, err := os.ReadFile(it.copyPath)
		if err == nil {
			return m.showPreviewContent(it.entry.Name, it.entry.Source, data, nil), nil
		}
	}

	// 캐시에 이미 있으면 저장소에 다시 요청하지 않는다.
	if cached, ok := m.previewCache[path.Clean(it.entry.Source)]; ok {
		return m.showPreviewContent(it.entry.Name, it.entry.Source, cached, nil), nil
	}

	// 로컬 사본이 없거나 업데이트가 있으면 저장소에서 최신본을 받는다.
	m.previewScreen = previewScreen{name: it.entry.Name, source: it.entry.Source, loading: true}
	entry, client := it.entry, m.client
	load := func() tea.Msg {
		data, err := client.Fetch(context.Background(), entry.Source)
		return previewLoadedMsg{name: entry.Name, source: entry.Source, data: data, err: err}
	}
	return m, tea.Batch(load, m.spinner.Tick)
}

// showPreviewContent 는 읽어온 파일 내용이나 에러를 미리보기 상태에 반영한다.
func (m Model) showPreviewContent(name, source string, data []byte, err error) Model {
	m.previewScreen = previewScreen{name: name, source: source}
	if err != nil {
		m.previewScreen.err = name + " 파일을 불러오지 못했어요 (" + errorReason(err, m.home) + ")"
		return m
	}
	if len(data) > diff.MaxSize {
		m.previewScreen.err = fmt.Sprintf("%s 파일이 %dMB보다 커서 미리 볼 수 없어요", name, diff.MaxSize>>20)
		return m
	}
	if bytes.IndexByte(data, 0) >= 0 {
		m.previewScreen.err = name + " 파일은 텍스트가 아니라서 미리 볼 수 없어요"
		return m
	}

	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	m.previewScreen.lines = lines
	return m
}

// applyPreview 는 저장소에서 파일 내용을 받아 온 결과를 반영한다.
func (m Model) applyPreview(msg previewLoadedMsg) Model {
	if msg.name != m.previewScreen.name || !m.previewScreen.loading {
		return m
	}
	if msg.err == nil {
		if m.previewCache == nil {
			m.previewCache = make(map[string][]byte)
		}
		m.previewCache[path.Clean(msg.source)] = msg.data
	}
	return m.showPreviewContent(msg.name, msg.source, msg.data, msg.err)
}

// handlePreviewKey 는 미리보기 화면의 키 입력을 처리한다.
func (m Model) handlePreviewKey(key string) (Model, tea.Cmd) {
	layout := m.computePreviewLayout(newStyles(!m.light))
	y, x := m.previewScreen.yOffset, m.previewScreen.xOffset
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "tab", "esc", "q":
		m.previewScreen = previewScreen{}
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
		y = len(m.previewScreen.lines)
	case "left", "h":
		x -= diffHorizontalStep
	case "right", "l":
		x += diffHorizontalStep
	}
	y = min(max(y, 0), max(len(m.previewScreen.lines)-layout.height, 0))
	x = min(max(x, 0), layout.maxXOffset)
	m.previewScreen.yOffset, m.previewScreen.xOffset = y, x
	return m, nil
}

// isSplitPreview 는 터미널 폭이 충분해서 좌우 분할로 그릴 수 있는지 알려 준다.
func (m Model) isSplitPreview() bool {
	return m.previewScreen.name != "" && m.width >= previewMinSplitWidth
}

// computePreviewLayout 은 미리보기 창 본문의 폭과 높이를 정한다.
func (m Model) computePreviewLayout(st styles) previewLayout {
	lines := m.previewScreen.lines
	layout := previewLayout{height: min(len(lines), previewMaxHeight)}
	if m.height > 0 {
		// 상단 캐릭터와 제목, 본문 안내 및 도움말 줄을 고려한 여백이다.
		reservedLines := 11
		layout.height = min(len(lines), max(m.height-reservedLines, 3))
	}
	layout.scrollbar = len(lines) > layout.height

	largestLineNumber, longestContent := 1, 0
	for _, line := range lines {
		longestContent = max(longestContent, lipgloss.Width(expandTabs(line)))
	}
	if len(lines) > 0 {
		largestLineNumber = len(lines)
	}
	layout.numberWidth = len(strconv.Itoa(largestLineNumber))

	scrollbarWidth := 0
	if layout.scrollbar {
		scrollbarWidth = 2
	}
	prefix := layout.numberWidth + 2

	if m.isSplitPreview() {
		layout.split = true
		listWidth := min(48, m.width*45/100)
		layout.width = max(m.width-listWidth-3, 20)
	} else if m.width > 0 {
		layout.width = max(m.width-2, 1)
	} else {
		layout.width = max(prefix+longestContent+scrollbarWidth, 40)
	}

	layout.rowWidth = layout.width - scrollbarWidth
	layout.contentWidth = max(layout.rowWidth-prefix, 1)
	layout.maxXOffset = max(longestContent-layout.contentWidth, 0)
	return layout
}

// renderPreviewPane 은 본문 줄들을 번호와 함께 그린다.
func (m Model) renderPreviewPane(st styles, layout previewLayout) []string {
	y, x := m.previewScreen.yOffset, m.previewScreen.xOffset
	if m.previewScreen.loading {
		return []string{" " + m.spinner.View() + " " + st.muted.Render("파일 내용을 불러오는 중이에요...")}
	}
	if m.previewScreen.err != "" {
		return []string{" " + st.stateStyle(errored).Render(m.previewScreen.err)}
	}
	if len(m.previewScreen.lines) == 0 {
		return []string{" " + st.muted.Render("파일이 비어 있어요")}
	}

	lines := m.previewScreen.lines
	end := min(y+layout.height, len(lines))
	body := make([]string, 0, layout.height)
	for i, line := range lines[y:end] {
		number := fmt.Sprintf("%*d", layout.numberWidth, i+y+1)
		row := st.muted.Render(number) + "  " + ansi.Cut(expandTabs(line), x, x+layout.contentWidth)
		if layout.scrollbar {
			padding := max(layout.width-2-lipgloss.Width(row), 0)
			row += strings.Repeat(" ", padding) + " " + st.renderScrollbarCell(i, layout.height, y, len(lines))
		}
		body = append(body, row)
	}
	return body
}

// previewFullScreenView 는 터미널 폭이 좁을 때 전체 화면으로 미리보기를 그린다.
func (m Model) previewFullScreenView(st styles) tea.View {
	layout := m.computePreviewLayout(st)
	title := m.renderItemTitle(st, m.indexOf(m.previewScreen.name))
	pane := m.renderPreviewPane(st, layout)

	lines := make([]string, 0, len(pane)+4)
	for line := range strings.SplitSeq(title, "\n") {
		lines = append(lines, " "+line)
	}
	lines = append(lines, "")
	for _, line := range pane {
		lines = append(lines, " "+line)
	}
	lines = append(lines, "")
	lines = append(lines, " "+st.muted.Render(readOnlyNotice))

	right := ""
	if !m.previewScreen.loading && m.previewScreen.err == "" && len(m.previewScreen.lines) > 0 {
		right = st.muted.Render(fmt.Sprintf("%d/%d줄", min(m.previewScreen.yOffset+layout.height, len(m.previewScreen.lines)), len(m.previewScreen.lines)))
	}
	footer := st.renderHelp(m.width, "방향키", "스크롤", "pgup/pgdn", "한 쪽씩", "tab/esc", "닫기")
	return tea.NewView(st.renderPage(st.pageWidth(m.width, right, lines...), m.version, right, lines, footer))
}

// splitPreviewView 는 터미널 폭이 넓을 때 목록과 미리보기를 좌우로 나란히 그린다.
func (m Model) splitPreviewView(st styles) tea.View {
	layout := m.computePreviewLayout(st)
	listWidth := min(48, m.width*45/100)

	// 왼쪽 목록 줄들을 준비한다.
	list, selected := m.renderItemLines(st)
	if selected >= 0 {
		list[selected] += lipgloss.NewStyle().Background(st.selection).Render(strings.Repeat(" ", max(listWidth-lipgloss.Width(list[selected]), 0)))
	}
	for i := range list {
		list[i] = ansi.Cut(list[i], 0, listWidth)
	}

	leftLines := append(slices.Clone(list), "", st.renderRule(listWidth), " "+m.renderDescription(st, m.items[m.cursor].entry.Description))
	leftBlock := strings.Join(leftLines, "\n")

	// 오른쪽 미리보기 창을 준비한다.
	pane := m.renderPreviewPane(st, layout)
	previewTitle := st.heading.Render(m.previewScreen.name)
	rightLines := []string{previewTitle, ""}
	rightLines = append(rightLines, pane...)
	rightLines = append(rightLines, "", st.muted.Render(readOnlyNotice))
	rightBlock := strings.Join(rightLines, "\n")

	combined := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, " │ ", rightBlock)
	lines := strings.Split(combined, "\n")

	right := ""
	if !m.previewScreen.loading && m.previewScreen.err == "" && len(m.previewScreen.lines) > 0 {
		right = st.muted.Render(fmt.Sprintf("%d/%d줄", min(m.previewScreen.yOffset+layout.height, len(m.previewScreen.lines)), len(m.previewScreen.lines)))
	}
	footer := st.renderHelp(m.width, "방향키", "스크롤", "pgup/pgdn", "한 쪽씩", "tab/esc", "닫기")
	return tea.NewView(st.renderPage(st.pageWidth(m.width, right, lines...), m.version, right, lines, footer))
}
