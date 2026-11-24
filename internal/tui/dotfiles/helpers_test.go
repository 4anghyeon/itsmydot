package dotfiles

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github/githubtest"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// press 는 키 하나를 누른 것처럼 Update를 호출하고 새 목록 모델을 돌려준다.
func press(t *testing.T, m List, key string) (List, tea.Cmd) {
	t.Helper()
	var msg tea.KeyPressMsg
	switch key {
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		msg = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		msg = tea.KeyPressMsg{Code: tea.KeyRight}
	case "space":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	default:
		msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	next, cmd := m.Update(msg)
	return next.(List), cmd
}

func loadedModel(items ...item) List {
	m := List{home: "/home/me"}
	next, _ := m.Update(entriesLoadedMsg{items: items})
	return next.(List)
}

func entry(name, target string) manifest.Entry {
	return manifest.Entry{Name: name, Source: "files/" + name, Target: target}
}

// viewText 는 화면에서 색상 코드를 걷어낸 글자만 돌려준다.
func viewText(m List) string {
	return ansi.Strip(m.View().Content)
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

// toggleFixture 는 가짜 저장소, 임시 로컬 사본 폴더, 임시 홈에 놓인 항목 하나로 목록 모델을 만든다.
func toggleFixture(t *testing.T, files map[string]string) (m List, target string) {
	t.Helper()
	home := t.TempDir()
	copies := localcopy.New(t.TempDir())
	target = filepath.Join(home, ".gitconfig")

	e := manifest.Entry{Name: "gitconfig", Source: "files/git/.gitconfig", Target: target}
	copyPath, err := copies.Path(e.Source)
	if err != nil {
		t.Fatal(err)
	}
	m = New(githubtest.NewClient(t, files), copies, "", EntriesSection)
	m.home = home
	next, _ := m.Update(entriesLoadedMsg{items: []item{{entry: e, copyPath: copyPath}}})
	return next.(List), target
}

// confirmToggle 은 space로 토글 확인 질문을 띄우고 1번으로 답한다.
// 다른 질문이 확인을 겸하는 경우에는 쓰지 않는다.
func confirmToggle(t *testing.T, m List) (List, tea.Cmd) {
	t.Helper()
	m, cmd := press(t, m, "space")
	if cmd != nil || (m.confirmKind != confirmTurnOn && m.confirmKind != confirmTurnOff) {
		t.Fatalf("want a toggle prompt, got cmd=%v kind=%d", cmd != nil, m.confirmKind)
	}
	return press(t, m, "1")
}

// linkedFixture 는 백업 없이 연결된 항목 하나로 목록 모델을 만든다.
func linkedFixture(t *testing.T) (List, string) {
	t.Helper()
	m, target := toggleFixture(t, map[string]string{"files/git/.gitconfig": "from repo"})
	m, cmd := confirmToggle(t, m)
	m = runCmd(t, m, cmd)
	if m.items[0].state != ui.Synced {
		t.Fatalf("setup: state = %d (err %v)", m.items[0].state, m.items[0].err)
	}
	return m, target
}

// runCmd 는 백그라운드 작업을 바로 실행하고 그 결과 메시지를 Update에 넣는다.
// tea.Batch로 묶인 작업은 풀어서 하나씩 실행하고 스피너 tick은 건너뛴다.
func runCmd(t *testing.T, m List, cmd tea.Cmd) List {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			continue
		}
		next, _ := m.Update(msg)
		m = next.(List)
	}
	return m
}

// runAll 은 cmd를 실행해 나온 메시지를 모두 모은다. tea.Batch로 묶인 작업도 풀어서 실행한다.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, runAll(c)...)
	}
	return msgs
}
