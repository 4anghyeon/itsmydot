package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4anghyeon/itsmydot/internal/homebrew"
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
func (st styles) brewMarkStyle(b brewMark) lipgloss.Style {
	switch b {
	case brewInstalled:
		return st.stateStyle(synced)
	case brewPicked:
		return lipgloss.NewStyle().Bold(true).Foreground(st.palette.accent)
	default:
		return st.stateStyle(notSynced)
	}
}

// mark 는 i번째 줄의 체크박스 상태다.
// 프로필 줄은 고른 하위 패키지가 있으면 [*], 하위 패키지가 모두 설치돼 있으면 [+]다.
func (m brewfileList) mark(i int) brewMark {
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
	children := treeChildren(m.items, i)
	if slices.ContainsFunc(children, func(j int) bool { return m.mark(j) == brewPicked }) {
		return brewPicked
	}
	if len(children) > 0 && !slices.ContainsFunc(children, func(j int) bool { return m.mark(j) != brewInstalled }) {
		return brewInstalled
	}
	return brewNotInstalled
}

// View 는 프로필별 패키지 목록과 그 아래에 커서 줄의 설명이나 설치 확인 질문, 상태 줄을 그린다.
// Homebrew가 없으면 목록 대신 설치 방법을 안내한다.
func (m brewfileList) View() tea.View {
	st := newStyles(!m.light)

	right := ""
	if m.ready && !m.missing && m.loadErr == nil && len(m.items) > 0 {
		installed, total := m.installedCount()
		right = st.muted.Render(fmt.Sprintf("설치됨 %d/%d ", installed, total)) + st.renderGauge(ratio(installed, total))
	}

	var list, lower []string
	selected := -1
	switch {
	case !m.ready:
		list = append(list, " "+st.muted.Render("Brewfile 불러오는 중이에요..."))
	case m.missing:
		list = m.renderMissingLines(st)
	case m.loadErr != nil:
		list = append(list, " "+st.stateStyle(errored).Render("Brewfile을 불러오지 못했어요. "+errorReason(m.loadErr, "")))
		for line := range strings.SplitSeq(m.loadErr.Error(), "\n") {
			list = append(list, " "+st.stateStyle(errored).Render(line))
		}
	case len(m.items) == 0:
		list = append(list, " "+st.muted.Render("등록된 Brewfile이 없어요"))
	default:
		list, selected = m.renderBrewLines(st)
		// 설명이 없어도 줄은 남겨서 커서를 옮길 때 화면 높이가 출렁이지 않게 한다.
		lower = append(lower, " "+st.renderDescription(m.items[m.cursor].description(), m.width))
	}

	footer := st.renderHelp(m.width, "방향키", "이동", "space", "고르기", "enter", "설치", "esc", "진입 메뉴")
	if !m.ready || m.missing || m.loadErr != nil || len(m.items) == 0 {
		footer = st.renderHelp(m.width, "esc", "진입 메뉴")
	}
	status := m.status
	if m.confirming {
		var numbers []string
		lower, numbers = m.renderInstallConfirmLines(st)
		footer = st.renderHelp(m.width, "방향키", "이동", "enter", "선택", strings.Join(numbers, "/"), "바로 선택", "esc", "취소")
		status = ""
	}

	marker, tone := "", notSynced
	if status != "" {
		marker, tone = toneMarker(m.statusTone, syncing.marker())
	}

	width := st.pageWidth(m.width, right, append(slices.Concat(list, lower), " "+marker+" "+status)...)
	if selected >= 0 {
		// 선택한 줄은 배경을 목록 끝까지 칠한다.
		list[selected] += lipgloss.NewStyle().Background(st.selection).Render(strings.Repeat(" ", max(width-lipgloss.Width(list[selected]), 0)))
	}
	lines := append(append(list, "", st.renderRule(width)), lower...)
	lines = append(lines, "", st.renderStatusLine(width, marker, st.stateColor(tone), status))
	return tea.NewView(st.renderPage(width, m.version, right, lines, footer))
}

// renderBrewLines 는 프로필과 패키지를 한 줄씩 만들고 커서가 있는 줄의 위치를 함께 돌려준다.
//
//	▌ v [ ] base       3/18
//	  ├── [+] zsh      brew
//	  ├── [*] raycast  cask
//	  └── [ ] switch   cask
func (m brewfileList) renderBrewLines(st styles) (lines []string, selected int) {
	rows := make([]listRow, 0, len(m.items))
	for i, it := range m.items {
		if !treeVisible(m.items, m.collapsed, i) {
			continue
		}
		mark := m.mark(i)
		row := listRow{
			fold:        treeFoldMarker(m.items, m.collapsed, i),
			branch:      treeBranch(m.items, i),
			marker:      mark.marker(),
			markerStyle: st.brewMarkStyle(mark),
			name:        it.nodeName(),
			current:     i == m.cursor,
		}
		if it.isProfile() {
			children := treeChildren(m.items, i)
			installed := 0
			for _, j := range children {
				if m.installed.Has(m.items[j].pkg) {
					installed++
				}
			}
			row.tail = append(row.tail, listSegment{"", fmt.Sprintf("%d/%d", installed, len(children)), st.count})
		} else {
			row.detail = string(it.pkg.Kind)
		}
		rows = append(rows, row)
	}
	return st.renderRows(rows)
}

// renderInstallConfirmLines 는 설치 확인 질문과 고른 패키지 이름, 번호 선택지를 한 줄씩 만든다.
func (m brewfileList) renderInstallConfirmLines(st styles) (lines, numbers []string) {
	packages := m.pickedPackages()
	names := make([]string, len(packages))
	for i, p := range packages {
		names[i] = p.Name
	}
	lines = []string{
		" " + st.question.Render(fmt.Sprintf("%d개 패키지를 설치할까요?", len(packages))),
		" " + st.muted.Render(strings.Join(names, ", ")),
		"",
	}
	options, numbers := st.renderOptions(brewConfirmOptions, m.choice)
	return append(lines, options...), numbers
}

// renderMissingLines 는 Homebrew가 없을 때 목록 대신 보여 줄 안내다. itsmydot은 Homebrew를 직접 설치하지 않는다.
//
// 설치 스크립트는 셸 설정을 고치지 않고 끝에 Next steps 명령만 보여 준다.
// Apple Silicon Mac에서는 그 명령을 실행해야 새 셸에서 brew가 PATH에 잡히므로 함께 안내한다.
// PATH 설정은 Homebrew 안내대로 각 Mac의 ~/.zprofile에 두고 itsmydot은 관리하지 않는다.
func (m brewfileList) renderMissingLines(st styles) []string {
	return []string{
		" " + st.question.Render("Homebrew가 필요해요"),
		"",
		" " + "터미널에서 아래 명령으로 Homebrew를 설치해 주세요.",
		"",
		"   " + st.key.Render(homebrew.InstallGuide),
		"",
		" " + "설치가 끝나면 화면 끝에 나오는 Next steps 명령도 실행해 주세요.",
		" " + "새 셸에서 brew와 brew로 설치한 도구를 찾으려면 이 단계가 필요해요.",
		" " + "그다음 itsmydot을 다시 실행해 주세요.",
		"",
		" " + st.muted.Render("자세한 내용은 https://brew.sh 를 참고해 주세요"),
	}
}

// ratio 는 part가 total에서 차지하는 비율이다. total이 0이면 0이다.
func ratio(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total)
}
