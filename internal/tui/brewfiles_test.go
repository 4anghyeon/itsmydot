package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/brewfile"
	"github.com/4anghyeon/itsmydot/internal/homebrew"
	"github.com/4anghyeon/itsmydot/internal/manifest"
	"github.com/4anghyeon/itsmydot/internal/tui/ui"
)

// fakeRunner 는 정해진 설치 상태를 돌려주고 설치 명령에 넘긴 Brewfile 내용을 기록하는 가짜 brew다.
type fakeRunner struct {
	installed homebrew.Installed
	err       error
	brewfile  *string // InstallCommand가 받은 Brewfile 내용
}

func (f fakeRunner) Installed(context.Context) (homebrew.Installed, error) {
	return f.installed, f.err
}

func (f fakeRunner) InstallCommand(path string) *exec.Cmd {
	data, _ := os.ReadFile(path)
	*f.brewfile = string(data)
	return exec.Command("true")
}

const testManifest = `entries: [{name: a, source: x, target: y}]
brewfiles:
  - name: base
    source: files/brew/base.Brewfile
    description: 공통 앱
  - name: dev
    source: files/brew/dev.Brewfile
`

const (
	testBaseBrewfile = `# --- Shell ---
brew "zsh"  # Z Shell
tap "Sanyam-G/switch"
cask "switch"
`
	testDevBrewfile = `brew "git"
brew "PeonPing/tap/peon-ping"
cask "switch"
`
)

// brewFixture 는 base와 dev 프로필을 불러온 brewfiles 목록을 만든다. zsh와 git만 설치돼 있다.
func brewFixture(t *testing.T) (brewfileList, *string) {
	t.Helper()
	client := fakeRepo(t, map[string]string{
		"manifest.yaml":            testManifest,
		"files/brew/base.Brewfile": testBaseBrewfile,
		"files/brew/dev.Brewfile":  testDevBrewfile,
	})
	written := new(string)
	runner := fakeRunner{
		installed: homebrew.Installed{Formulae: map[string]bool{"zsh": true, "git": true}},
		brewfile:  written,
	}
	m := newBrewfileList(client, "")
	m.find = func() (homebrew.Runner, error) { return runner, nil }
	m.tempDir = t.TempDir()
	return updateBrew(m, m.Init()()), written
}

func updateBrew(m brewfileList, msg tea.Msg) brewfileList {
	next, _ := m.Update(msg)
	return next.(brewfileList)
}

func pressBrew(t *testing.T, m brewfileList, keys ...string) (brewfileList, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, key := range keys {
		var msg tea.KeyPressMsg
		switch key {
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "left":
			msg = tea.KeyPressMsg{Code: tea.KeyLeft}
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		default:
			msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(brewfileList)
	}
	return m, cmd
}

func brewView(m brewfileList) string {
	return ansi.Strip(m.View().Content)
}

func TestBrewList(t *testing.T) {
	m, _ := brewFixture(t)
	if m.loadErr != nil || len(m.items) != 8 {
		t.Fatalf("loaded %d items (err %v)", len(m.items), m.loadErr)
	}
	v := brewView(m)
	for _, want := range []string{
		"설치됨 2/5", // switch는 두 프로필에 있어도 한 번만 센다
		"v [ ] base",
		"├── [+] zsh",
		"└── [ ] switch                  cask",
		"v [ ] dev",
		"[ ] PeonPing/tap/peon-ping  brew",
		"┃ 공통 앱",
		"space 고르기",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}

	// 줄 끝 주석이 설명이고 없으면 섹션 주석을 쓴다.
	m, _ = pressBrew(t, m, "down")
	if v := brewView(m); !strings.Contains(v, "┃ Z Shell") {
		t.Errorf("package description missing:\n%s", v)
	}
	m, _ = pressBrew(t, m, "down")
	if v := brewView(m); !strings.Contains(v, "┃ Shell") {
		t.Errorf("section description missing:\n%s", v)
	}

	// 프로필을 접으면 하위 패키지가 숨는다.
	m, _ = pressBrew(t, m, "left")
	if v := brewView(m); m.cursor != 0 || strings.Contains(v, "zsh") || !strings.Contains(v, "> [ ] base") {
		t.Errorf("collapse: cursor=%d\n%s", m.cursor, v)
	}
}

func TestBrewPick(t *testing.T) {
	m, _ := brewFixture(t)

	// 설치된 패키지는 고를 수 없다.
	m, _ = pressBrew(t, m, "down", "space")
	if len(m.picked) != 0 || !strings.Contains(m.Status, "이미 설치돼 있어요") {
		t.Errorf("installed package picked: %v %q", m.picked, m.Status)
	}

	// 고르면 [*]이고 다시 누르면 취소된다. 다른 프로필에 있는 같은 패키지도 함께 고른 것으로 보인다.
	m, _ = pressBrew(t, m, "down", "down", "space")
	if v := brewView(m); !strings.Contains(v, "└── [*] switch") || strings.Count(v, "[*] switch") != 2 {
		t.Errorf("picked switch should show in both profiles:\n%s", v)
	}
	m, _ = pressBrew(t, m, "space")
	if len(m.picked) != 0 {
		t.Errorf("second space should unpick: %v", m.picked)
	}

	// 프로필 줄에서 누르면 설치 안 된 하위 패키지를 모두 고르고 다시 누르면 모두 취소한다.
	m.cursor = 0
	m, _ = pressBrew(t, m, "space")
	if want := []string{"Sanyam-G/switch", "switch"}; !samePackages(m, want) {
		t.Errorf("profile pick = %v, want %v", m.picked, want)
	}
	if v := brewView(m); !strings.Contains(v, "[*] base") {
		t.Errorf("profile with picked children should show [*]:\n%s", v)
	}
	m, _ = pressBrew(t, m, "space")
	if len(m.picked) != 0 {
		t.Errorf("second space on profile should unpick all: %v", m.picked)
	}

	// 고른 게 없으면 enter는 질문 대신 안내를 띄운다.
	m, _ = pressBrew(t, m, "enter")
	if m.confirming || !strings.Contains(m.Status, "먼저 골라") {
		t.Errorf("enter without picks: confirming=%v status=%q", m.confirming, m.Status)
	}
}

func samePackages(m brewfileList, names []string) bool {
	picked := m.pickedPackages()
	if len(picked) != len(names) {
		return false
	}
	for i, p := range picked {
		if p.Name != names[i] {
			return false
		}
	}
	return true
}

func TestBrewInstall(t *testing.T) {
	m, written := brewFixture(t)
	// dev의 peon-ping과 base의 switch, 그 tap을 고른다.
	m, _ = pressBrew(t, m, "down", "down", "down", "space", "up", "space", "down", "down", "down", "down", "space")

	m, _ = pressBrew(t, m, "enter")
	v := brewView(m)
	for _, want := range []string{"3개 패키지를 설치할까요?", "Sanyam-G/switch, switch, PeonPing/tap/peon-ping", "> 1. 설치", "2. 취소"} {
		if !strings.Contains(v, want) {
			t.Errorf("confirm missing %q:\n%s", want, v)
		}
	}

	// esc는 고른 걸 그대로 두고 질문만 닫는다.
	m, _ = pressBrew(t, m, "esc")
	if m.confirming || len(m.pickedPackages()) != 3 {
		t.Fatalf("esc: confirming=%v picked=%d", m.confirming, len(m.pickedPackages()))
	}

	m, cmd := pressBrew(t, m, "enter", "1")
	if cmd == nil || m.confirming {
		t.Fatalf("1 should start the install (cmd=%v)", cmd != nil)
	}
	// tap 줄을 앞에 두고 고른 줄만 원래 모양 그대로 쓴다.
	if want := "tap \"Sanyam-G/switch\"\ncask \"switch\"\nbrew \"PeonPing/tap/peon-ping\"\n"; *written != want {
		t.Errorf("install Brewfile = %q, want %q", *written, want)
	}

	// 설치가 끝나면 설치 상태를 다시 확인한다. switch만 설치에 실패했다.
	m.runner = fakeRunner{installed: homebrew.Installed{
		Formulae: map[string]bool{"zsh": true, "git": true, "peon-ping": true},
		Taps:     map[string]bool{"sanyam-g/switch": true},
	}}
	next, check := m.Update(brewInstallFinishedMsg{})
	m = next.(brewfileList)
	if !m.checking || check == nil || !strings.Contains(brewView(m), "설치 상태를 확인하는 중") {
		t.Fatalf("finished install should re-check (checking=%v)", m.checking)
	}
	m = updateBrew(m, check())
	if m.Status != "3개 중 1개 패키지를 설치하지 못했어요. 실패한 패키지는 고른 채로 남겨 뒀어요" || m.StatusTone != ui.StatusFailed {
		t.Errorf("status = %q (tone %d)", m.Status, m.StatusTone)
	}
	if !samePackages(m, []string{"switch"}) {
		t.Errorf("only the failed package should stay picked: %v", m.picked)
	}
	if v := brewView(m); !strings.Contains(v, "설치됨 4/5") || !strings.Contains(v, "[-] 3개 중 1개") {
		t.Errorf("view after install:\n%s", v)
	}

	// 모두 설치되면 성공으로 알린다.
	m.installing = []string{m.pickedPackages()[0].Key()}
	m = updateBrew(m, brewInstalledCheckedMsg{installed: homebrew.Installed{Casks: map[string]bool{"switch": true}}})
	if m.Status != "1개 패키지를 설치했어요" || len(m.picked) != 0 {
		t.Errorf("all installed: status=%q picked=%v", m.Status, m.picked)
	}
}

func TestBrewMissing(t *testing.T) {
	m := newBrewfileList(nil, "")
	m.find = func() (homebrew.Runner, error) { return nil, homebrew.ErrNotInstalled }
	m = updateBrew(m, m.Init()())
	v := brewView(m)
	for _, want := range []string{"Homebrew가 필요해요", "Homebrew/install/HEAD/install.sh", "Next steps 명령도 실행해 주세요", "esc 진입 메뉴"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing view lacks %q:\n%s", want, v)
		}
	}
	if m, cmd := pressBrew(t, m, "space"); cmd != nil || len(m.picked) != 0 {
		t.Error("list keys should do nothing without Homebrew")
	}
	if _, cmd := pressBrew(t, m, "esc"); cmd == nil {
		t.Error("esc should go back to the menu")
	} else if _, ok := cmd().(backToMenuMsg); !ok {
		t.Error("esc should go back to the menu")
	}
}

func TestBrewLoadFails(t *testing.T) {
	client := fakeRepo(t, map[string]string{"manifest.yaml": testManifest})
	m := newBrewfileList(client, "")
	m.find = func() (homebrew.Runner, error) { return fakeRunner{}, nil }
	m = updateBrew(m, m.Init()())
	if v := brewView(m); !strings.Contains(v, "Brewfile을 불러오지 못했어요. 저장소에 파일이 없어요") {
		t.Errorf("view:\n%s", v)
	}

	m.find = func() (homebrew.Runner, error) { return fakeRunner{err: errors.New("boom")}, nil }
	client = fakeRepo(t, map[string]string{
		"manifest.yaml":            testManifest,
		"files/brew/base.Brewfile": testBaseBrewfile,
		"files/brew/dev.Brewfile":  testDevBrewfile,
	})
	m.client = client
	m = updateBrew(m, m.Init()())
	if v := brewView(m); !strings.Contains(v, "boom") {
		t.Errorf("brew failure should show its error:\n%s", v)
	}
}

// 목록이 터미널보다 길면 남는 높이만큼만 그리고 커서를 따라 창을 옮긴다.
// 화면이 터미널 높이를 넘으면 렌더러가 위쪽 줄을 버리므로 어느 상태에서도 높이 안에 들어와야 한다.
func TestBrewScroll(t *testing.T) {
	const height = 18
	m, _ := brewFixture(t)
	m = updateBrew(m, tea.WindowSizeMsg{Width: 60, Height: height})

	fits := func(step string) string {
		t.Helper()
		v := brewView(m)
		if lines := strings.Count(v, "\n") + 1; lines > height {
			t.Errorf("%s: view is %d lines, taller than the terminal:\n%s", step, lines, v)
		}
		if !strings.Contains(v, "itsmydot") {
			t.Errorf("%s: title cut off:\n%s", step, v)
		}
		return v
	}

	v := fits("first page")
	if !strings.Contains(v, "v [ ] base") || strings.Contains(v, "peon-ping") || !strings.Contains(v, "█") {
		t.Errorf("first page should show the top rows with a scrollbar:\n%s", v)
	}
	// 터미널 창의 스크롤 막대와 겹치지 않게 오른쪽 끝에서 두 칸 떨어져 있다.
	for line := range strings.SplitSeq(v, "\n") {
		if strings.Contains(line, "[") && !strings.HasSuffix(line, "░  ") && !strings.HasSuffix(line, "█  ") {
			t.Errorf("scrollbar should keep a margin from the right edge: %q", line)
		}
	}

	// 맨 아래까지 내려가면 창이 따라 내려가 마지막 줄이 보인다.
	m, _ = pressBrew(t, m, "down", "down", "down", "down", "down", "down", "down")
	if v := fits("bottom"); m.cursor != 7 || !strings.Contains(v, "peon-ping") || strings.Contains(v, "v [ ] base") {
		t.Errorf("bottom: cursor=%d\n%s", m.cursor, v)
	}

	// 창 안에서 움직이면 창은 그대로 있다.
	offset := m.offset
	m, _ = pressBrew(t, m, "up")
	if m.offset != offset {
		t.Errorf("offset moved from %d to %d while the cursor stayed inside the window", offset, m.offset)
	}

	// 확인 질문으로 아래쪽 줄이 늘어도 커서 줄은 보이고 화면은 높이 안에 들어온다.
	m, _ = pressBrew(t, m, "space", "enter")
	if v := fits("confirm"); !m.confirming || !strings.Contains(v, "▌") {
		t.Errorf("confirm: confirming=%v\n%s", m.confirming, v)
	}

	// 맨 위로 돌아오면 첫 줄이 다시 보인다.
	m, _ = pressBrew(t, m, "esc", "up", "up", "up", "up", "up", "up")
	if v := fits("top"); m.cursor != 0 || !strings.Contains(v, "v [ ] base") {
		t.Errorf("top: cursor=%d\n%s", m.cursor, v)
	}
}

// 진입 메뉴에서 brewfiles에 처음 들어갈 때만 불러오고 불러온 뒤에는 카드에 설치된 비율을 보여 준다.
func TestRootLoadsBrewfilesOnce(t *testing.T) {
	m := rootFixture(t)
	m, cmd := pressRoot(t, m, "2")
	if m.screen != brewfilesScreen || cmd == nil {
		t.Fatalf("2 should enter brewfiles and load (screen=%d)", m.screen)
	}
	m, back := pressRoot(t, m, "esc")
	m = updateRoot(m, back())
	if _, cmd = pressRoot(t, m, "2"); cmd != nil {
		t.Error("entering brewfiles again should not reload")
	}

	m = updateRoot(m, brewfilesLoadedMsg{
		items:     []brewItem{{profile: testProfile("base")}, {pkg: testPackage("zsh"), parent: "base"}, {pkg: testPackage("git"), parent: "base"}},
		installed: homebrew.Installed{Formulae: map[string]bool{"zsh": true}},
	})
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "설치됨 1/2") {
		t.Errorf("brewfiles card should show the installed count:\n%s", v)
	}
}

func testProfile(name string) manifest.Brewfile {
	return manifest.Brewfile{Name: name, Source: "files/brew/" + name + ".Brewfile"}
}

func testPackage(name string) brewfile.Package {
	return brewfile.Package{Kind: brewfile.Formula, Name: name, Line: `brew "` + name + `"`}
}
