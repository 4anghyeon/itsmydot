// Package tui 는 itsmydot의 터미널 화면이다. 다른 패키지(manifest, github, localcopy, link)를 조립해서 쓴다.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/link"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// itemState 는 화면에 표시하는 항목 하나의 상태다.
// link.State(디스크 상태)에 화면에서만 쓰는 상태(syncing, errored)를 더한 것이다.
type itemState int

const (
	notSynced itemState = iota // [ ] symlink 없음
	synced                     // [+] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [~] GitHub에서 받는 중
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
	return Model{client: client, copies: copies, home: home}
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
		return m, nil

	case entrySyncedMsg:
		return m.applySynced(msg), nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.BackgroundColorMsg:
		m.light = !msg.IsDark()
		return m, nil

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

// toggle 은 커서가 있는 항목을 켜거나 끈다.
//
// 화면에 표시된 상태가 아니라 디스크를 다시 확인해서 결정한다.
// errored 항목도 실제로 연결돼 있으면 끄고, 아니면 다시 받아서 연결을 시도한다.
// target 자리에 다른 파일이 있으면 바로 연결하지 않고 확인 질문을 띄운다.
func (m Model) toggle() (Model, tea.Cmd) {
	if !m.ready || len(m.items) == 0 {
		return m, nil
	}
	// m.items는 원래 Model과 같은 배열을 가리키므로, 고치기 전에 복사해서 원래 상태를 건드리지 않는다.
	m.items = slices.Clone(m.items)
	it := &m.items[m.cursor]
	if it.state == syncing {
		return m, nil
	}

	state, err := link.Check(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 상태 확인 실패"
		return m, nil
	}

	// 끄기: 로컬 작업이라 바로 처리한다.
	if state == link.Synced {
		// 되돌릴 백업이 없으면 target 자리가 비게 되므로 먼저 묻는다.
		// 처음부터 파일이 없었던 건지, 백업이 사고로 사라진 건지 디스크만으로는 구분할 수 없다.
		has, err := link.HasBackup(it.entry.Target)
		if err != nil {
			it.state, it.err = errored, err
			m.status = it.entry.Name + ": 백업 확인 실패"
			return m, nil
		}
		if !has {
			m.confirming, m.confirmKind = it.entry.Name, confirmUnlink
			return m, nil
		}
		return m.unlink(m.cursor), nil
	}

	// 다른 파일이 있으면 백업이 일어나므로 먼저 묻는다. 백업 자체는 생략할 수 없고, 진행 여부만 고른다.
	if state == link.Conflict {
		it.state, it.err = conflict, nil
		m.confirming, m.confirmKind = it.entry.Name, confirmLink
		return m, nil
	}

	return m.startSync(m.cursor)
}

// unlink 는 i번째 항목의 연결을 끊고, 백업이 있으면 원래 파일을 되돌린다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) unlink(i int) Model {
	it := &m.items[i]
	restored, err := link.Unlink(it.entry.Target, it.copyPath)
	if err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m
	}
	it.state, it.err = notSynced, nil
	m.status = it.entry.Name + ": 해제됨"
	if restored != "" {
		// 백업을 되돌렸으면 target 자리에 다시 일반 파일이 있다.
		it.state = conflict
		m.status += " · 원래 파일을 되돌림"
	}
	return m
}

// detach 는 i번째 항목의 연결만 끊고, 지금 설정은 target에 일반 파일로 남긴다.
// m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) detach(i int) Model {
	it := &m.items[i]
	if err := link.Detach(it.entry.Target, it.copyPath); err != nil {
		it.state, it.err = errored, err
		m.status = it.entry.Name + ": 해제 실패"
		return m
	}
	// target 자리에 일반 파일이 남았으므로 디스크 기준으로는 conflict다.
	it.state, it.err = conflict, nil
	m.status = it.entry.Name + ": 연결 끊음 · 지금 설정은 " + tildify(it.entry.Target, m.home) + " 에 남겨 둠"
	return m
}

// answerConfirm 은 확인 질문에 대한 키 입력(선택지 번호, esc 취소)을 처리한다.
// y/n 대신 숫자를 쓰는 이유는, 한글 입력기가 자모를 조합 대기로 붙잡아 y/n이 늦게 도착하기 때문이다.
// 그 밖의 키는 무시해서, 답하기 전에 다른 항목을 건드리지 않게 한다.
func (m Model) answerConfirm(key string) (Model, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	_, options := m.confirmPrompt()
	cancel := strconv.Itoa(len(options)) // 마지막 선택지는 항상 "취소"
	if key == "esc" {
		key = cancel
	}
	n, err := strconv.Atoi(key)
	if err != nil || n < 1 || n > len(options) {
		return m, nil
	}

	name := m.confirming
	m.confirming = ""
	if key == cancel {
		m.status = name + ": 취소됨"
		return m, nil
	}

	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == name })
	if i < 0 {
		return m, nil
	}
	m.items = slices.Clone(m.items)

	switch m.confirmKind {
	case confirmLink: // 1. 백업 후 연결
		return m.startSync(i)
	case confirmUnlink:
		if n == 1 { // 1. 지금 설정은 남겨 두고 연결만 끊기
			return m.detach(i), nil
		}
		// 2. 연결을 끊고 파일도 지우기
		m = m.unlink(i)
		if m.items[i].state == notSynced {
			m.status = name + ": 연결 끊음 · 내용은 " + tildify(m.items[i].copyPath, m.home) + " 에 남아 있음"
		}
		return m, nil
	}
	return m, nil
}

// startSync 는 i번째 항목을 syncing으로 바꾸고, 받아서 연결하는 작업을 백그라운드로 시작한다.
// 끝나면 entrySyncedMsg로 결과가 돌아온다. m.items는 호출하기 전에 복사돼 있어야 한다.
func (m Model) startSync(i int) (Model, tea.Cmd) {
	it := &m.items[i]
	it.state, it.err = syncing, nil
	m.status = it.entry.Name + ": 받는 중..."
	entry, client, copies := it.entry, m.client, m.copies
	return m, func() tea.Msg {
		return syncEntry(context.Background(), client, copies, entry)
	}
}

// confirmPrompt 는 확인 질문 문구와 선택지를 만든다. 마지막 선택지는 항상 "취소"다.
func (m Model) confirmPrompt() (question string, options []string) {
	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == m.confirming })
	if i < 0 {
		return "", []string{"취소"}
	}
	target := tildify(m.items[i].entry.Target, m.home)

	switch m.confirmKind {
	case confirmUnlink:
		return fmt.Sprintf("%s 의 원래 파일(백업)을 찾을 수 없습니다. 연결을 어떻게 끊을까요?", target),
			[]string{"지금 설정은 남겨 두고 연결만 끊기 (추천)", "연결을 끊고 파일도 지우기", "취소"}
	default:
		return fmt.Sprintf("%s 에 이미 파일이 있습니다. 기존 파일을 백업한 뒤 연결할까요?", target),
			[]string{"백업 후 연결", "취소"}
	}
}

// applySynced 는 syncEntry의 결과를 해당 항목에 반영한다.
func (m Model) applySynced(msg entrySyncedMsg) Model {
	i := slices.IndexFunc(m.items, func(it item) bool { return it.entry.Name == msg.name })
	if i < 0 {
		return m
	}
	m.items = slices.Clone(m.items)
	it := &m.items[i]

	if msg.err != nil {
		it.state, it.err = errored, msg.err
		m.status = it.entry.Name + ": 연결 실패"
		return m
	}
	it.state, it.err = synced, nil
	m.status = it.entry.Name + ": 연결됨"
	if msg.backup != "" {
		m.status += " · 기존 파일 백업: " + tildify(msg.backup, m.home)
	}
	return m
}

// View 는 상자 안에 제목·진행 게이지, 항목 목록(또는 불러오는 중/에러), 상태 메시지나 확인 질문을 그리고,
// 상자 아래에 도움말을 붙인다.
func (m Model) View() tea.View {
	st := newStyles(!m.light)

	var body []string
	switch {
	case !m.ready:
		body = append(body, st.muted.Render("manifest 불러오는 중..."))
	case m.loadErr != nil:
		body = append(body, st.state(errored).Render("manifest를 불러오지 못했습니다: "+m.loadErr.Error()))
	case len(m.items) == 0:
		body = append(body, st.muted.Render("항목이 없습니다."))
	default:
		body = append(body, m.itemLines(st)...)
	}

	footer := st.help("방향키", "이동", "space", "토글", "esc", "종료")
	if m.confirming != "" {
		question, options := m.confirmPrompt()
		body = append(body, "", st.question.Render(question), "")
		numbers := make([]string, len(options))
		for i, opt := range options {
			numbers[i] = strconv.Itoa(i + 1)
			body = append(body, st.key.Render(numbers[i]+".")+" "+opt)
		}
		footer = st.help(strings.Join(numbers, "/"), "선택", "esc", "취소")
	} else if m.status != "" {
		body = append(body, "", m.status)
	}

	// 상자 안쪽 폭: 가장 긴 줄에 맞추되, 터미널보다 넓으면 터미널 폭에 맞추고 긴 줄은 상자 안에서 줄바꿈한다.
	title := st.title.Render("itsmydot")
	right := ""
	if m.ready && m.loadErr == nil && len(m.items) > 0 {
		done := 0
		for _, it := range m.items {
			if it.state == synced {
				done++
			}
		}
		right = st.muted.Render(fmt.Sprintf("연결됨 %d/%d ", done, len(m.items))) + st.gauge(done, len(m.items))
	}
	inner := lipgloss.Width(title) + 4 + lipgloss.Width(right)
	for _, line := range body {
		inner = max(inner, lipgloss.Width(line))
	}
	if m.width > 0 && inner+frameExtra > m.width {
		inner = max(m.width-frameExtra, 1)
	}

	header := title + strings.Repeat(" ", max(inner-lipgloss.Width(title)-lipgloss.Width(right), 1)) + right
	content := header + "\n\n" + strings.Join(body, "\n")
	box := st.frame.Width(inner + frameExtra).Render(content)
	return tea.NewView(box + "\n " + footer + "\n")
}

// itemLines 는 항목 목록을 한 줄씩 만든다.
//
//	> [+] gitconfig  ~/.gitconfig
//	  [!] zshrc      ~/.zshrc   (다른 파일이 있음)
func (m Model) itemLines(st styles) []string {
	// 이름 칸 너비를 가장 긴 이름에 맞춘다.
	width := 0
	for _, it := range m.items {
		width = max(width, lipgloss.Width(it.entry.Name))
	}

	lines := make([]string, 0, len(m.items))
	for i, it := range m.items {
		cursor, name := " ", it.entry.Name+strings.Repeat(" ", width-lipgloss.Width(it.entry.Name))
		if i == m.cursor {
			cursor, name = st.cursor.Render(">"), st.selected.Render(name)
		}
		line := cursor + " " + st.state(it.state).Render(it.state.marker()) + " " + name + "  " + st.muted.Render(tildify(it.entry.Target, m.home))

		switch it.state {
		case conflict:
			line += st.state(it.state).Render("   (다른 파일이 있음)")
		case errored:
			line += st.state(it.state).Render(fmt.Sprintf("   (%v)", it.err))
		case notSynced, synced, syncing:
			// 체크박스만으로 충분해서 덧붙일 설명이 없다.
		}
		lines = append(lines, line)
	}
	return lines
}

// loadEntries 는 GitHub에서 manifest를 받아 파싱하고, 각 항목의 현재 상태를 확인한다.
// 디스크는 바꾸지 않는다. Init의 백그라운드 작업에서 호출된다.
func loadEntries(ctx context.Context, client *github.Client, copies *localcopy.Dir) entriesLoadedMsg {
	// 1. manifest 받기 + 파싱
	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return entriesLoadedMsg{err: err}
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return entriesLoadedMsg{err: err}
	}

	// 2. 항목마다 상태 확인. 한 항목이 실패해도 나머지는 보여 주도록 그 항목만 errored로 표시한다.
	items := make([]item, 0, len(m.Entries))
	for _, e := range m.Entries {
		it := item{entry: e}
		it.copyPath, err = copies.Path(e.Source)
		if err == nil {
			var s link.State
			s, err = link.Check(e.Target, it.copyPath)
			it.state = fromLinkState(s)
		}
		if err != nil {
			it.state, it.err = errored, err
		}
		items = append(items, it)
	}
	return entriesLoadedMsg{items: items}
}

// syncEntry 는 항목 하나를 GitHub에서 최신본으로 받아 로컬 사본에 저장하고 target에 연결한다.
// 이미 연결된 항목이면 로컬 사본만 최신으로 바뀐다.
func syncEntry(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name}

	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	copyPath, err := copies.Write(e.Source, data)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.backup, msg.err = link.Link(e.Target, copyPath)
	return msg
}

// fromLinkState 는 link 패키지의 디스크 상태를 화면 상태로 바꾼다.
func fromLinkState(s link.State) itemState {
	switch s {
	case link.Synced:
		return synced
	case link.Conflict:
		return conflict
	default:
		return notSynced
	}
}

// tildify 는 홈 디렉토리로 시작하는 경로를 ~로 줄인다. "/Users/me/.zshrc" → "~/.zshrc"
func tildify(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
