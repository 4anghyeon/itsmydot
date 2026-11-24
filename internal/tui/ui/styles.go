package ui

import (
	"image/color"
	"math"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// Palette 는 화면에 쓰는 색 묶음이다.
// 터미널의 기본 16색은 테마에 따라 초록이 올리브색으로 보이기도 해서 색을 직접 지정한다.
// 배경 밝기에 따라 두 벌 중 하나를 고른다.
type Palette struct {
	synced, conflict, errored, syncing color.Color
	muted, Accent                      color.Color
	update                             color.Color // 업데이트 있음 표시. 충돌의 노란색과 헷갈리지 않게 주황 쪽으로 둔다
	group, file                        color.Color // 메뉴 제목의 그룹 이름과 파일 이름
	count                              color.Color // 그룹 줄 끝의 하위 항목 수. 회색 경로와 구분되게 채도 낮은 파랑을 쓴다
	Selection, statusBar               color.Color // 선택한 줄과 맨 아래 상태 줄의 배경. 256색표에 있는 회색만 쓴다

	DiffRemoved, DiffAdded diffTones   // 비교 보기에서 지운 줄과 추가한 줄의 배경
	DiffHunk               color.Color // 비교 보기에서 @@ 줄의 배경
}

// diffTones 는 비교 보기에서 지운 줄이나 추가한 줄 한쪽을 칠하는 배경색이다. 줄, 줄 번호 칸, 바뀐 단어 순으로 진하다.
// 256색 터미널은 색을 가장 가까운 칸으로 바꾸는데 옅은 분홍은 노랑으로, 옅은 초록은 하늘색으로 바뀐다.
// 그래서 256색표에 있는 색만 골라 어느 터미널에서나 빨강과 초록 계열로 보이게 한다.
type diffTones struct {
	Line, Number, Word color.Color
}

var (
	DarkPalette = Palette{
		synced:   lipgloss.Color("#4ADE80"),
		conflict: lipgloss.Color("#FBBF24"),
		errored:  lipgloss.Color("#F87171"),
		syncing:  lipgloss.Color("#22D3EE"),
		muted:    lipgloss.Color("#8A8A8A"),
		Accent:   lipgloss.Color("#C084FC"),
		update:   lipgloss.Color("#FB923C"),
		group:    lipgloss.Color("#60A5FA"),
		file:     lipgloss.Color("#F472B6"),
		count:    lipgloss.Color("#5F87AF"),

		Selection: lipgloss.Color("#3A3A3A"),
		statusBar: lipgloss.Color("#303030"),

		DiffRemoved: diffTones{lipgloss.Color("#5F0000"), lipgloss.Color("#870000"), lipgloss.Color("#AF0000")},
		DiffAdded:   diffTones{lipgloss.Color("#005F00"), lipgloss.Color("#008700"), lipgloss.Color("#00AF00")},
		DiffHunk:    lipgloss.Color("#303030"),
	}
	LightPalette = Palette{
		synced:   lipgloss.Color("#15803D"),
		conflict: lipgloss.Color("#B45309"),
		errored:  lipgloss.Color("#DC2626"),
		syncing:  lipgloss.Color("#0E7490"),
		muted:    lipgloss.Color("#6B7280"),
		Accent:   lipgloss.Color("#9333EA"),
		update:   lipgloss.Color("#C2410C"),
		group:    lipgloss.Color("#2563EB"),
		file:     lipgloss.Color("#DB2777"),
		count:    lipgloss.Color("#5F87AF"),

		Selection: lipgloss.Color("#E4E4E4"),
		statusBar: lipgloss.Color("#EEEEEE"),

		DiffRemoved: diffTones{lipgloss.Color("#FFD7D7"), lipgloss.Color("#FFAFAF"), lipgloss.Color("#FF8787")},
		DiffAdded:   diffTones{lipgloss.Color("#D7FFD7"), lipgloss.Color("#AFFFAF"), lipgloss.Color("#87FF87")},
		DiffHunk:    lipgloss.Color("#EEEEEE"),
	}
)

// Styles 는 [Palette] 로 만든 스타일 묶음이다.
type Styles struct {
	Palette
	Title, Muted, Cursor, selected, Question, Key lipgloss.Style
	Update, GroupName, FileName, Count            lipgloss.Style
	description, descriptionBar                   lipgloss.Style
	Heading                                       lipgloss.Style // 도움말 화면의 소제목
	mascotFace, mascotMouth                       lipgloss.Style // 맨 위 캐릭터의 눈과 코, 입
	DiffRemoved, DiffAdded                        DiffStyles
	DiffHunk                                      lipgloss.Style
}

// NewStyles 는 배경 밝기에 맞는 스타일 묶음을 만든다.
func NewStyles(dark bool) Styles {
	p := LightPalette
	if dark {
		p = DarkPalette
	}
	return Styles{
		Palette:        p,
		Title:          lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		Muted:          lipgloss.NewStyle().Foreground(p.muted),
		Cursor:         lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		selected:       lipgloss.NewStyle().Bold(true),
		Question:       lipgloss.NewStyle().Bold(true).Foreground(p.conflict),
		Key:            lipgloss.NewStyle().Bold(true),
		Update:         lipgloss.NewStyle().Bold(true).Foreground(p.update),
		GroupName:      lipgloss.NewStyle().Bold(true).Foreground(p.group),
		Count:          lipgloss.NewStyle().Foreground(p.count),
		FileName:       lipgloss.NewStyle().Bold(true).Foreground(p.file),
		description:    lipgloss.NewStyle().Italic(true).Foreground(p.muted),
		descriptionBar: lipgloss.NewStyle().Foreground(p.muted),
		Heading:        lipgloss.NewStyle().Bold(true),
		mascotFace:     lipgloss.NewStyle().Bold(true),
		mascotMouth:    lipgloss.NewStyle().Bold(true).Foreground(p.group),
		DiffRemoved:    newDiffStyles(p.DiffRemoved, p.errored, p.muted),
		DiffAdded:      newDiffStyles(p.DiffAdded, p.synced, p.muted),
		DiffHunk:       lipgloss.NewStyle().Background(p.DiffHunk).Foreground(p.muted),
	}
}

// SyncSpinner 는 받는 중인 항목의 체크박스 안에서 . o O o 순서로 돈다.
// 기본 스피너의 점자 문자는 폰트에 따라 폭이 깨지고 막대가 도는 모양은 실패 표시 [-]와 헷갈려서 직접 정의한다.
var SyncSpinner = spinner.Spinner{
	Frames: []string{".", "o", "O", "o"},
	FPS:    time.Second / 8,
}

// StateStyle 는 상태별 체크박스 색이다.
func (st Styles) StateStyle(s ItemState) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(st.StateColor(s))
}

// StateColor 는 상태별 색이다.
func (st Styles) StateColor(s ItemState) color.Color {
	switch s {
	case Synced:
		return st.Palette.synced
	case Syncing:
		return st.Palette.syncing
	case Conflict:
		return st.Palette.conflict
	case Errored:
		return st.Palette.errored
	default:
		return st.Palette.muted
	}
}

// FlashStyle 는 상태 색에 강조색을 flash만큼 섞은 스타일이다. flash가 1이면 강조색이고 0이면 상태 색 그대로다.
func (st Styles) FlashStyle(s ItemState, flash float64) lipgloss.Style {
	if flash <= 0 {
		return st.StateStyle(s)
	}
	const steps = 101
	blend := lipgloss.Blend1D(steps, st.StateColor(s), st.Palette.Accent)
	i := int(math.Round(min(flash, 1) * (steps - 1)))
	return lipgloss.NewStyle().Foreground(blend[i])
}

// DiffStyles 는 비교 보기에서 지운 줄이나 추가한 줄 한쪽을 그리는 스타일이다.
// 글자 색은 터미널 기본색을 두고 배경만 칠한다. 빨간 배경에 빨간 글자는 읽기 어렵다.
type DiffStyles struct {
	Line, Number, Word, Marker lipgloss.Style
}

// newDiffStyles 는 tones로 칠하는 [DiffStyles] 를 만든다. marker는 +나 - 표시라서 줄 배경 위에 mark 색 글자로 그린다.
func newDiffStyles(tones diffTones, mark, muted color.Color) DiffStyles {
	return DiffStyles{
		Line:   lipgloss.NewStyle().Background(tones.Line),
		Number: lipgloss.NewStyle().Background(tones.Number).Foreground(muted),
		Word:   lipgloss.NewStyle().Background(tones.Word),
		Marker: lipgloss.NewStyle().Background(tones.Line).Foreground(mark).Bold(true),
	}
}
