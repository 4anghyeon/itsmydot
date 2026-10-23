package tui

import (
	"math"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"
)

// 토글하면 두 가지가 움직인다. 진행 게이지는 새 비율까지 스프링처럼 늘거나 줄고
// 상태가 바뀐 항목의 체크박스는 잠깐 강조색으로 빛났다가 원래 색으로 돌아온다.
// 글자 모양은 그대로 두고 색과 막대 길이만 바꾼다. 움직이는 동안에만 animationTick을 예약한다.

const animationFPS = 60

var (
	// gaugeSpring 은 게이지 길이를 움직인다. 감쇠비가 1이라 목표를 지나치지 않고 빠르게 멈춘다.
	gaugeSpring = harmonica.NewSpring(harmonica.FPS(animationFPS), 10, 1)
	// flashSpring 은 체크박스의 강조색을 원래 색으로 되돌린다. 게이지보다 조금 천천히 사라진다.
	flashSpring = harmonica.NewSpring(harmonica.FPS(animationFPS), 6, 1)
)

// animationSettleThreshold 은 이보다 가까워지면 목표에 도달한 것으로 보고 멈추는 기준이다.
const animationSettleThreshold = 0.005

// animationTickMsg 는 애니메이션 한 프레임을 진행하라는 메시지다.
type animationTickMsg struct{}

func animationTick() tea.Cmd {
	return tea.Tick(time.Second/animationFPS, func(time.Time) tea.Msg { return animationTickMsg{} })
}

// syncedCount 는 연결된 항목 수다.
func (m Model) syncedCount() int {
	count := 0
	for _, it := range m.items {
		if it.state == synced {
			count++
		}
	}
	return count
}

// syncedRatio 는 전체 항목 중 연결된 항목의 비율이다. 게이지가 향하는 목표다.
func (m Model) syncedRatio() float64 {
	if len(m.items) == 0 {
		return 0
	}
	return float64(m.syncedCount()) / float64(len(m.items))
}

// startAnimation 은 애니메이션이 멈춰 있으면 다시 시작한다. 이미 돌고 있으면 tick을 겹쳐 예약하지 않는다.
func (m Model) startAnimation() (Model, tea.Cmd) {
	if m.animating {
		return m, nil
	}
	m.animating = true
	return m, animationTick()
}

// stepAnimation 은 한 프레임만큼 게이지와 강조색을 움직인다. 모두 멈추면 다음 tick을 예약하지 않는다.
func (m Model) stepAnimation() (Model, tea.Cmd) {
	moving := false

	target := m.syncedRatio()
	m.gauge, m.gaugeVelocity = gaugeSpring.Update(m.gauge, m.gaugeVelocity, target)
	if settled(m.gauge, m.gaugeVelocity, target) {
		m.gauge, m.gaugeVelocity = target, 0
	} else {
		moving = true
	}

	m.items = slices.Clone(m.items)
	for i := range m.items {
		it := &m.items[i]
		if it.flash == 0 && it.flashVelocity == 0 {
			continue
		}
		it.flash, it.flashVelocity = flashSpring.Update(it.flash, it.flashVelocity, 0)
		if settled(it.flash, it.flashVelocity, 0) {
			it.flash, it.flashVelocity = 0, 0
		} else {
			moving = true
		}
	}

	if !moving {
		m.animating = false
		return m, nil
	}
	return m, animationTick()
}

func settled(position, velocity, target float64) bool {
	return math.Abs(position-target) < animationSettleThreshold && math.Abs(velocity) < animationSettleThreshold
}
