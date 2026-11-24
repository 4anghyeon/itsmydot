package dotfiles

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// View 는 항목 목록과 그 아래에 커서 항목의 설명이나 확인 질문, 상태 줄을 그린다.
// 도움말이나 비교 보기 화면이 열려 있으면 그 화면을 그린다.
// 미리보기 창은 터미널이 넓으면 목록 오른쪽에 붙이고 좁으면 목록 대신 화면 전체에 그린다.
func (m List) View() tea.View {
	st := ui.NewStyles(!m.light)
	if m.helpOpen {
		return m.helpView(st)
	}
	if m.diffScreen.name != "" {
		return m.diffView(st)
	}
	listWidth, split := m.previewSplit(st)
	if m.preview.name != "" && !split {
		return m.previewView(st)
	}

	right := m.renderSummary(st, m.gauge)

	// 목록 옆에 미리보기 창을 붙이면 설명과 구분선, 상태 줄은 화면 대신 목록 칸 폭에 맞춘다.
	textWidth := m.width
	if split {
		textWidth = listWidth
	}

	var list, lower []string
	selected := -1
	switch {
	case !m.ready:
		list = append(list, " "+st.Muted.Render("manifest 불러오는 중이에요..."))
	case m.loadErr != nil:
		list = append(list, " "+st.StateStyle(ui.Errored).Render("manifest를 불러오지 못했어요. "+ui.ErrorReason(m.loadErr, m.home)))
		for line := range strings.SplitSeq(m.loadErr.Error(), "\n") {
			list = append(list, " "+st.StateStyle(ui.Errored).Render(line))
		}
	case len(m.items) == 0:
		list = append(list, " "+st.Muted.Render("manifest에 항목이 없어요"))
	default:
		list, selected = m.renderItemLines(st)
		// 설명이 없어도 줄은 남겨서 커서를 옮길 때 화면 높이가 출렁이지 않게 한다.
		lower = append(lower, " "+st.RenderDescription(m.items[m.cursor].entry.Description, textWidth))
	}

	footer := st.RenderHelp(m.width, "방향키", "이동", "space", "연결/연결 끊기", "enter", "메뉴", "tab", "미리보기", "?", "도움말", "esc", "진입 메뉴")
	status := m.Status
	if m.confirmingName != "" {
		var numbers []string
		lower, numbers = m.renderConfirmLines(st)
		footer = st.RenderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
		// 질문이 떠 있는 동안 상태 줄에 남은 이전 결과는 질문과 상관없어서 비워 둔다.
		status = ""
	}

	marker, tone := "", ui.NotSynced
	if status != "" {
		marker, tone = m.statusMarker()
	}

	width := st.PageWidth(m.width, right, append(slices.Concat(list, lower), " "+marker+" "+status)...)
	column := width
	if split {
		column = listWidth
	}
	if selected >= 0 {
		list[selected] += lipgloss.NewStyle().Background(st.Selection).Render(strings.Repeat(" ", max(column-lipgloss.Width(list[selected]), 0)))
	}
	// 설명이나 질문은 구분선 바로 아래에 붙여서 목록과 멀어지지 않게 한다.
	lines := append(append(list, "", st.RenderRule(column)), lower...)
	lines = append(lines, "", st.RenderStatusLine(column, marker, st.StateColor(tone), status))
	if split {
		lines, footer = m.joinPreview(st, lines, listWidth), m.renderPreviewHelp(st)
	}
	return tea.NewView(st.RenderPage(width, m.version, right, lines, footer))
}

// renderItemLines 는 항목 목록을 한 줄씩 만들고 커서가 있는 줄의 위치를 함께 돌려준다. 접힌 그룹의 하위 항목은 뺀다.
// 그룹 줄의 체크박스는 그룹 파일의 상태이고 켜진 하위 항목 수는 줄 끝에 따로 적는다.
// 폴더 그룹은 그룹 파일이 없어서 체크박스에 하위 항목 상태를 모아 그린다.
//
//	    [+] gitconfig  ~/.gitconfig
//	▌ v [+] zshrc      ~/.zshrc  1/2
//	  ├── [+] env      ~/.config/zsh/env.zsh
//	  └── [!] git      ~/.config/zsh/git.zsh   (다른 파일이 있음)
//	  > [ ] claude     ~/.claude/CLAUDE.md  0/3
func (m List) renderItemLines(st ui.Styles) (lines []string, selected int) {
	rows := make([]ui.ListRow, 0, len(m.items))
	for i, it := range m.items {
		if !m.visible(i) {
			continue
		}
		state := it.state
		if m.isFolder(i) {
			state = m.folderState(i)
		}
		row := ui.ListRow{
			Fold:        ui.TreeFoldMarker(m.items, m.collapsed, i),
			Branch:      ui.TreeBranch(m.items, i),
			Marker:      m.renderMarker(state),
			MarkerStyle: st.FlashStyle(state, it.flash),
			Name:        it.entry.Name,
			Detail:      ui.Tildify(it.entry.Target, m.home),
			Current:     i == m.cursor,
		}
		if children := m.children(i); len(children) > 0 {
			row.Tail = append(row.Tail, ui.ListSegment{Gap: "  ", Text: fmt.Sprintf("%d/%d", len(m.syncedChildren(i)), len(children)), Style: st.Count})
		}
		if it.state == ui.Synced && it.outdated {
			row.Tail = append(row.Tail, ui.ListSegment{Gap: "  ", Text: "^ 업데이트 있음", Style: st.Update})
		}
		// 접힌 그룹은 하위 항목 줄이 안 보이므로 하위 항목의 업데이트를 그룹 줄에 모아 보여 준다.
		if n := m.outdatedChildren(i); n > 0 && m.collapsed[it.entry.Name] {
			row.Tail = append(row.Tail, ui.ListSegment{Gap: "  ", Text: fmt.Sprintf("^ 하위 %d개 업데이트 있음", n), Style: st.Update})
		}

		switch it.state {
		case ui.Conflict:
			row.Tail = append(row.Tail, ui.ListSegment{Gap: "   ", Text: "(다른 파일이 있음)", Style: st.StateStyle(it.state)})
		case ui.Errored:
			row.Tail = append(row.Tail, ui.ListSegment{Gap: "   ", Text: "(" + ui.ErrorReason(it.err, m.home) + ")", Style: st.StateStyle(it.state)})
		case ui.NotSynced, ui.Synced, ui.Syncing:
			// 체크박스만으로 충분해서 덧붙일 설명이 없다.
		}
		rows = append(rows, row)
	}
	return st.RenderRows(rows)
}

// renderMarker 는 체크박스를 그린다. 받는 중이면 ~ 자리에 스피너의 현재 모양을 넣는다.
func (m List) renderMarker(s ui.ItemState) string {
	if s == ui.Syncing && len(m.spinner.Spinner.Frames) > 0 {
		return ui.RenderSpinner(m.spinner)
	}
	return s.Marker()
}

// renderItemTitle 은 i번째 항목의 제목이다. 메뉴와 비교 보기 화면이 같이 쓰고 하위 항목은 그룹 줄 아래에 매단다.
//
//	zshrc  ~/.zshrc
//	└─ env  ~/.config/zsh/env.zsh
func (m List) renderItemTitle(st ui.Styles, i int) string {
	if i < 0 {
		return ""
	}
	it := m.items[i]
	target := ui.Tildify(it.entry.Target, m.home)

	if len(m.children(i)) > 0 {
		return m.renderGroupTitle(st, i)
	}
	line := st.FileName.Render(it.entry.Name) + "  " + target
	if it.parent != "" {
		return m.renderGroupTitle(st, m.indexOf(it.parent)) + "\n" + st.Muted.Render("└─ ") + line
	}
	return line
}

// renderGroupTitle 은 메뉴 제목에 쓰는 그룹 한 줄이다. 폴더 그룹은 경로가 없어서 이름만 쓴다.
func (m List) renderGroupTitle(st ui.Styles, i int) string {
	if m.isFolder(i) {
		return st.GroupName.Render(m.items[i].entry.Name)
	}
	return st.GroupName.Render(m.items[i].entry.Name) + "  " + ui.Tildify(m.items[i].entry.Target, m.home)
}

// statusMarker 는 상태 줄 앞에 붙일 표시와 그 색을 고를 상태다. 목록의 체크박스와 같은 표시를 쓴다.
func (m List) statusMarker() (marker string, tone ui.ItemState) {
	return ui.ToneMarker(m.StatusTone, m.renderMarker(ui.Syncing))
}

// renderSummary 는 연결된 항목 수와 전체 수를 ratio만큼 채운 게이지와 함께 그린다. 목록을 불러오기 전이면 빈 문자열이다.
func (m List) renderSummary(st ui.Styles, ratio float64) string {
	if !m.ready || m.loadErr != nil || len(m.items) == 0 {
		return ""
	}
	return st.Muted.Render(fmt.Sprintf("연결됨 %d/%d ", m.syncedCount(), m.fileCount())) + st.RenderGauge(ratio)
}

// Summary 는 진입 메뉴 카드에 붙이는 연결 비율이다. 화면 머리와 달리 움직이는 게이지 대신 지금 비율을 그대로 그린다.
func (m List) Summary(st ui.Styles) string {
	return m.renderSummary(st, m.syncedRatio())
}
