// Package tui 는 itsmydot의 터미널 화면이다. 다른 패키지(manifest, github, localcopy, link)를 조립해서 쓴다.
package tui

import (
	"context"
	"os"
	"slices"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// itemState 는 화면에 표시하는 항목 하나의 상태다.
// link.State(디스크 상태)에 화면에서만 쓰는 상태(syncing, errored)를 더한 것이다.
type itemState int

const (
	notSynced itemState = iota // [ ] symlink 없음
	synced                     // [+] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [~] GitHub에서 받는 중 (화면에서는 ~ 자리에 스피너가 돈다)
	conflict                   // [!] target 자리에 다른 파일이 있음
	errored                    // [-] 상태 확인이나 동기화 실패
)

// marker 는 상태를 체크박스 모양으로 바꾼다.
// ASCII만 쓴다. ✓ 같은 기호는 폰트에 없으면 다른 폰트에서 빌려 그리면서 두 칸으로 그려져 정렬이 깨진다.
func (s itemState) marker() string {
	switch s {
	case synced:
		return "[+]"
	case syncing:
		return "[~]"
	case conflict:
		return "[!]"
	case errored:
		return "[-]"
	default:
		return "[ ]"
	}
}

// item 은 목록의 한 줄이다.
type item struct {
	entry    manifest.Entry
	copyPath string // 이 항목의 로컬 사본 경로. symlink가 가리켜야 할 곳
	state    itemState
	err      error // state가 errored일 때의 원인

	flash, flashVelocity float64 // 상태가 바뀐 직후의 강조 정도(1 → 0)와 그 속도. animation.go 참고
}

// Model 은 화면의 상태 전부를 담는다.
type Model struct {
	client *github.Client
	copies *localcopy.Dir
	home   string // 화면에서 경로를 ~로 줄여 보여 줄 때 쓴다
	width  int    // 터미널 폭. 모르면 0이고, 이때는 상자 폭을 제한하지 않는다
	light  bool   // 터미널 배경이 밝으면 true. 감지 전에는 어두운 배경으로 본다

	items   []item
	cursor  int    // 선택된 줄 번호 (0부터)
	ready   bool   // manifest를 다 불러왔으면 true
	loadErr error  // manifest 자체를 못 불러왔을 때의 에러
	status  string // 마지막 토글 결과. 목록 아래에 한 줄로 보여 준다

	// confirming 은 확인 질문의 답을 기다리는 항목의 이름이다. 없으면 "".
	// items는 Update마다 복사되므로 포인터 대신 이름으로 가리킨다.
	confirming  string
	confirmKind confirmKind
	choice      int // 확인 질문에서 커서가 있는 선택지 (0부터)

	spinner spinner.Model // 받는 중인 항목이 있을 때만 돈다

	gauge, gaugeVelocity float64 // 게이지에 그려지는 연결 비율(0~1)과 그 속도. syncedRatio를 향해 움직인다
	animating            bool    // animationTick이 예약돼 있으면 true
}

// confirmKind 는 어떤 확인 질문을 띄웠는지 나타낸다.
type confirmKind int

const (
	confirmLink   confirmKind = iota // 다른 파일이 있어서, 백업한 뒤 연결할지 묻는 중
	confirmUnlink                    // 되돌릴 백업이 없어서, 연결을 어떻게 끊을지 묻는 중
)

// New 는 처음 화면 상태를 만든다. 실제 불러오기는 Init에서 시작한다.
func New(client *github.Client, copies *localcopy.Dir) Model {
	home, _ := os.UserHomeDir() // 실패해도 경로를 줄이지 않고 그대로 보여 줄 뿐이라 무시한다
	return Model{client: client, copies: copies, home: home, spinner: spinner.New(spinner.WithSpinner(syncSpinner))}
}

// Init 은 manifest 불러오기를 시작하고, 색을 고르기 위해 터미널 배경색을 물어본다.
func (m Model) Init() tea.Cmd {
	load := func() tea.Msg {
		return loadEntries(context.Background(), m.client, m.copies)
	}
	return tea.Batch(load, tea.RequestBackgroundColor)
}

// Update 는 불러오기/동기화 결과와 키 입력(방향키 이동, space 토글, esc/q 종료, 확인 질문의 1/2)을 반영한다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.ready = true
		m.items = msg.items
		m.loadErr = msg.err
		m.gauge = m.syncedRatio() // 처음 불러올 때는 애니메이션 없이 바로 맞춘다
		return m, nil

	case entrySyncedMsg:
		return m.applySynced(msg).startAnimation()

	case animationTickMsg:
		return m.stepAnimation()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
		return m, nil

	case spinner.TickMsg:
		// 받는 중인 항목이 없으면 다음 tick을 예약하지 않아 스피너가 멈춘다. startSync가 다시 돌린다.
		if !slices.ContainsFunc(m.items, func(it item) bool { return it.state == syncing }) {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		key := normalizeKey(msg)
		if m.confirming != "" {
			return m.answerConfirm(key)
		}
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "space":
			return m.toggle()
		}
	}
	return m, nil
}
