package brewfiles

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4anghyeon/itsmydot/internal/homebrew"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// brewMark 는 brewfiles 목록 체크박스의 상태다.
type brewMark int

const (
	brewNotInstalled brewMark = iota // [ ] 설치 안 됨
	brewInstalled                    // [+] 설치됨
	brewPicked                       // [*] 설치하려고 고름
)

// marker 는 체크박스 모양이다. dotfiles 목록처럼 ASCII만 쓴다.
func (b brewMark) marker() string {
	switch b {
	case brewInstalled:
		return "[+]"
	case brewPicked:
		return "[*]"
	default:
		return "[ ]"
	}
}

// brewMarkStyle 은 체크박스 색이다. 설치됨은 dotfiles의 연결됨과 같은 초록이고 고른 줄은 강조색이다.
func brewMarkStyle(st ui.Styles, b brewMark) lipgloss.Style {
	switch b {
	case brewInstalled:
		return st.StateStyle(ui.Synced)
	case brewPicked:
		return lipgloss.NewStyle().Bold(true).Foreground(st.Palette.Accent)
	default:
		return st.StateStyle(ui.NotSynced)
	}
}

// mark 는 i번째 줄의 체크박스 상태다.
// 프로필 줄은 고른 하위 패키지가 있으면 [*], 하위 패키지가 모두 설치돼 있으면 [+]다.
func (m List) mark(i int) brewMark {
	it := m.items[i]
	if !it.isProfile() {
		switch {
		case m.installed.Has(it.pkg):
			return brewInstalled
		case m.picked[it.pkg.Key()]:
			return brewPicked
		}
		return brewNotInstalled
	}
	children := ui.TreeChildren(m.items, i)
	if slices.ContainsFunc(children, func(j int) bool { return m.mark(j) == brewPicked }) {
		return brewPicked
	}
	if len(children) > 0 && !slices.ContainsFunc(children, func(j int) bool { return m.mark(j) != brewInstalled }) {
		return brewInstalled
	}
	return brewNotInstalled
}

// brewScrollbarMargin 은 목록 스크롤 막대와 화면 오른쪽 끝 사이의 칸 수다.
// 맨 끝 칸에 붙이면 터미널 창의 스크롤 막대와 겹쳐 보인다.
const brewScrollbarMargin = 2

// View 는 프로필별 패키지 목록과 그 아래에 커서 줄의 설명이나 설치 확인 질문, 상태 줄을 그린다.
// Homebrew가 없으면 목록 대신 설치 방법을 안내한다.
//
// 인라인 화면은 터미널보다 높아지면 렌더러가 위쪽 줄부터 버려서 제목과 목록 앞부분이 사라진다.
// 그래서 터미널 높이를 알면 목록을 남는 높이만큼만 잘라 그린다.
func (m List) View() tea.View {
	st := ui.NewStyles(!m.light)
	if !m.showsList() {
		return tea.NewView(m.renderScreen(st, m.renderNoticeLines(st), -1, nil))
	}

	list, selected := m.renderBrewLines(st)
	height := m.listHeight(st)
	if height >= len(list) {
		return tea.NewView(m.renderScreen(st, list, selected, nil))
	}
	offset := ui.ClampListOffset(m.offset, selected, height, len(list))
	scrollbar := make([]string, height)
	for i := range scrollbar {
		scrollbar[i] = st.RenderScrollbarCell(i, height, offset, len(list))
	}
	return tea.NewView(m.renderScreen(st, list[offset:offset+height], selected-offset, scrollbar))
}

// Summary 는 설치된 패키지 수와 전체 수를 게이지와 함께 그린다. 화면 머리와 진입 메뉴 카드에 쓴다.
// 목록을 보여 줄 수 없거나 패키지가 하나도 없으면 빈 문자열이다.
func (m List) Summary(st ui.Styles) string {
	installed, total := m.installedCount()
	if !m.showsList() || total == 0 {
		return ""
	}
	return st.Muted.Render(fmt.Sprintf("설치됨 %d/%d ", installed, total)) + st.RenderGauge(ui.Ratio(installed, total))
}

// showsList 는 패키지 목록을 보여 줄 수 있으면 true다. 아니면 목록 자리에 안내를 보여 준다.
func (m List) showsList() bool {
	return m.ready && !m.missing && m.loadErr == nil && len(m.items) > 0
}

// listHeight 는 목록 창에 쓸 줄 수다. 터미널 높이를 모르면 목록 전체 줄 수다.
// 목록 아래 설명과 확인 질문, 상태 줄은 폭에 따라 줄바꿈되므로 목록을 뺀 화면을 실제로 그려서 남는 높이를 잰다.
// 확인 질문이 떠도 화면이 넘치지 않도록 낮은 터미널에서는 커서 줄 하나까지 줄인다.
func (m List) listHeight(st ui.Styles) int {
	total := 0
	for i := range m.items {
		if ui.TreeVisible(m.items, m.collapsed, i) {
			total++
		}
	}
	if m.height <= 0 {
		return total
	}
	rest := lipgloss.Height(m.renderScreen(st, nil, -1, nil))
	return max(m.height-rest, 1)
}

// followCursor 는 커서 줄이 목록 창 안에 들어오도록 옮긴 창의 맨 위 위치다.
// 커서가 창 안에 있으면 그대로 두어 커서를 옮길 때마다 목록이 출렁이지 않게 한다.
func (m List) followCursor() int {
	if !m.showsList() {
		return 0
	}
	st := ui.NewStyles(!m.light)
	list, selected := m.renderBrewLines(st)
	return ui.ClampListOffset(m.offset, selected, m.listHeight(st), len(list))
}

// renderNoticeLines 는 목록을 보여 줄 수 없을 때 목록 자리에 놓을 안내다.
func (m List) renderNoticeLines(st ui.Styles) []string {
	switch {
	case !m.ready:
		return []string{" " + st.Muted.Render("Brewfile 불러오는 중이에요...")}
	case m.missing:
		return m.renderMissingLines(st)
	case m.loadErr != nil:
		lines := []string{" " + st.StateStyle(ui.Errored).Render("Brewfile을 불러오지 못했어요. "+ui.ErrorReason(m.loadErr, ""))}
		for line := range strings.SplitSeq(m.loadErr.Error(), "\n") {
			lines = append(lines, " "+st.StateStyle(ui.Errored).Render(line))
		}
		return lines
	default:
		return []string{" " + st.Muted.Render("등록된 Brewfile이 없어요")}
	}
}

// renderScreen 은 목록 창 list를 넣어 화면 전체를 그린다. selected는 list 안에서 커서가 있는 줄이고 없으면 -1이다.
// scrollbar가 있으면 list의 각 줄 오른쪽 끝에 그 칸을 붙인다.
func (m List) renderScreen(st ui.Styles, list []string, selected int, scrollbar []string) string {
	list = slices.Clone(list)

	right := m.Summary(st)
	var lower []string
	if m.showsList() {
		// 설명이 없어도 줄은 남겨서 커서를 옮길 때 화면 높이가 출렁이지 않게 한다.
		lower = append(lower, " "+st.RenderDescription(m.items[m.cursor].description(), m.width))
	}

	footer := st.RenderHelp(m.width, "방향키", "이동", "space", "고르기", "enter", "설치", "esc", "진입 메뉴")
	if !m.showsList() {
		footer = st.RenderHelp(m.width, "esc", "진입 메뉴")
	}
	status := m.Status
	if m.confirming {
		var numbers []string
		lower, numbers = m.renderInstallConfirmLines(st)
		footer = st.RenderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
		status = ""
	}

	marker, tone := "", ui.NotSynced
	if status != "" {
		marker, tone = ui.ToneMarker(m.StatusTone, ui.Syncing.Marker())
	}

	// 스크롤 막대는 목록과 한 칸 띄우고 화면 오른쪽 끝과도 brewScrollbarMargin칸 띄운다.
	scrollbarWidth := 0
	if scrollbar != nil {
		scrollbarWidth = 2 + brewScrollbarMargin
	}
	measured := slices.Concat(lower, []string{" " + marker + " " + status})
	for _, line := range list {
		measured = append(measured, line+strings.Repeat(" ", scrollbarWidth))
	}
	width := st.PageWidth(m.width, right, measured...)
	rowWidth := width - scrollbarWidth
	for i := range list {
		padding := strings.Repeat(" ", max(rowWidth-lipgloss.Width(list[i]), 0))
		if i == selected {
			// 선택한 줄은 배경을 목록 끝까지 칠한다.
			padding = lipgloss.NewStyle().Background(st.Selection).Render(padding)
		}
		if i == selected || scrollbar != nil {
			list[i] += padding
		}
		if scrollbar != nil {
			list[i] += " " + scrollbar[i] + strings.Repeat(" ", brewScrollbarMargin)
		}
	}
	lines := append(append(list, "", st.RenderRule(width)), lower...)
	lines = append(lines, "", st.RenderStatusLine(width, marker, st.StateColor(tone), status))
	return st.RenderPage(width, m.version, right, lines, footer)
}

// renderBrewLines 는 프로필과 패키지를 한 줄씩 만들고 커서가 있는 줄의 위치를 함께 돌려준다.
//
//	▌ v [ ] base       3/18
//	  ├── [+] zsh      brew
//	  ├── [*] raycast  cask
//	  └── [ ] switch   cask
func (m List) renderBrewLines(st ui.Styles) (lines []string, selected int) {
	rows := make([]ui.ListRow, 0, len(m.items))
	for i, it := range m.items {
		if !ui.TreeVisible(m.items, m.collapsed, i) {
			continue
		}
		mark := m.mark(i)
		row := ui.ListRow{
			Fold:        ui.TreeFoldMarker(m.items, m.collapsed, i),
			Branch:      ui.TreeBranch(m.items, i),
			Marker:      mark.marker(),
			MarkerStyle: brewMarkStyle(st, mark),
			Name:        it.NodeName(),
			Current:     i == m.cursor,
		}
		if it.isProfile() {
			children := ui.TreeChildren(m.items, i)
			installed := 0
			for _, j := range children {
				if m.installed.Has(m.items[j].pkg) {
					installed++
				}
			}
			row.Tail = append(row.Tail, ui.ListSegment{Text: fmt.Sprintf("%d/%d", installed, len(children)), Style: st.Count})
		} else {
			row.Detail = string(it.pkg.Kind)
		}
		rows = append(rows, row)
	}
	return st.RenderRows(rows)
}

// renderInstallConfirmLines 는 설치 확인 질문과 고른 패키지 이름, 번호 선택지를 한 줄씩 만든다.
func (m List) renderInstallConfirmLines(st ui.Styles) (lines, numbers []string) {
	packages := m.pickedPackages()
	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	lines = []string{
		" " + st.Question.Render(fmt.Sprintf("%d개 패키지를 설치할까요?", len(packages))),
		" " + st.Muted.Render(strings.Join(names, ", ")),
		"",
	}
	options, numbers := st.RenderOptions(brewConfirmOptions, m.choice)
	return append(lines, options...), numbers
}

// renderMissingLines 는 Homebrew가 없을 때 목록 대신 보여 줄 안내다. itsmydot은 Homebrew를 직접 설치하지 않는다.
//
// 설치 스크립트는 셸 설정을 고치지 않고 끝에 Next steps 명령만 보여 준다.
// Apple Silicon Mac에서는 그 명령을 실행해야 새 셸에서 brew가 PATH에 잡히므로 함께 안내한다.
// PATH 설정은 Homebrew 안내대로 각 Mac의 ~/.zprofile에 두고 itsmydot은 관리하지 않는다.
func (m List) renderMissingLines(st ui.Styles) []string {
	return []string{
		" " + st.Question.Render("Homebrew가 필요해요"),
		"",
		" " + "터미널에서 아래 명령으로 Homebrew를 설치해 주세요.",
		"",
		"   " + st.Key.Render(homebrew.InstallGuide),
		"",
		" " + "설치가 끝나면 화면 끝에 나오는 Next steps 명령도 실행해 주세요.",
		" " + "새 셸에서 brew와 brew로 설치한 도구를 찾으려면 이 단계가 필요해요.",
		" " + "그다음 itsmydot을 다시 실행해 주세요.",
		"",
		" " + st.Muted.Render("자세한 내용은 https://brew.sh 를 참고해 주세요"),
	}
}
