package ui

import "charm.land/lipgloss/v2"

// horizontalScrollStep 은 좌우 방향키 한 번에 옆으로 미는 칸 수다.
const horizontalScrollStep = 8

// ScrollBy 는 스크롤 키에 맞춰 세로 위치 y와 가로 위치 x를 옮긴다. 비교 보기와 미리보기가 같은 키를 쓴다.
// page는 한 쪽의 줄 수이고 total은 전체 줄 수다. 내용 밖으로 나간 위치는 부르는 쪽에서 자른다.
func ScrollBy(key string, y, x, page, total int) (int, int) {
	switch key {
	case "up", "k":
		y--
	case "down", "j":
		y++
	case "pgup":
		y -= page
	case "pgdown", "space":
		y += page
	case "home":
		y = 0
	case "end":
		y = total
	case "left", "h":
		x -= horizontalScrollStep
	case "right", "l":
		x += horizontalScrollStep
	}
	return y, x
}

// RenderScrollbarCell 은 본문 오른쪽 스크롤 막대의 row번째 칸이다. 지금 보이는 범위는 █, 나머지는 ░로 그린다.
// 터미널 창의 회색 스크롤 막대와 헷갈리지 않게 █는 강조색으로 칠한다.
func (st Styles) RenderScrollbarCell(row, height, offset, total int) string {
	thumb := max(height*height/total, 1)
	start := 0
	if total > height {
		start = (offset*(height-thumb) + (total-height)/2) / (total - height)
	}
	if row >= start && row < start+thumb {
		return lipgloss.NewStyle().Foreground(st.Palette.Accent).Render("█")
	}
	return st.Muted.Render("░")
}

// ClampListOffset 은 창의 맨 위 위치 offset을 selected 줄이 보이고 total줄 안에서 창이 꽉 차는 범위로 옮긴다.
func ClampListOffset(offset, selected, height, total int) int {
	if selected >= 0 {
		offset = min(offset, selected)
		offset = max(offset, selected-height+1)
	}
	return min(max(offset, 0), max(total-height, 0))
}
