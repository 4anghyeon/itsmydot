package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// palette 는 화면에 쓰는 색 묶음이다.
// 터미널의 기본 16색은 테마에 따라 초록이 올리브색으로 보이기도 해서 색을 직접 지정한다.
// 배경 밝기에 따라 두 벌 중 하나를 고른다.
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
	update                                               lipgloss.Style
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
		update:   lipgloss.NewStyle().Bold(true).Foreground(p.syncing),
	}
}

// syncSpinner 는 받는 중인 항목의 체크박스 안에서 . o O o 순서로 돈다.
// 기본 스피너의 점자 문자는 폰트에 따라 폭이 깨지고 막대가 도는 모양은 실패 표시 [-]와 헷갈려서 직접 정의한다.
var syncSpinner = spinner.Spinner{
	Frames: []string{".", "o", "O", "o"},
	FPS:    time.Second / 8,
}

// frameExtra 는 frame의 좌우 테두리와 안쪽 여백을 합친 폭이다. 네 가지 모두 한 칸씩이다.
const frameExtra = 4

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

// help 는 키와 설명을 번갈아 받아 도움말 한 줄로 이어 붙인다.
func (st styles) help(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, st.key.Render(pairs[i])+" "+st.muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}
