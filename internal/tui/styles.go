package tui

import (
	"image/color"
	"math"
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
	update                             color.Color // 업데이트 있음 표시. 충돌의 노란색과 헷갈리지 않게 주황 쪽으로 둔다
	group, file                        color.Color // 메뉴 제목의 그룹 이름과 파일 이름
	count                              color.Color // 그룹 줄 끝의 하위 항목 수. 회색 경로와 구분되게 채도 낮은 파랑을 쓴다
	selection, statusBar               color.Color // 선택한 줄과 맨 아래 상태 줄의 배경. 256색표에 있는 회색만 쓴다

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
		count:    lipgloss.Color("#5F87AF"),

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
		count:    lipgloss.Color("#5F87AF"),

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
	update, groupName, fileName, count            lipgloss.Style
	description, descriptionBar                   lipgloss.Style
	heading                                       lipgloss.Style // 도움말 화면의 소제목
	mascotFace, mascotMouth                       lipgloss.Style // 맨 위 캐릭터의 눈과 코, 입
	diffRemoved, diffAdded                        diffStyles
	diffHunk                                      lipgloss.Style
}

// newStyles 는 배경 밝기에 맞는 스타일 묶음을 만든다.
func newStyles(dark bool) styles {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	return styles{
		palette:        p,
		title:          lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		muted:          lipgloss.NewStyle().Foreground(p.muted),
		cursor:         lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		selected:       lipgloss.NewStyle().Bold(true),
		question:       lipgloss.NewStyle().Bold(true).Foreground(p.conflict),
		key:            lipgloss.NewStyle().Bold(true),
		update:         lipgloss.NewStyle().Bold(true).Foreground(p.update),
		groupName:      lipgloss.NewStyle().Bold(true).Foreground(p.group),
		count:          lipgloss.NewStyle().Foreground(p.count),
		fileName:       lipgloss.NewStyle().Bold(true).Foreground(p.file),
		description:    lipgloss.NewStyle().Italic(true).Foreground(p.muted),
		descriptionBar: lipgloss.NewStyle().Foreground(p.muted),
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

// stateStyle 는 상태별 체크박스 색이다.
func (st styles) stateStyle(s itemState) lipgloss.Style {
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

// flashStyle 는 상태 색에 강조색을 flash만큼 섞은 스타일이다. flash가 1이면 강조색이고 0이면 상태 색 그대로다.
func (st styles) flashStyle(s itemState, flash float64) lipgloss.Style {
	if flash <= 0 {
		return st.stateStyle(s)
	}
	const steps = 101
	blend := lipgloss.Blend1D(steps, st.stateColor(s), st.palette.accent)
	i := int(math.Round(min(flash, 1) * (steps - 1)))
	return lipgloss.NewStyle().Foreground(blend[i])
}
