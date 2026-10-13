package tui

import (
	"math"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"
)

// 토글 애니메이션.
//   - 진행 게이지는 새 비율까지 스프링처럼 늘어나거나 줄어든다.
//   - 상태가 바뀐 항목의 체크박스는 잠깐 강조색으로 빛났다가 원래 상태 색으로 돌아온다.
//
// 글자 모양은 그대로 두고 색과 막대 길이만 움직인다. 움직이는 동안에만 animationTick을 예약한다.

const animationFPS = 60

var (
	// gaugeSpring 은 게이지 길이를 움직인다. 감쇠 1(임계 감쇠)이라 목표를 넘치지 않고 빠르게 멈춘다.
	gaugeSpring = harmonica.NewSpring(harmonica.FPS(animationFPS), 10, 1)
	// flashSpring 은 체크박스의 강조색을 원래 색으로 되돌린다. 게이지보다 조금 천천히 사라진다.
	flashSpring = harmonica.NewSpring(harmonica.FPS(animationFPS), 6, 1)
)

// animationSettle 은 이보다 가까워지면 목표에 도달한 것으로 보고 멈추는 기준이다.
const animationSettle = 0.005

// animationTickMsg 는 애니메이션 한 프레임을 진행하라는 메시지다.
type animationTickMsg struct{}

func animationTick() tea.Cmd {
	return tea.Tick(time.Second/animationFPS, func(time.Time) tea.Msg { return animationTickMsg{} })
}

// syncedRatio 는 전체 항목 중 연결된 항목의 비율(0~1)이다. 게이지가 향하는 목표다.
func (m Model) syncedRatio() float64 {
	if len(m.items) == 0 {
		return 0
	}
	done := 0
	for _, it := range m.items {
		if it.state == synced {
			done++
		}
	}
	return float64(done) / float64(len(m.items))
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

func settled(pos, vel, target float64) bool {
	return math.Abs(pos-target) < animationSettle && math.Abs(vel) < animationSettle
}
