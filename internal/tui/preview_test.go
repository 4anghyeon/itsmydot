package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4anghyeon/itsmydot/internal/diff"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

func TestTabOpensAndClosesPreview(t *testing.T) {
	temp := t.TempDir()
	copyPath := filepath.Join(temp, "gitconfig")
	if err := os.WriteFile(copyPath, []byte("[user]\n\tname = me\n"), 0600); err != nil {
		t.Fatal(err)
	}

	it := item{
		entry:    manifest.Entry{Name: "gitconfig", Source: "files/gitconfig", Target: "~/.gitconfig"},
		copyPath: copyPath,
		state:    synced,
	}
	m := loadedModel(it)

	// Tab을 누르면 미리보기가 열린다
	m, cmd := press(t, m, "tab")
	if cmd != nil {
		t.Fatalf("로컬 사본이 있으면 원격 요청 없이 바로 열려야 하는데 cmd가 반환됨")
	}
	if m.previewScreen.name != "gitconfig" {
		t.Fatalf("미리보기 이름이 gitconfig 여야 하는데 %q", m.previewScreen.name)
	}
	text := viewText(m)
	if !strings.Contains(text, "읽기 전용이에요. 고치려면 GitHub 저장소에서 편집해 주세요") {
		t.Errorf("안내 문구가 보여야 하는데 안 보임: %s", text)
	}
	if !strings.Contains(text, "name = me") {
		t.Errorf("파일 내용이 보여야 하는데 안 보임: %s", text)
	}

	// 다시 Tab을 누르면 닫힌다
	m, _ = press(t, m, "tab")
	if m.previewScreen.name != "" {
		t.Fatalf("Tab을 다시 누르면 미리보기가 닫혀야 함")
	}

	// 다시 열고 esc를 누르면 닫힌다
	m, _ = press(t, m, "tab")
	if m.previewScreen.name != "gitconfig" {
		t.Fatalf("미리보기가 다시 열려야 함")
	}
	m, _ = press(t, m, "esc")
	if m.previewScreen.name != "" {
		t.Fatalf("esc를 누르면 미리보기가 닫혀야 함")
	}
}

func TestPreviewScrollKeys(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 50; i++ {
		sb.WriteString("line\n")
	}
	temp := t.TempDir()
	copyPath := filepath.Join(temp, "long")
	if err := os.WriteFile(copyPath, []byte(sb.String()), 0600); err != nil {
		t.Fatal(err)
	}

	it := item{
		entry:    manifest.Entry{Name: "long", Source: "files/long"},
		copyPath: copyPath,
		state:    synced,
	}
	m := loadedModel(it)
	m, _ = press(t, m, "tab")

	// 아래 방향키로 스크롤
	m, _ = press(t, m, "down")
	if m.previewScreen.yOffset != 1 {
		t.Errorf("down 후 yOffset이 1이어야 하는데 %d", m.previewScreen.yOffset)
	}

	// end로 맨 끝으로 이동
	m, _ = press(t, m, "end")
	if m.previewScreen.yOffset <= 1 {
		t.Errorf("end 후 yOffset이 끝 쪽이어야 함: %d", m.previewScreen.yOffset)
	}

	// home으로 맨 위로 이동
	m, _ = press(t, m, "home")
	if m.previewScreen.yOffset != 0 {
		t.Errorf("home 후 yOffset이 0이어야 하는데 %d", m.previewScreen.yOffset)
	}
}

func TestPreviewCacheAndLateDiscard(t *testing.T) {
	it := item{
		entry: manifest.Entry{Name: "remote-file", Source: "files/remote-file"},
		state: notSynced,
	}
	m := loadedModel(it)

	// 로컬 사본이 없으므로 Tab을 누르면 비동기 요청을 시작한다
	m, cmd := press(t, m, "tab")
	if cmd == nil {
		t.Fatalf("로컬 사본이 없으면 원격 요청 cmd가 반환되어야 함")
	}
	if !m.previewScreen.loading {
		t.Fatalf("로딩 상태여야 함")
	}

	// 다른 항목을 보거나 닫은 뒤 도착한 늦은 결과는 버린다
	closed := m
	closed.previewScreen = previewScreen{}
	closed = closed.applyPreview(previewLoadedMsg{name: "remote-file", source: "files/remote-file", data: []byte("content")})
	if closed.previewScreen.name != "" {
		t.Fatalf("닫힌 상태에서 도착한 결과는 버려져야 함")
	}

	// 정상 수신 시 캐시되고 화면에 내용이 반영된다
	m = m.applyPreview(previewLoadedMsg{name: "remote-file", source: "files/remote-file", data: []byte("hello cached")})
	if m.previewScreen.loading {
		t.Fatalf("로딩이 끝나야 함")
	}
	if len(m.previewScreen.lines) == 0 || m.previewScreen.lines[0] != "hello cached" {
		t.Fatalf("내용이 반영되어야 함: %v", m.previewScreen.lines)
	}
	if string(m.previewCache["files/remote-file"]) != "hello cached" {
		t.Fatalf("캐시에 저장되어야 함")
	}

	// 닫았다가 다시 열면 캐시에서 바로 불러온다
	m, _ = press(t, m, "tab")
	m, nextCmd := press(t, m, "tab")
	if nextCmd != nil {
		t.Fatalf("캐시된 파일은 네트워크 요청 없이 열려야 함")
	}
	if len(m.previewScreen.lines) == 0 || m.previewScreen.lines[0] != "hello cached" {
		t.Fatalf("캐시된 내용이 보여야 함")
	}
}

func TestPreviewBinaryAndTooLarge(t *testing.T) {
	itBinary := item{entry: manifest.Entry{Name: "bin", Source: "files/bin"}}
	m := loadedModel(itBinary)
	m = m.showPreviewContent("bin", "files/bin", []byte("hello\x00world"), nil)
	if !strings.Contains(m.previewScreen.err, "텍스트가 아니라서") {
		t.Errorf("바이너리 에러가 표시되어야 하는데: %s", m.previewScreen.err)
	}

	large := make([]byte, diff.MaxSize+1)
	m = m.showPreviewContent("big", "files/big", large, nil)
	if !strings.Contains(m.previewScreen.err, "커서 미리 볼 수 없어요") {
		t.Errorf("용량 초과 에러가 표시되어야 하는데: %s", m.previewScreen.err)
	}
}

func TestPreviewSplitAndFullScreenLayout(t *testing.T) {
	temp := t.TempDir()
	copyPath := filepath.Join(temp, "cfg")
	if err := os.WriteFile(copyPath, []byte("alpha\nbeta\n"), 0600); err != nil {
		t.Fatal(err)
	}

	it := item{
		entry:    manifest.Entry{Name: "cfg", Source: "files/cfg"},
		copyPath: copyPath,
		state:    synced,
	}

	// 좁은 터미널에서는 전체 화면
	narrow := loadedModel(it)
	narrow.width = 80
	narrow.height = 24
	narrow, _ = press(t, narrow, "tab")
	if narrow.isSplitPreview() {
		t.Errorf("폭 80에서는 분할이 아니어야 함")
	}
	narrowText := viewText(narrow)
	if !strings.Contains(narrowText, "읽기 전용이에요") {
		t.Errorf("전체 화면 미리보기에 안내 문구가 있어야 함: %s", narrowText)
	}

	// 넓은 터미널에서는 좌우 분할
	wide := loadedModel(it)
	wide.width = 120
	wide.height = 24
	wide, _ = press(t, wide, "tab")
	if !wide.isSplitPreview() {
		t.Errorf("폭 120에서는 분할이어야 함")
	}
	wideText := viewText(wide)
	if !strings.Contains(wideText, "│") {
		t.Errorf("분할 화면에는 구분자가 있어야 함: %s", wideText)
	}
	if !strings.Contains(wideText, "읽기 전용이에요") {
		t.Errorf("분할 화면에도 안내 문구가 있어야 함: %s", wideText)
	}
}
