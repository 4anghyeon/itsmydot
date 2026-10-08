package localcopy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	// t.TempDir()은 테스트용 임시 디렉토리를 만들고, 테스트가 끝나면 자동으로 지운다.
	root := t.TempDir()
	c := New(root)

	got, err := c.Write("files/zsh/.zshrc", []byte("v1"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// 리포 경로가 그대로 미러링됐는지 확인한다.
	want := filepath.Join(root, "files", "zsh", ".zshrc")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	assertFile(t, want, "v1")

	info, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("perm = %o, want 644", perm)
	}

	// 다시 쓰면 최신 내용으로 덮어써야 한다.
	if _, err := c.Write("files/zsh/.zshrc", []byte("v2")); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	assertFile(t, want, "v2")

	// 임시 파일이 남아 있으면 안 된다.
	entries, err := os.ReadDir(filepath.Dir(want))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only .zshrc: %v", len(entries), entries)
	}
}

// symlink가 가리키고 있는 사본 파일을 덮어써도, symlink를 통해 새 내용이 보여야 한다.
func TestWriteThroughSymlink(t *testing.T) {
	root := t.TempDir()
	c := New(root)

	dst, err := c.Write("files/git/.gitconfig", []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), ".gitconfig")
	if err := os.Symlink(dst, link); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Write("files/git/.gitconfig", []byte("new")); err != nil {
		t.Fatal(err)
	}
	assertFile(t, link, "new")
}

func TestPathRejectsEscape(t *testing.T) {
	c := New(t.TempDir())
	for _, src := range []string{"../outside", "files/../../outside", ".", ""} {
		if _, err := c.Path(src); err == nil {
			t.Errorf("Path(%q) = nil error, want error", src)
		}
		if _, err := c.Write(src, []byte("x")); err == nil {
			t.Errorf("Write(%q) = nil error, want error", src)
		}
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}
