package link

import (
	"os"
	"path/filepath"
	"testing"
)

// setup 은 임시 폴더 안에 가짜 홈(home)과 로컬 사본 파일(copyPath)을 만든다.
// 실제 $HOME은 건드리지 않는다.
func setup(t *testing.T) (home, copyPath string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	copyPath = filepath.Join(root, "itsmydot", "files", "zsh", ".zshrc")
	mustWrite(t, copyPath, "from repo")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return home, copyPath
}

func TestCheck(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".zshrc")

	assertState(t, target, copyPath, NotSynced)

	if err := os.Symlink(copyPath, target); err != nil {
		t.Fatal(err)
	}
	assertState(t, target, copyPath, Synced)

	// 상대 경로 symlink도 같은 파일을 가리키면 Synced.
	relative, err := filepath.Rel(home, copyPath)
	if err != nil {
		t.Fatal(err)
	}
	relativeTarget := filepath.Join(home, ".zshrc-rel")
	if err := os.Symlink(relative, relativeTarget); err != nil {
		t.Fatal(err)
	}
	assertState(t, relativeTarget, copyPath, Synced)

	// 다른 곳을 가리키는 symlink는 Conflict.
	other := filepath.Join(home, ".other")
	if err := os.Symlink("/somewhere/else", other); err != nil {
		t.Fatal(err)
	}
	assertState(t, other, copyPath, Conflict)

	// 일반 파일, 폴더도 Conflict.
	file := filepath.Join(home, ".gitconfig")
	mustWrite(t, file, "mine")
	assertState(t, file, copyPath, Conflict)
	assertState(t, home, copyPath, Conflict)
}

func TestLinkAndUnlinkEmptyTarget(t *testing.T) {
	home, copyPath := setup(t)
	// 새 Mac에서 ~/.claude/CLAUDE.md를 처음 연결할 때처럼 폴더가 아직 없는 경우
	target := filepath.Join(home, ".claude", "CLAUDE.md")

	backup, err := Link(target, copyPath)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if backup != "" {
		t.Errorf("backup = %q, want none", backup)
	}
	assertState(t, target, copyPath, Synced)
	assertContent(t, target, "from repo")

	// 이미 연결돼 있으면 다시 Link해도 아무 일도 없다.
	if _, err := Link(target, copyPath); err != nil {
		t.Fatalf("second Link: %v", err)
	}

	restored, err := Unlink(target, copyPath)
	if err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if restored != "" {
		t.Errorf("restored = %q, want none", restored)
	}
	assertState(t, target, copyPath, NotSynced)
	// symlink만 지워지고 로컬 사본은 남아 있어야 한다.
	assertContent(t, copyPath, "from repo")
}

// 일반 파일이 있던 자리에 Link하면 .itsmydot.bak으로 백업하고 Unlink하면 원래 파일을 되돌린다.
func TestLinkBacksUpAndUnlinkRestores(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".gitconfig")
	mustWrite(t, target, "original")

	backup, err := Link(target, copyPath)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if want := target + ".itsmydot.bak"; backup != want {
		t.Errorf("backup = %q, want %q", backup, want)
	}
	assertContent(t, backup, "original")
	assertState(t, target, copyPath, Synced)

	restored, err := Unlink(target, copyPath)
	if err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if restored != backup {
		t.Errorf("restored = %q, want %q", restored, backup)
	}
	assertContent(t, target, "original")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Errorf("backup %s should be gone after restore", backup)
	}
}

// 우리가 건 symlink가 아니면 Unlink는 아무것도 지우지 않는다.
func TestUnlinkRefusesForeignFile(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".zshrc")
	mustWrite(t, target, "user file")

	if _, err := Unlink(target, copyPath); err == nil {
		t.Fatal("Unlink on a regular file should fail")
	}
	assertContent(t, target, "user file")

	if _, err := Unlink(filepath.Join(home, ".missing"), copyPath); err == nil {
		t.Fatal("Unlink on a missing target should fail")
	}
}

func assertState(t *testing.T, target, copyPath string, want State) {
	t.Helper()
	got, err := Check(target, copyPath)
	if err != nil {
		t.Fatalf("Check(%s): %v", target, err)
	}
	if got != want {
		t.Errorf("Check(%s) = %s, want %s", target, got, want)
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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 사용자나 다른 도구가 만든 .bak은 itsmydot의 백업이 아니므로 되돌리지 않는다.
func TestUnlinkIgnoresForeignBak(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".zshrc")
	mustWrite(t, target+".bak", "made by oh-my-zsh")

	if _, err := Link(target, copyPath); err != nil {
		t.Fatal(err)
	}
	restored, err := Unlink(target, copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if restored != "" {
		t.Errorf("restored = %q, want none", restored)
	}
	assertState(t, target, copyPath, NotSynced)
	assertContent(t, target+".bak", "made by oh-my-zsh")
}

// Detach하면 symlink가 같은 내용의 일반 파일로 바뀌고, 로컬 사본과 백업은 그대로 남는다.
func TestDetach(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".zshrc")
	if _, err := Link(target, copyPath); err != nil {
		t.Fatal(err)
	}

	if err := Detach(target, copyPath); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Errorf("target mode = %v, want regular file", info.Mode())
	}
	assertContent(t, target, "from repo")
	assertContent(t, copyPath, "from repo")

	// 일반 파일이 된 target을 고쳐도 로컬 사본은 바뀌지 않아야 한다.
	mustWrite(t, target, "edited")
	assertContent(t, copyPath, "from repo")

	// 우리가 건 symlink가 아니면 거부한다.
	if err := Detach(target, copyPath); err == nil {
		t.Error("Detach on a regular file should fail")
	}
	assertContent(t, target, "edited")
}
