package brewfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got := Parse([]byte(`# --- Shell ---
brew "zsh"                            # Z Shell
brew "zsh-completions"

# --- Fonts ---
cask "font-pretendard"
tap "Sanyam-G/switch"
cask 'switch'  # alt-tab "대체"
brew "PeonPing/tap/peon-ping", args: ["HEAD"] # 알림 # 사운드
mas "Xcode", id: 497799835
vscode "golang.go"
brew zsh
brew ""
cask "unterminated
`))
	want := []Package{
		{Formula, "zsh", `brew "zsh"`, "Z Shell"},
		{Formula, "zsh-completions", `brew "zsh-completions"`, "Shell"},
		{Cask, "font-pretendard", `cask "font-pretendard"`, "Fonts"},
		{Tap, "Sanyam-G/switch", `tap "Sanyam-G/switch"`, "Fonts"},
		{Cask, "switch", `cask 'switch'`, `alt-tab "대체"`},
		{Formula, "PeonPing/tap/peon-ping", `brew "PeonPing/tap/peon-ping", args: ["HEAD"]`, "알림 # 사운드"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse =\n%+v\nwant\n%+v", got, want)
	}
}

func TestNames(t *testing.T) {
	tests := []struct {
		p          Package
		short, key string
	}{
		{Package{Kind: Formula, Name: "PeonPing/tap/peon-ping"}, "peon-ping", "brew peonping/tap/peon-ping"},
		{Package{Kind: Cask, Name: "raycast"}, "raycast", "cask raycast"},
		{Package{Kind: Tap, Name: "Sanyam-G/switch"}, "Sanyam-G/switch", "tap sanyam-g/switch"},
	}
	for _, tt := range tests {
		if got := tt.p.ShortName(); got != tt.short {
			t.Errorf("%s ShortName = %q, want %q", tt.p.Name, got, tt.short)
		}
		if got := tt.p.Key(); got != tt.key {
			t.Errorf("%s Key = %q, want %q", tt.p.Name, got, tt.key)
		}
	}
}

// 저장소의 Brewfile은 줄마다 패키지 하나라 읽은 패키지 수가 tap, brew, cask로 시작하는 줄 수와 같아야 한다.
func TestRepoBrewfiles(t *testing.T) {
	paths, err := filepath.Glob("../../files/brew/*.Brewfile")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Brewfile in the repo (err %v)", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		for line := range strings.SplitSeq(string(data), "\n") {
			word, _, _ := strings.Cut(strings.TrimSpace(line), " ")
			if word == "tap" || word == "brew" || word == "cask" {
				want++
			}
		}
		if got := len(Parse(data)); got != want {
			t.Errorf("%s: %d packages, want %d", path, got, want)
		}
	}
}
