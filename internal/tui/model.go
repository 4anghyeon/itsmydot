// Package tui 는 itsmydot의 터미널 화면이다. manifest, github, localcopy, link 패키지를 엮어서 항목을 켜고 끈다.
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

// itemState 는 화면에 보여 주는 항목 하나의 상태다.
// 디스크 상태인 [link.State] 에 화면에서만 쓰는 syncing과 errored를 더했다.
type itemState int

const (
	notSynced itemState = iota // [ ] symlink 없음
	synced                     // [+] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [~] GitHub에서 받는 중. 화면에서는 ~ 대신 스피너가 돈다
	conflict                   // [!] target 자리에 다른 파일이 있음
	errored                    // [-] 상태 확인이나 동기화 실패
)

// marker 는 상태를 체크박스 모양으로 바꾼다.
// 체크 표시 같은 기호는 폰트에 없으면 다른 폰트로 대신 그리는데 이때 두 칸을 차지해 정렬이 깨진다. 그래서 ASCII만 쓴다.
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
	copyPath string // 로컬 사본 경로. symlink가 가리켜야 하는 곳이다
	state    itemState
	err      error  // state가 errored일 때의 원인
	parent   string // 하위 항목이면 그룹 이름. 하위 항목은 items에서 그룹 바로 뒤에 이어 붙는다
	outdated bool   // 연결된 항목의 로컬 사본이 시작할 때 본 리포 파일과 다르면 true

	flash, flashVelocity float64 // 상태가 바뀐 직후 남아 있는 강조색의 정도와 그 속도. 1에서 0으로 줄어든다
}

// Model 은 화면의 상태 전부를 담는다.
type Model struct {
	client *github.Client
	copies *localcopy.Dir
	home   string // 화면에서 경로를 ~로 줄여 보여 줄 때 쓴다
	width  int    // 터미널 폭. 모르면 0이고 이때는 상자 폭을 제한하지 않는다
	height int    // 터미널 높이. 모르면 0이고 이때는 비교 보기 화면 높이를 내용에 맞춘다
	light  bool   // 터미널 배경이 밝으면 true. 감지 전에는 어두운 배경으로 본다

	items   []item
	cursor  int    // 선택된 줄. 0부터 센다
	ready   bool   // manifest를 다 불러왔으면 true
	loadErr error  // manifest 자체를 못 불러왔을 때의 에러
	status  string // 마지막 토글 결과. 목록 아래 한 줄로 보여 준다

	// collapsed 는 접힌 그룹의 이름이다. Model이 복사돼도 map은 공유되므로 바꿀 때는 새 map을 만든다.
	collapsed map[string]bool

	// confirming 은 확인 질문의 답을 기다리는 항목의 이름이다. 없으면 빈 문자열이다.
	// items는 Update마다 복사되므로 포인터 대신 이름으로 가리킨다.
	confirming  string
	confirmKind confirmKind
	choice      int      // 확인 질문에서 커서가 있는 선택지. 0부터 센다
	pending     []string // confirmLink에서 백업 후 연결을 고르면 받을 항목들. 그룹 파일이 함께 들어 있을 수 있다
	confirmNote string   // 확인 질문 아래 한 줄. 질문에서 고른 차이 보기의 진행 상황이나 결과를 알린다

	comparing  string     // 비교할 리포 최신본을 받는 중인 항목의 이름. 없으면 빈 문자열이다
	diffScreen diffScreen // 비교 보기 화면. name이 비어 있으면 닫혀 있다

	spinner     spinner.Model // 받는 중인 항목이 있거나 비교할 파일을 받는 중일 때만 돈다
	syncResults []syncResult  // 아직 받는 중인 항목이 있어서 상태 메시지로 내보내지 않은 결과

	gauge, gaugeVelocity float64 // 게이지가 지금 그리는 연결 비율과 그 속도. syncedRatio 쪽으로 움직인다
	animating            bool    // animationTick이 예약돼 있으면 true
}

// syncResult 는 받기가 끝난 항목 하나의 결과 문구다. index는 목록에서의 위치로 정렬에 쓴다.
type syncResult struct {
	index int
	text  string
}

// confirmKind 는 어떤 확인 질문을 띄웠는지 나타낸다.
type confirmKind int

const (
	confirmLink        confirmKind = iota // 다른 파일이 있어서 백업한 뒤 연결할지 묻는 중
	confirmUnlink                         // 되돌릴 백업이 없어서 연결을 어떻게 끊을지 묻는 중
	confirmGroupUnlink                    // 켜진 하위 항목이 있어서 그룹 파일과 함께 끌지 묻는 중
	confirmAction                         // enter로 연 항목 메뉴에서 할 일을 고르는 중
)

// New 는 처음 화면 상태를 만든다. 실제 불러오기는 [Model.Init] 에서 시작한다.
func New(client *github.Client, copies *localcopy.Dir) Model {
	home, _ := os.UserHomeDir() // 실패하면 경로를 줄이지 않고 그대로 보여 줄 뿐이라 무시한다
	return Model{client: client, copies: copies, home: home, spinner: spinner.New(spinner.WithSpinner(syncSpinner))}
}

// Init 은 manifest 불러오기를 시작하고, 색을 고르기 위해 터미널 배경색을 물어본다.
func (m Model) Init() tea.Cmd {
	load := func() tea.Msg {
		return loadEntries(context.Background(), m.client, m.copies)
	}
	return tea.Batch(load, tea.RequestBackgroundColor)
}

// Update 는 불러오기와 동기화 결과, 키 입력을 화면 상태에 반영한다.
// 비교 보기 화면이 열려 있으면 키 입력을 모두 scrollDiff로, 확인 질문이 떠 있으면 answerConfirm으로 넘긴다.
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

	case diffLoadedMsg:
		return m.applyDiff(msg), nil

	case animationTickMsg:
		return m.stepAnimation()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
		return m, nil

	case spinner.TickMsg:
		// 받는 중인 항목이 없으면 다음 tick을 예약하지 않아 스피너가 멈춘다. startSync가 다시 돌린다.
		if m.comparing == "" && !slices.ContainsFunc(m.items, func(it item) bool { return it.state == syncing }) {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		key := normalizeKey(msg)
		if m.diffScreen.name != "" {
			return m.scrollDiff(key)
		}
		if m.confirming != "" {
			return m.answerConfirm(key)
		}
		switch key {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor = m.nextVisible(m.cursor, -1)
		case "down", "j":
			m.cursor = m.nextVisible(m.cursor, 1)
		case "left", "h":
			m = m.collapse()
		case "right", "l":
			m = m.expand()
		case "space":
			return m.toggle()
		case "enter":
			return m.openMenu(), nil
		}
	}
	return m, nil
}
