package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fakeBrew 는 인자에 따라 정해진 목록을 출력하는 가짜 brew 스크립트를 만든다. 실제 brew는 부르지 않는다.
func fakeBrew(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "brew")
	script := `#!/bin/sh
case "$*" in
  "list --formula -1") printf 'git\nPeon-Ping\n' ;;
  "list --cask -1") printf 'raycast\n' ;;
  "tap") printf 'sanyam-g/switch\n' ;;
  *) echo "unexpected $*" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstalled(t *testing.T) {
	b := &Brew{Path: fakeBrew(t)}
	in, err := b.Installed(context.Background())
	if err != nil {
		t.Fatalf("Installed: %v", err)
	}
	want := Installed{
		Formulae: map[string]bool{"git": true, "peon-ping": true},
		Casks:    map[string]bool{"raycast": true},
		Taps:     map[string]bool{"sanyam-g/switch": true},
	}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("Installed = %+v, want %+v", in, want)
	}

	tests := []struct {
		p    Package
		want bool
	}{
		{Package{Kind: Formula, Name: "PeonPing/tap/peon-ping"}, true},
		{Package{Kind: Tap, Name: "Sanyam-G/switch"}, true},
		{Package{Kind: Cask, Name: "raycast"}, true},
		{Package{Kind: Formula, Name: "raycast"}, false}, // 종류가 다르면 다른 패키지다
		{Package{Kind: Cask, Name: "zed"}, false},
	}
	for _, tt := range tests {
		if got := in.Has(tt.p); got != tt.want {
			t.Errorf("Has(%s %s) = %v, want %v", tt.p.Kind, tt.p.Name, got, tt.want)
		}
	}
}

func TestInstalledFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brew")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho broken >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := (&Brew{Path: path}).Installed(context.Background())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v, want brew's stderr in it", err)
	}
}

func TestInstallCommand(t *testing.T) {
	cmd := (&Brew{Path: "/x/brew"}).InstallCommand("/tmp/a.Brewfile")
	if want := []string{"/x/brew", "bundle", "--file", "/tmp/a.Brewfile"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
}

func TestFind(t *testing.T) {
	saved := searchPaths
	t.Cleanup(func() { searchPaths = saved })

	dir := t.TempDir()
	t.Setenv("PATH", dir)
	searchPaths = []string{filepath.Join(dir, "missing")}
	if _, err := Find(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("without brew: err = %v, want ErrNotInstalled", err)
	}

	// PATH에 없어도 기본 설치 경로에 있으면 찾는다.
	fake := fakeBrew(t)
	searchPaths = []string{filepath.Join(dir, "missing"), fake}
	if b, err := Find(); err != nil || b.Path != fake {
		t.Errorf("Find = %v, %v, want %s", b, err, fake)
	}
}
