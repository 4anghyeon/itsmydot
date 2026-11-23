package rcfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testLine = "[[ -r ~/.config/zsh/itsmydot.zsh ]] && source ~/.config/zsh/itsmydot.zsh   # itsmydot"

func TestLine(t *testing.T) {
	got := Line("/home/me/.config/zsh/itsmydot.zsh", "/home/me")
	if got != testLine {
		t.Errorf("Line = %q, want %q", got, testLine)
	}

	// 홈 밖의 경로는 그대로 쓴다.
	got = Line("/opt/zsh/itsmydot.zsh", "/home/me")
	want := "[[ -r /opt/zsh/itsmydot.zsh ]] && source /opt/zsh/itsmydot.zsh   # itsmydot"
	if got != want {
		t.Errorf("Line = %q, want %q", got, want)
	}
}

func TestAddAndRemove(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	// 다른 설치 프로그램이 덧붙인 설정이 있고 마지막 줄에 줄바꿈이 없는 경우
	mustWrite(t, rc, "export A=1\n# added by installer\nexport B=2", 0o600)

	added, err := Add(rc, testLine)
	if err != nil || !added {
		t.Fatalf("Add = %v, %v, want true", added, err)
	}
	assertContent(t, rc, "export A=1\n# added by installer\nexport B=2\n"+testLine+"\n")
	assertMode(t, rc, 0o600)

	has, err := Has(rc, testLine)
	if err != nil || !has {
		t.Fatalf("Has = %v, %v, want true", has, err)
	}

	// 이미 있으면 다시 넣지 않는다.
	added, err = Add(rc, testLine)
	if err != nil || added {
		t.Fatalf("second Add = %v, %v, want false", added, err)
	}

	// 넣은 뒤에 사용자가 아래에 덧붙인 설정은 남는다.
	appendText(t, rc, "export C=3\n")
	removed, err := Remove(rc, testLine)
	if err != nil || !removed {
		t.Fatalf("Remove = %v, %v, want true", removed, err)
	}
	assertContent(t, rc, "export A=1\n# added by installer\nexport B=2\nexport C=3\n")
	assertMode(t, rc, 0o600)

	removed, err = Remove(rc, testLine)
	if err != nil || removed {
		t.Fatalf("second Remove = %v, %v, want false", removed, err)
	}
}

func TestAddCreatesMissingFile(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")

	has, err := Has(rc, testLine)
	if err != nil || has {
		t.Fatalf("Has = %v, %v, want false", has, err)
	}
	removed, err := Remove(rc, testLine)
	if err != nil || removed {
		t.Fatalf("Remove = %v, %v, want false", removed, err)
	}

	if _, err := Add(rc, testLine); err != nil {
		t.Fatalf("Add: %v", err)
	}
	assertContent(t, rc, testLine+"\n")
}

func TestLeavesEditedLine(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	// 사용자가 고친 줄은 itsmydot이 넣은 줄로 보지 않는다. 줄 끝 공백과 CRLF만 같은 줄로 본다.
	edited := "source ~/.config/zsh/itsmydot.zsh # itsmydot\n"
	mustWrite(t, rc, edited+testLine+" \r\n", 0o644)

	removed, err := Remove(rc, testLine)
	if err != nil || !removed {
		t.Fatalf("Remove = %v, %v, want true", removed, err)
	}
	assertContent(t, rc, edited)
}

func TestRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "zshrc-original")
	mustWrite(t, real, "export A=1\n", 0o644)
	rc := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(real, rc); err != nil {
		t.Fatal(err)
	}

	if _, err := Add(rc, testLine); !errors.Is(err, ErrSymlink) {
		t.Errorf("Add on symlink = %v, want ErrSymlink", err)
	}
	if _, err := Remove(rc, testLine); !errors.Is(err, ErrSymlink) {
		t.Errorf("Remove on symlink = %v, want ErrSymlink", err)
	}
	assertContent(t, real, "export A=1\n")
	if info, err := os.Lstat(rc); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is no longer a symlink", rc)
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func appendText(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
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

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %v, want %v", path, got, want)
	}
}
