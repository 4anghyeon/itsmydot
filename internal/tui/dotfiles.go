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

// item 은 목록의 한 줄이다.
type item struct {
	entry    manifest.Entry
	copyPath string // 로컬 사본 경로. symlink가 가리켜야 하는 곳이다
	state    itemState
	err      error  // state가 errored일 때의 원인
	parent   string // 하위 항목이면 그룹 이름. 하위 항목은 items에서 그룹 바로 뒤에 이어 붙는다
	outdated bool   // 연결된 항목의 로컬 사본이 시작할 때 본 저장소 파일과 다르면 true

	flash, flashVelocity float64 // 상태가 바뀐 직후 남아 있는 강조색의 정도와 그 속도. 1에서 0으로 줄어든다
}

// dotfileList 는 dotfiles 메뉴의 목록 화면이다. manifest 항목을 보여 주고 켜고 끈다.
type dotfileList struct {
	client  *github.Client
	copies  *localcopy.Dir
	home    string // 화면에서 경로를 ~로 줄여 보여 줄 때와 loaded_by 파일에 넣을 줄을 만들 때 쓴다
	version string // 제목 옆에 보여 주는 itsmydot 버전. 비어 있으면 보여 주지 않는다
	width   int    // 터미널 폭. 모르면 0이고 이때는 화면 폭을 내용에 맞춘다
	height  int    // 터미널 높이. 모르면 0이고 이때는 비교 보기 화면 높이를 내용에 맞춘다
	light   bool   // 터미널 배경이 밝으면 true. 감지 전에는 어두운 배경으로 본다

	items   []item
	cursor  int   // 선택된 줄. 0부터 센다
	ready   bool  // manifest를 다 불러왔으면 true
	loadErr error // manifest 자체를 못 불러왔을 때의 에러

	statusLine

	// hashes 는 시작할 때 받은 저장소 파일의 blob 해시다. 정리한 source 경로가 키이고 받지 못했으면 nil이다.
	hashes map[string]string

	// collapsed 는 접힌 그룹의 이름이다. 바꿀 때는 [withCollapsed] 로 새 map을 만든다.
	collapsed map[string]bool

	// confirmingName 은 확인 질문의 답을 기다리는 항목의 이름이다. 없으면 빈 문자열이다.
	// items는 Update마다 복사되므로 포인터 대신 이름으로 가리킨다.
	confirmingName string
	confirmKind    confirmKind
	choice         int      // 확인 질문에서 커서가 있는 선택지. 0부터 센다
	pendingNames   []string // confirmBackupLink나 confirmTurnOn에서 연결을 고르면 받을 항목들. 그룹 파일이 함께 들어 있을 수 있다
	confirmNote    string   // 확인 질문 아래 한 줄. 질문에서 고른 차이 보기의 진행 상황이나 결과를 알린다

	comparingName string     // 비교할 저장소 최신본을 받는 중인 항목의 이름. 없으면 빈 문자열이다
	diffScreen    diffScreen // 비교 보기 화면. name이 비어 있으면 닫혀 있다
	helpOpen      bool       // ?로 연 도움말 화면이 떠 있으면 true

	// preview 는 tab으로 연 미리보기 창이다. name이 비어 있으면 닫혀 있다.
	preview previewScreen

	// previewCache 는 미리보기로 받은 저장소 파일의 내용이다. 정리한 source 경로가 키이고 바꿀 때는 새 map을 만든다.
	previewCache map[string][]byte

	spinner     spinner.Model // 받는 중인 항목이 있거나 비교할 파일을 받는 중일 때만 돈다
	syncResults []syncResult  // 아직 받는 중인 항목이 있어서 상태 메시지로 내보내지 않은 결과

	gauge, gaugeVelocity float64 // 게이지가 지금 그리는 연결 비율과 그 속도. syncedRatio 쪽으로 움직인다
	animating            bool    // animationTick이 예약돼 있으면 true
}

// newDotfileList 는 처음 목록 상태를 만든다. version은 제목 옆에 보여 줄 버전이다. 실제 불러오기는 [dotfileList.Init] 에서 시작한다.
func newDotfileList(client *github.Client, copies *localcopy.Dir, version string) dotfileList {
	home, _ := os.UserHomeDir() // 실패하면 manifest.Parse도 실패해서 목록이 뜨지 않는다
	return dotfileList{client: client, copies: copies, home: home, version: version, spinner: spinner.New(spinner.WithSpinner(syncSpinner))}
}

// Init 은 manifest 불러오기를 시작한다. 진입 메뉴에서 dotfiles를 처음 고를 때 한 번만 부른다.
func (m dotfileList) Init() tea.Cmd {
	return func() tea.Msg {
		return loadEntries(context.Background(), m.client, m.copies, m.home)
	}
}

// Update 는 불러오기와 동기화 결과, 키 입력을 화면 상태에 반영한다.
func (m dotfileList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.ready = true
		m.items = msg.items
		m.hashes = msg.hashes
		m.loadErr = msg.err
		m.gauge = m.syncedRatio() // 처음 불러올 때는 애니메이션 없이 바로 맞춘다
		return m, nil

	case entrySyncedMsg:
		return m.applySynced(msg).startAnimation()

	case diffLoadedMsg:
		return m.applyDiff(msg), nil

	case previewLoadedMsg:
		return m.applyPreview(msg), nil

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
		if m.comparingName == "" && !m.preview.loading && !slices.ContainsFunc(m.items, func(it item) bool { return it.state == syncing }) {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		key := normalizeKey(msg)
		if m.helpOpen {
			return m.handleHelpKey(key)
		}
		if m.diffScreen.name != "" {
			return m.handleDiffKey(key)
		}
		if m.preview.name != "" {
			return m.handlePreviewKey(key)
		}
		if m.confirmingName != "" {
			return m.handleConfirmKey(key)
		}
		m = m.countStatusKey()
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, backToMenu
		case "up", "k":
			m.cursor = treeNextVisible(m.items, m.collapsed, m.cursor, -1)
		case "down", "j":
			m.cursor = treeNextVisible(m.items, m.collapsed, m.cursor, 1)
		case "left", "h":
			m = m.collapse()
		case "right", "l":
			m = m.expand()
		case "space":
			return m.toggle()
		case "enter":
			return m.openMenu(), nil
		case "tab":
			return m.openPreview()
		case "?":
			m.helpOpen = true
		}
	}
	return m, nil
}
