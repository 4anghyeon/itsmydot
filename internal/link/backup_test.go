package link

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// fixNow 는 테스트 동안 현재 시각을 고정하고 끝나면 원래대로 돌려놓는다.
func fixNow(t *testing.T, at time.Time) {
	t.Helper()
	orig := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = orig })
}

// 백업이 이미 있으면 시각을 붙이고 같은 초에 또 하면 번호를 붙인다.
// 되돌릴 때는 가장 최근 백업부터 되돌린다.
func TestBackupNamingAndOrder(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".zshrc")
	fixNow(t, time.Date(2026, 10, 13, 10, 30, 15, 0, time.Local))

	// itsmydot이 예전에 만든 백업이 이미 남아 있다.
	mustWrite(t, target+".itsmydot.bak", "old backup")

	var backups []string
	for i := 1; i <= 11; i++ {
		mustWrite(t, target, "version "+strconv.Itoa(i))
		b, err := Link(target, copyPath)
		if err != nil {
			t.Fatalf("Link #%d: %v", i, err)
		}
		backups = append(backups, b)
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
	}

	if want := target + ".itsmydot.bak.20261013-103015"; backups[0] != want {
		t.Errorf("first backup = %q, want %q", backups[0], want)
	}
	if want := target + ".itsmydot.bak.20261013-103015-2"; backups[1] != want {
		t.Errorf("second backup = %q, want %q", backups[1], want)
	}

	// 11번째 백업이 가장 최근이어야 한다. -11이 -2보다 뒤로 정렬되는지 본다.
	if err := os.Symlink(copyPath, target); err != nil {
		t.Fatal(err)
	}
	restored, err := Unlink(target, copyPath)
	if err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if restored != backups[10] {
		t.Errorf("restored = %q, want %q", restored, backups[10])
	}
	assertContent(t, target, "version 11")
}

func TestBackupLess(t *testing.T) {
	ordered := []string{
		".zshrc.itsmydot.bak.20261012-235959",
		".zshrc.itsmydot.bak.20261013-103015",
		".zshrc.itsmydot.bak.20261013-103015-2",
		".zshrc.itsmydot.bak.20261013-103015-10",
		".zshrc.itsmydot.bak.20261013-103016",
	}
	for i := 0; i < len(ordered)-1; i++ {
		if !backupLess(ordered[i], ordered[i+1]) {
			t.Errorf("want %q < %q", ordered[i], ordered[i+1])
		}
		if backupLess(ordered[i+1], ordered[i]) {
			t.Errorf("want not %q < %q", ordered[i+1], ordered[i])
		}
	}
}

func TestHasBackup(t *testing.T) {
	home, copyPath := setup(t)
	target := filepath.Join(home, ".gitconfig")

	if has, err := HasBackup(target); err != nil || has {
		t.Fatalf("HasBackup = %v, %v; want false", has, err)
	}

	mustWrite(t, target, "mine")
	if _, err := Link(target, copyPath); err != nil {
		t.Fatal(err)
	}
	if has, err := HasBackup(target); err != nil || !has {
		t.Fatalf("HasBackup = %v, %v; want true", has, err)
	}

	// 다른 도구가 만든 .bak은 itsmydot 백업으로 보지 않는다.
	other := filepath.Join(home, ".zshrc")
	mustWrite(t, other+".bak", "x")
	if has, _ := HasBackup(other); has {
		t.Error("plain .bak should not count as an itsmydot backup")
	}
}
