// Package tui 는 itsmydot의 터미널 화면이다. 다른 패키지(manifest, github, localcopy, link)를 조립해서 쓴다.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

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
	synced                     // [✓] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [~] GitHub에서 받는 중
	conflict                   // [!] target 자리에 다른 파일이 있음
	errored                    // [✗] 상태 확인이나 동기화 실패
)

// marker 는 상태를 체크박스 모양으로 바꾼다.
// 줄 정렬이 어긋나지 않도록 터미널에서 항상 한 칸 폭인 문자만 쓴다.
func (s itemState) marker() string {
	switch s {
	case synced:
		return "[✓]"
	case syncing:
		return "[~]"
	case conflict:
		return "[!]"
	case errored:
		return "[✗]"
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

	items   []item
	cursor  int    // 선택된 줄 번호 (0부터)
	ready   bool   // manifest를 다 불러왔으면 true
	loadErr error  // manifest 자체를 못 불러왔을 때의 에러
	status  string // 마지막 토글 결과. 목록 아래에 한 줄로 보여 준다
}

// New 는 처음 화면 상태를 만든다. 실제 불러오기는 Init에서 시작한다.
func New(client *github.Client, copies *localcopy.Dir) Model {
	home, _ := os.UserHomeDir() // 실패해도 경로를 줄이지 않고 그대로 보여 줄 뿐이라 무시한다
	return Model{client: client, copies: copies, home: home}
}

// Init 은 manifest 불러오기를 시작한다.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return loadEntries(context.Background(), m.client, m.copies)
	}
}

// Update 는 불러오기/동기화 결과와 키 입력(↑/↓ 이동, space 토글, q 종료)을 반영한다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.ready = true
		m.items = msg.items
		m.loadErr = msg.err
		return m, nil

	case entrySyncedMsg:
		return m.applySynced(msg), nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
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
		restored, err := link.Unlink(it.entry.Target, it.copyPath)
		if err != nil {
			it.state, it.err = errored, err
			m.status = it.entry.Name + ": 해제 실패"
			return m, nil
		}
		it.state, it.err = notSynced, nil
		m.status = it.entry.Name + ": 해제됨"
		if restored != "" {
			// 백업을 되돌렸으면 target 자리에 다시 일반 파일이 있다.
			it.state = conflict
			m.status += " · 원래 파일을 되돌림"
		}
		return m, nil
	}

	// 켜기: GitHub 요청이 있어서 백그라운드에서 처리하고, 끝나면 entrySyncedMsg로 결과를 받는다.
	it.state, it.err = syncing, nil
	m.status = it.entry.Name + ": 받는 중..."
	entry, client, copies := it.entry, m.client, m.copies
	return m, func() tea.Msg {
		return syncEntry(context.Background(), client, copies, entry)
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

// View 는 불러오는 중 / 에러 / 항목 목록 중 하나를 그린다.
func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString("itsmydot\n\n")

	switch {
	case !m.ready:
		b.WriteString("  manifest 불러오는 중...\n")
	case m.loadErr != nil:
		b.WriteString(fmt.Sprintf("  manifest를 불러오지 못했습니다: %v\n", m.loadErr))
	case len(m.items) == 0:
		b.WriteString("  항목이 없습니다.\n")
	default:
		m.writeItems(&b)
	}

	if m.status != "" {
		b.WriteString("\n  " + m.status + "\n")
	}
	b.WriteString("\n  ↑/↓ 이동   space 토글   q 종료\n")
	return tea.NewView(b.String())
}

// writeItems 는 항목 목록을 한 줄씩 쓴다.
//
//	> [✓] gitconfig  ~/.gitconfig
//	  [!] zshrc      ~/.zshrc   (다른 파일이 있음)
func (m Model) writeItems(b *strings.Builder) {
	// 이름 칸 너비를 가장 긴 이름에 맞춘다.
	width := 0
	for _, it := range m.items {
		width = max(width, len(it.entry.Name))
	}

	for i, it := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s %-*s  %s", cursor, it.state.marker(), width, it.entry.Name, tildify(it.entry.Target, m.home)))

		switch it.state {
		case conflict:
			b.WriteString("   (다른 파일이 있음)")
		case errored:
			b.WriteString(fmt.Sprintf("   (%v)", it.err))
		case notSynced, synced, syncing:
			// do nothing
		}
		b.WriteString("\n")
	}
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
