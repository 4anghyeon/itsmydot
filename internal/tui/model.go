// Package tui 는 itsmydot의 터미널 화면이다. 다른 패키지(manifest, github, localcopy, link)를 조립해서 쓴다.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	synced                     // [x] 로컬 사본을 가리키는 symlink 있음
	syncing                    // [·] GitHub에서 받는 중
	conflict                   // [!] target 자리에 다른 파일이 있음
	errored                    // [!] 상태 확인이나 동기화 실패
)

// marker 는 상태를 체크박스 모양으로 바꾼다.
func (s itemState) marker() string {
	switch s {
	case synced:
		return "[x]"
	case syncing:
		return "[·]"
	case conflict, errored:
		return "[!]"
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
	cursor  int   // 선택된 줄 번호 (0부터)
	ready   bool  // manifest를 다 불러왔으면 true
	loadErr error // manifest 자체를 못 불러왔을 때의 에러
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

// Update 는 불러오기 결과와 키 입력(↑/↓ 이동, q 종료)을 반영한다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesLoadedMsg:
		m.ready = true
		m.items = msg.items
		m.loadErr = msg.err
		return m, nil

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
		}
	}
	return m, nil
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

	b.WriteString("\n  ↑/↓ 이동   q 종료\n")
	return tea.NewView(b.String())
}

// writeItems 는 항목 목록을 한 줄씩 쓴다.
//
//	> [x] gitconfig  ~/.gitconfig
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
