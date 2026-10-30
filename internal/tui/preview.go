package tui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/diff"
	"github.com/4anghyeon/itsmydot/internal/github"
)

// previewScreen 은 미리보기 창의 내용과 스크롤 위치다.
// 터미널 크기가 바뀌면 보이는 범위도 달라지므로 위치는 그릴 때마다 다시 맞춘다.
type previewScreen struct {
	name             string   // 미리 보는 항목의 이름. 비어 있으면 닫혀 있다
	fromRepo         bool     // 로컬 사본 대신 저장소 파일을 보여 주면 true
	loading          bool     // 저장소 파일을 받는 중이면 true
	lines            []string // 파일 내용을 줄바꿈 기준으로 나눈 것
	note             string   // 내용 대신 보여 줄 안내. 이때 lines는 비어 있다
	failed           bool     // note가 받기 실패를 알리면 true
	yOffset, xOffset int
}

// previewLayout 은 지금 크기에서 미리보기 본문을 어떻게 그릴지다.
type previewLayout struct {
	width, height int  // 본문 폭과 높이. 폭에는 스크롤 막대 칸이 들어 있다
	numberWidth   int  // 줄 번호 칸의 폭
	contentWidth  int  // 줄 번호를 뺀 내용 칸의 폭
	maxXOffset    int  // 가장 긴 줄이 끝까지 보이는 가로 위치
	scrollbar     bool // 한 화면에 다 들어가지 않으면 true
}

// previewReadOnlyText 는 미리보기 창 아래에 두는 안내다. 동기화는 저장소에서 이 컴퓨터로 한 방향이라 여기서는 고칠 수 없다.
const previewReadOnlyText = "읽기 전용이에요. 고치려면 GitHub 저장소에서 편집해 주세요"

// openPreview 는 커서 항목의 미리보기 창을 연다.
// 로컬 사본이 시작할 때 본 저장소 파일과 같으면 사본을 읽고 아니면 저장소 파일을 보여 준다.
// 저장소 파일은 처음 열 때만 백그라운드로 받고 그 뒤로는 캐시에서 꺼낸다.
func (m Model) openPreview() (Model, tea.Cmd) {
	if !m.ready || len(m.items) == 0 {
		return m, nil
	}
	it := m.items[m.cursor]
	m.preview = previewScreen{name: it.entry.Name}
	if data, err := os.ReadFile(it.copyPath); err == nil && m.matchesRepo(it.entry.Source, data) {
		return m.showPreview(data, nil), nil
	}

	m.preview.fromRepo = true
	key := path.Clean(it.entry.Source)
	if data, ok := m.previewCache[key]; ok {
		return m.showPreview(data, nil), nil
	}
	m.preview.loading = true
	entry, client := it.entry, m.client
	load := func() tea.Msg {
		data, err := client.Fetch(context.Background(), entry.Source)
		return previewLoadedMsg{name: entry.Name, key: key, data: data, err: err}
	}
	return m, tea.Batch(load, m.spinner.Tick)
}

// matchesRepo 는 data가 시작할 때 본 저장소의 source 파일과 같은지 알려 준다.
// 저장소 해시를 받지 못했으면 같은지 알 수 없으므로 다르다고 보고 저장소 파일을 보여 준다.
func (m Model) matchesRepo(source string, data []byte) bool {
	remote, ok := m.hashes[path.Clean(source)]
	return ok && github.BlobHash(data) == remote
}

// applyPreview 는 받아 온 저장소 파일을 캐시에 넣고 창이 아직 그 항목을 기다리고 있으면 보여 준다.
// 받는 사이에 창을 닫았거나 다른 항목을 열었으면 늦게 온 결과는 보여 주지 않는다. 내용은 맞으니 캐시에는 넣는다.
func (m Model) applyPreview(msg previewLoadedMsg) Model {
	if msg.err == nil {
		m.previewCache = maps.Clone(m.previewCache)
		if m.previewCache == nil {
			m.previewCache = make(map[string][]byte)
		}
		m.previewCache[msg.key] = msg.data
	}
	if !m.preview.loading || m.preview.name != msg.name {
		return m
	}
	return m.showPreview(msg.data, msg.err)
}

// showPreview 는 data를 창에 보여 줄 줄로 나눈다. 받지 못했거나 줄 단위로 보여 줄 수 없는 파일이면 안내로 대신한다.
func (m Model) showPreview(data []byte, err error) Model {
	m.preview.loading = false
	if err == nil {
		err = diff.CheckText(data)
	}
	switch {
	case errors.Is(err, diff.ErrBinary):
		m.preview.note = "텍스트가 아닌 파일이라서 미리 볼 수 없어요"
	case errors.Is(err, diff.ErrTooLarge):
		m.preview.note = fmt.Sprintf("파일이 %dMB보다 커서 미리 볼 수 없어요", diff.MaxSize>>20)
	case err != nil:
		m.preview.note, m.preview.failed = "저장소 파일을 받지 못했어요 ("+errorReason(err, m.home)+")", true
	case len(data) == 0:
		m.preview.note = "빈 파일이에요"
	default:
		m.preview.lines = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	return m
}

// handlePreviewKey 는 미리보기 창의 키 입력을 처리한다. 스크롤은 비교 보기와 같은 키를 쓰고 tab이나 esc, q로 닫으면 목록으로 돌아간다.
func (m Model) handlePreviewKey(key string) (Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "tab", "esc", "q":
		m.preview = previewScreen{}
		return m, nil
	}
	layout := m.computePreviewLayout(newStyles(!m.light))
	y, x := scrollBy(key, m.preview.yOffset, m.preview.xOffset, layout.height, len(m.preview.lines))
	m.preview.yOffset, m.preview.xOffset = m.clampPreviewOffsets(layout, y, x)
	return m, nil
}

// clampPreviewOffsets 는 스크롤 위치를 내용이 있는 범위 안으로 자른다.
func (m Model) clampPreviewOffsets(layout previewLayout, y, x int) (int, int) {
	y = min(max(y, 0), max(len(m.preview.lines)-layout.height, 0))
	x = min(max(x, 0), layout.maxXOffset)
	return y, x
}

// computePreviewLayout 은 본문 폭과 높이를 정한다. 터미널 크기를 알면 머리와 제목, 안내, 도움말 줄을 빼고 남는 만큼을 쓴다.
func (m Model) computePreviewLayout(st styles) previewLayout {
	lines := m.preview.lines
	layout := previewLayout{height: min(len(lines), diffMaxHeight)}
	if m.height > 0 {
		// 맨 위 캐릭터 세 줄과 구분선, 빈 줄, 제목과 빈 줄, 빈 줄과 읽기 전용 안내, 도움말과 그 위 빈 줄,
		// 마지막 줄바꿈 뒤의 빈 줄로 12줄이다. 하위 항목은 제목이 그룹 줄까지 두 줄이다.
		reservedLines := 12 + strings.Count(m.renderPreviewTitle(st), "\n")
		layout.height = min(len(lines), max(m.height-reservedLines, 3))
	}
	layout.scrollbar = len(lines) > layout.height

	longest := 0
	for _, line := range lines {
		longest = max(longest, lipgloss.Width(expandTabs(line)))
	}
	layout.numberWidth = len(strconv.Itoa(max(len(lines), 1)))

	scrollbarWidth := 0
	if layout.scrollbar {
		scrollbarWidth = 2
	}
	prefix := previewPrefixWidth(layout.numberWidth)
	layout.width = prefix + longest + scrollbarWidth
	if m.width > 0 {
		// 본문 앞에 화면 왼쪽 여백 한 칸을 둔다.
		layout.width = max(m.width-1, 1)
	}
	layout.contentWidth = max(layout.width-scrollbarWidth-prefix, 1)
	layout.maxXOffset = max(longest-layout.contentWidth, 0)
	return layout
}

// previewPrefixWidth 는 내용 앞에 붙는 칸의 폭이다. 줄 번호와 두 칸 띄움이다.
func previewPrefixWidth(numberWidth int) int {
	return numberWidth + 2
}

// renderPreviewTitle 은 미리보기 창에서 어떤 항목의 어느 쪽 파일을 보여 주는지 알리는 제목이다.
func (m Model) renderPreviewTitle(st styles) string {
	origin := "로컬 사본"
	if m.preview.fromRepo {
		origin = "저장소 파일"
	}
	return m.renderItemTitle(st, m.indexOf(m.preview.name)) + "  " + st.muted.Render(origin)
}

// renderPreviewLines 는 미리보기 창의 제목과 본문, 읽기 전용 안내를 한 줄씩 만든다.
// 받는 중이거나 내용 대신 안내를 보여 줄 때는 본문 자리에 한 줄만 둔다.
func (m Model) renderPreviewLines(st styles, layout previewLayout) []string {
	lines := strings.Split(m.renderPreviewTitle(st), "\n")
	lines = append(lines, "")

	p := m.preview
	switch {
	case p.loading:
		lines = append(lines, st.stateStyle(syncing).Render(m.renderMarker(syncing))+" 저장소 파일을 받는 중이에요...")
	case p.failed:
		lines = append(lines, st.stateStyle(errored).Render(p.note))
	case p.note != "":
		lines = append(lines, st.muted.Render(p.note))
	default:
		y, x := m.clampPreviewOffsets(layout, p.yOffset, p.xOffset)
		for i, text := range p.lines[y : y+layout.height] {
			line := st.muted.Render(fmt.Sprintf("%*d", layout.numberWidth, y+i+1)) + "  " + ansi.Cut(expandTabs(text), x, x+layout.contentWidth)
			if layout.scrollbar {
				padding := max(layout.width-2-lipgloss.Width(line), 0)
				line += strings.Repeat(" ", padding) + " " + st.renderScrollbarCell(i, layout.height, y, len(p.lines))
			}
			lines = append(lines, line)
		}
	}
	return append(lines, "", st.muted.Render(previewReadOnlyText))
}

// previewView 는 목록 대신 미리보기 창을 화면 전체에 그린다.
func (m Model) previewView(st styles) tea.View {
	layout := m.computePreviewLayout(st)
	right := ""
	if len(m.preview.lines) > 0 {
		y, _ := m.clampPreviewOffsets(layout, m.preview.yOffset, m.preview.xOffset)
		right = st.muted.Render(fmt.Sprintf("%d/%d줄", y+layout.height, len(m.preview.lines)))
	}

	var lines []string
	for _, line := range m.renderPreviewLines(st, layout) {
		lines = append(lines, " "+line)
	}
	footer := st.renderHelp(m.width, "방향키", "스크롤", "pgup/pgdn", "한 쪽씩", "tab/esc", "닫기")
	return tea.NewView(st.renderPage(st.pageWidth(m.width, right, lines...), m.version, right, lines, footer))
}
