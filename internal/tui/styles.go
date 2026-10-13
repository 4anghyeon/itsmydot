package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// palette 는 화면에 쓰는 색 묶음이다.
// 터미널 테마의 기본 16색은 테마에 따라 초록이 올리브색으로 보이는 등 의미가 흐려져서,
// 색을 직접 지정하고 배경 밝기에 따라 두 벌 중 하나를 고른다.
type palette struct {
	synced, conflict, errored, syncing color.Color
	muted, accent                      color.Color
}

var (
	darkPalette = palette{
		synced:   lipgloss.Color("#4ADE80"),
		conflict: lipgloss.Color("#FBBF24"),
		errored:  lipgloss.Color("#F87171"),
		syncing:  lipgloss.Color("#22D3EE"),
		muted:    lipgloss.Color("#8A8A8A"),
		accent:   lipgloss.Color("#C084FC"),
	}
	lightPalette = palette{
		synced:   lipgloss.Color("#15803D"),
		conflict: lipgloss.Color("#B45309"),
		errored:  lipgloss.Color("#DC2626"),
		syncing:  lipgloss.Color("#0E7490"),
		muted:    lipgloss.Color("#6B7280"),
		accent:   lipgloss.Color("#9333EA"),
	}
)

// styles 는 palette로 만든 스타일 묶음이다.
type styles struct {
	palette
	frame, title, muted, cursor, selected, question, key lipgloss.Style
}

// newStyles 는 배경 밝기에 맞는 스타일 묶음을 만든다.
func newStyles(dark bool) styles {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	return styles{
		palette:  p,
		frame:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.muted).Padding(0, 1),
		title:    lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		muted:    lipgloss.NewStyle().Foreground(p.muted),
		cursor:   lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		selected: lipgloss.NewStyle().Bold(true),
		question: lipgloss.NewStyle().Bold(true).Foreground(p.conflict),
		key:      lipgloss.NewStyle().Bold(true),
	}
}

// frameExtra 는 frame의 테두리(좌우 1칸씩)와 안쪽 여백(좌우 1칸씩)을 합친 폭이다.
const frameExtra = 4

// gaugeWidth 는 진행 게이지의 칸 수다.
const gaugeWidth = 16

// state 는 상태별 체크박스 색이다.
func (st styles) state(s itemState) lipgloss.Style {
	switch s {
	case synced:
		return lipgloss.NewStyle().Foreground(st.palette.synced)
	case syncing:
		return lipgloss.NewStyle().Foreground(st.palette.syncing)
	case conflict:
		return lipgloss.NewStyle().Foreground(st.palette.conflict)
	case errored:
		return lipgloss.NewStyle().Foreground(st.palette.errored)
	default:
		return st.muted
	}
}

// gauge 는 total 중 done만큼 채운 진행 막대를 그린다.
// 채워진 칸은 목록의 [+]와 같은 초록이라, 색만 봐도 연결된 비율임을 알 수 있다.
func (st styles) gauge(done, total int) string {
	filled := 0
	if total > 0 {
		filled = done * gaugeWidth / total
	}
	if done > 0 && filled == 0 {
		filled = 1 // 하나라도 연결돼 있으면 빈 막대로 보이지 않게 한다
	}

	return st.state(synced).Render(strings.Repeat("█", filled)) +
		st.muted.Render(strings.Repeat("░", gaugeWidth-filled))
}

// help 는 "키 설명" 쌍을 이어 붙인 도움말 한 줄을 만든다. help("space", "토글", "esc", "종료")
func (st styles) help(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, st.key.Render(pairs[i])+" "+st.muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}
