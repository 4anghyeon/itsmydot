package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// previewFixture 는 가짜 저장소 files와 임시 로컬 사본 폴더로 gitconfig 항목과 extra 항목을 띄운다.
// localCopy가 비어 있지 않으면 gitconfig의 로컬 사본을 그 내용으로 둔다. 저장소 해시는 files 전체로 받은 것으로 한다.
func previewFixture(t *testing.T, files map[string]string, localCopy string, extra ...item) Model {
	t.Helper()
	copies := localcopy.New(t.TempDir())
	e := manifest.Entry{Name: "gitconfig", Source: "files/git/.gitconfig", Target: "/home/me/.gitconfig"}
	copyPath, err := copies.Path(e.Source)
	if err != nil {
		t.Fatal(err)
	}
	if localCopy != "" {
		if _, err := copies.Write(e.Source, []byte(localCopy)); err != nil {
			t.Fatal(err)
		}
	}
	hashes := make(map[string]string, len(files))
	for source, content := range files {
		hashes[source] = github.BlobHash([]byte(content))
	}

	m := New(fakeRepo(t, files), copies, "")
	m.home = "/home/me"
	items := append([]item{{entry: e, copyPath: copyPath, state: synced}}, extra...)
	next, _ := m.Update(entriesLoadedMsg{items: items, hashes: hashes})
	return next.(Model)
}

// 로컬 사본이 저장소와 같으면 요청 없이 사본을 줄 번호와 함께 보여 준다. 탭은 펼치고 창 아래에 읽기 전용 안내를 둔다.
// tab을 다시 누르면 목록으로 돌아간다.
func TestPreviewShowsCurrentLocalCopy(t *testing.T) {
	content := "[user]\n\tname = me\n"
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": content}, content)

	m, cmd := press(t, m, "tab")
	if cmd != nil {
		t.Fatal("a current local copy should not be fetched")
	}
	v := viewText(m)
	for _, want := range []string{
		"gitconfig  ~/.gitconfig  로컬 사본",
		" 1  [user]",
		" 2      name = me",
		previewReadOnlyText,
		"tab/esc 닫기",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("preview missing %q:\n%s", want, v)
		}
	}

	// 창이 떠 있는 동안 목록 키는 먹지 않는다.
	m, cmd = press(t, m, "space")
	if cmd != nil || m.confirmingName != "" || m.preview.name == "" {
		t.Fatal("space acted on the list while the preview was open")
	}

	m, _ = press(t, m, "tab")
	if m.preview.name != "" || !strings.Contains(viewText(m), "▌ [+] gitconfig") {
		t.Errorf("tab should close the preview:\n%s", viewText(m))
	}
}

// 로컬 사본이 없거나 저장소와 다르거나 같은지 알 수 없으면 저장소 파일을 받아서 보여 준다.
// 받는 동안은 스피너를 보여 주고 한 번 받은 파일은 다시 열 때 요청하지 않는다.
func TestPreviewFetchesRepoFile(t *testing.T) {
	tests := []struct {
		name, localCopy string
		withoutHashes   bool
	}{
		{name: "no local copy"},
		{name: "outdated local copy", localCopy: "old\n"},
		{name: "unknown hashes", localCopy: "new\n", withoutHashes: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := previewFixture(t, map[string]string{"files/git/.gitconfig": "new\n"}, tt.localCopy)
			if tt.withoutHashes {
				m.hashes = nil
			}

			m, cmd := press(t, m, "tab")
			if v := viewText(m); cmd == nil || !strings.Contains(v, "] 저장소 파일을 받는 중이에요...") {
				t.Fatalf("want a spinner while fetching (cmd=%v):\n%s", cmd != nil, v)
			}
			m = runCmd(t, m, cmd)
			if v := viewText(m); !strings.Contains(v, "저장소 파일") || !strings.Contains(v, " 1  new") {
				t.Fatalf("preview should show the repo file:\n%s", v)
			}

			m, _ = press(t, m, "esc")
			m, cmd = press(t, m, "tab")
			if cmd != nil || !strings.Contains(viewText(m), " 1  new") {
				t.Errorf("reopening should use the cache (cmd=%v):\n%s", cmd != nil, viewText(m))
			}
		})
	}
}

// 받는 사이에 창을 닫거나 다른 항목을 열면 늦게 온 결과는 보여 주지 않는다. 받은 내용은 캐시에 남는다.
func TestPreviewDropsLateResult(t *testing.T) {
	files := map[string]string{"files/git/.gitconfig": "git\n", "files/zshrc": "zsh\n"}
	m := previewFixture(t, files, "", item{entry: entry("zshrc", "/home/me/.zshrc")})

	m, gitFetch := press(t, m, "tab")
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "down")
	m, zshFetch := press(t, m, "tab")

	m = runCmd(t, m, gitFetch)
	if m.preview.name != "zshrc" || !m.preview.loading {
		t.Fatalf("late gitconfig result replaced the zshrc preview: %+v", m.preview)
	}
	m = runCmd(t, m, zshFetch)
	if v := viewText(m); !strings.Contains(v, " 1  zsh") {
		t.Fatalf("zshrc preview:\n%s", v)
	}

	m, _ = press(t, m, "tab")
	m, _ = press(t, m, "up")
	m, cmd := press(t, m, "tab")
	if cmd != nil || !strings.Contains(viewText(m), " 1  git") {
		t.Errorf("the late gitconfig result should be cached (cmd=%v):\n%s", cmd != nil, viewText(m))
	}
}

// 받지 못했거나 줄 단위로 보여 줄 수 없는 파일이면 창 안에 이유를 보여 준다.
func TestPreviewNotes(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		note  string
	}{
		{"binary", map[string]string{"files/git/.gitconfig": "PNG\x00"}, "텍스트가 아닌 파일이라서 미리 볼 수 없어요"},
		{"too large", map[string]string{"files/git/.gitconfig": strings.Repeat("a", 1<<20+1)}, "파일이 1MB보다 커서 미리 볼 수 없어요"},
		{"empty", map[string]string{"files/git/.gitconfig": ""}, "빈 파일이에요"},
		{"not found", map[string]string{}, "저장소 파일을 받지 못했어요 (저장소에 파일이 없어요)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := previewFixture(t, tt.files, "")
			m, cmd := press(t, m, "tab")
			m = runCmd(t, m, cmd)
			if v := viewText(m); !strings.Contains(v, tt.note) || !strings.Contains(v, previewReadOnlyText) {
				t.Errorf("preview missing %q:\n%s", tt.note, v)
			}
		})
	}
}

// 터미널 높이에 맞춰 본문을 자르고 비교 보기와 같은 키로 스크롤한다.
func TestPreviewScroll(t *testing.T) {
	var text strings.Builder
	for i := 1; i <= 30; i++ {
		_, _ = fmt.Fprintf(&text, "line %d\n", i)
	}
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": text.String()}, text.String())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m, _ = press(t, next.(Model), "tab")

	// 높이 18에서 캐릭터와 구분선, 제목, 안내, 도움말 12줄을 빼면 본문은 6줄이다.
	v := viewText(m)
	if !strings.Contains(v, "6/30줄") || !strings.Contains(v, "line 6") || strings.Contains(v, "line 7") {
		t.Fatalf("first page:\n%s", v)
	}
	if bar := scrollbarColumn(v); bar != "█░░░░░" {
		t.Errorf("scrollbar = %q at the top", bar)
	}
	if lines := strings.Count(v, "\n"); lines > 18 {
		t.Errorf("preview is %d lines tall in an 18 line terminal:\n%s", lines, v)
	}

	m, _ = press(t, m, "down")
	m, _ = press(t, m, "space")
	if m.preview.yOffset != 7 {
		t.Errorf("yOffset = %d, want 7 after down and space", m.preview.yOffset)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(Model)
	if v := viewText(m); m.preview.yOffset != 24 || !strings.Contains(v, "30/30줄") || !strings.Contains(v, "line 30") {
		t.Errorf("yOffset = %d after end:\n%s", m.preview.yOffset, v)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m = next.(Model); m.preview.yOffset != 0 {
		t.Errorf("yOffset = %d after home", m.preview.yOffset)
	}
}

// 터미널이 넓으면 목록 오른쪽에 구분선을 두고 창을 붙인다. 머리에는 목록의 게이지가 그대로 있고 줄 수는 창 제목 끝에 붙는다.
// 좁으면 나누지 않고 화면 전체에 그린다.
func TestPreviewSplitsWideTerminal(t *testing.T) {
	content := "[user]\n\tname = me\n"
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": content}, content, item{entry: entry("zshrc", "/home/me/.zshrc")})
	m, _ = press(t, m, "tab")

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = next.(Model)
	v := viewText(m)
	for _, want := range []string{
		"연결됨 1/2",
		"▌ [+] gitconfig  ~/.gitconfig │ gitconfig  ~/.gitconfig  로컬 사본  2/2줄",
		"  [ ] zshrc      ~/.zshrc     │ ",
		"│ 1  [user]",
		"│ " + previewReadOnlyText,
		"tab/esc 닫기",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("split view missing %q:\n%s", want, v)
		}
	}
	for _, line := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(line); w > 120 {
			t.Errorf("line wider than terminal (%d > 120): %q", w, line)
		}
	}

	// 창이 목록 옆에 있어도 키는 창이 받는다.
	m, _ = press(t, m, "down")
	if m.cursor != 0 {
		t.Errorf("cursor moved to %d while the preview had focus", m.cursor)
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	v = viewText(next.(Model))
	if strings.Contains(v, "[+] gitconfig") || !strings.Contains(v, "2/2줄") || !strings.Contains(v, " 1  [user]") {
		t.Errorf("narrow terminal should show the preview full screen:\n%s", v)
	}
}

// 목록 옆에 붙인 창도 화면 전체일 때와 같은 높이로 본문을 자른다.
func TestPreviewSplitHeight(t *testing.T) {
	var text strings.Builder
	for i := 1; i <= 30; i++ {
		_, _ = fmt.Fprintf(&text, "line %d\n", i)
	}
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": text.String()}, text.String())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	m, _ = press(t, next.(Model), "tab")

	v := viewText(m)
	if !strings.Contains(v, "6/30줄") || !strings.Contains(v, "line 6") || strings.Contains(v, "line 7") {
		t.Fatalf("split first page:\n%s", v)
	}
	if lines := strings.Count(v, "\n"); lines > 18 {
		t.Errorf("split view is %d lines tall in an 18 line terminal:\n%s", lines, v)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if v := viewText(next.(Model)); !strings.Contains(v, "30/30줄") {
		t.Errorf("end should scroll the split preview:\n%s", v)
	}
}

// 업데이트가 있는 항목은 미리보기에서 enter로 사본과 저장소 파일의 차이를 연다. 받아 둔 파일을 쓰므로 다시 요청하지 않는다.
// 비교 보기를 닫으면 미리보기로 돌아온다.
func TestPreviewOpensDiffForOutdatedItem(t *testing.T) {
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": "name = you\n"}, "name = me\n")
	m.items[0].outdated = true
	m, cmd := press(t, m, "tab")
	m = runCmd(t, m, cmd)
	if v := viewText(m); !strings.Contains(v, "enter 차이 보기") {
		t.Fatalf("footer should offer the diff:\n%s", v)
	}

	m, cmd = press(t, m, "enter")
	if cmd != nil || m.diffScreen.name != "gitconfig" || m.comparingName != "" {
		t.Fatalf("diff = %q comparing = %q cmd = %v", m.diffScreen.name, m.comparingName, cmd != nil)
	}
	v := viewText(m)
	for _, want := range []string{"1    -  name = me", "  1  +  name = you"} {
		if !strings.Contains(v, want) {
			t.Errorf("diff missing %q:\n%s", want, v)
		}
	}

	m, _ = press(t, m, "esc")
	if v := viewText(m); m.diffScreen.name != "" || m.preview.name != "gitconfig" || !strings.Contains(v, previewReadOnlyText) {
		t.Errorf("esc should go back to the preview:\n%s", v)
	}
}

// 업데이트 표시가 없는 항목은 도움말 줄에 차이 보기를 알리지 않고 enter를 눌러도 아무것도 열지 않는다.
func TestPreviewWithoutUpdateHasNoDiff(t *testing.T) {
	content := "same\n"
	m := previewFixture(t, map[string]string{"files/git/.gitconfig": content}, content)
	m, _ = press(t, m, "tab")
	if v := viewText(m); strings.Contains(v, "차이 보기") {
		t.Errorf("footer should not offer the diff:\n%s", v)
	}
	m, cmd := press(t, m, "enter")
	if cmd != nil || m.diffScreen.name != "" || m.comparingName != "" || m.preview.name == "" {
		t.Errorf("enter acted on an item without updates: diff = %q preview = %q", m.diffScreen.name, m.preview.name)
	}
}
