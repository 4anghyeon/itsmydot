// Package link 는 ~/.zshrc 같은 $HOME의 target에 로컬 사본을 가리키는 symlink를 걸고 뗀다.
// 상태 파일을 따로 두지 않고 target 자리에 무엇이 있는지만 보고 상태를 판단한다.
//
// target 자리에 다른 파일이 있으면 [Link] 가 먼저 백업 이름으로 옮겨 두고,
// [Unlink] 는 symlink를 뗀 뒤 가장 최근 백업을 원래 자리로 되돌린다.
// [Detach] 는 symlink를 같은 내용의 일반 파일로 바꿔 연결만 끊는다.
package link

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// State 는 target 자리의 현재 상태다.
type State int

const (
	// NotSynced 는 target 자리에 아무것도 없는 상태다.
	NotSynced State = iota
	// Synced 는 target이 로컬 사본을 가리키는 symlink인 상태다.
	Synced
	// Conflict 는 target 자리에 일반 파일이나 폴더, 다른 곳을 가리키는 symlink가 있는 상태다.
	Conflict
)

// String 은 에러 메시지에 넣을 상태 이름을 돌려준다.
func (s State) String() string {
	switch s {
	case NotSynced:
		return "notSynced"
	case Synced:
		return "synced"
	case Conflict:
		return "conflict"
	default:
		return "unknown"
	}
}

// Check 는 target 자리를 보고 상태를 판단한다. 디스크는 바꾸지 않는다.
// copyPath는 symlink가 가리켜야 할 로컬 사본의 절대 경로다.
func Check(target, copyPath string) (State, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return NotSynced, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check %s: %w", target, err)
	}

	if info.Mode()&fs.ModeSymlink == 0 {
		return Conflict, nil
	}

	destination, err := os.Readlink(target)
	if err != nil {
		return 0, fmt.Errorf("check %s: %w", target, err)
	}
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(filepath.Dir(target), destination)
	}
	if filepath.Clean(destination) == filepath.Clean(copyPath) {
		return Synced, nil
	}
	return Conflict, nil
}

// Link 는 target에 copyPath를 가리키는 symlink를 건다.
// target 자리에 다른 것이 있으면 먼저 백업하고 그 백업 경로를 돌려준다. 백업하지 않았으면 빈 문자열이다.
func Link(target, copyPath string) (backup string, err error) {
	state, err := Check(target, copyPath)
	if err != nil {
		return "", err
	}
	if state == Synced {
		return "", nil
	}

	// 백업은 건너뛸 수 없다. 사용자에게는 진행할지만 묻는다.
	if state == Conflict {
		backup, err = nextBackupPath(target)
		if err != nil {
			return "", err
		}
		if err := os.Rename(target, backup); err != nil {
			return "", fmt.Errorf("back up %s: %w", target, err)
		}
	}

	// 새 Mac에는 ~/.claude 같은 폴더가 아직 없을 수 있다.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return backup, fmt.Errorf("create dir for %s: %w", target, err)
	}

	if err := os.Symlink(copyPath, target); err != nil {
		return backup, fmt.Errorf("link %s: %w", target, err)
	}
	return backup, nil
}

// Unlink 는 target에 걸린 symlink를 떼고 백업이 있으면 가장 최근 것을 원래 자리로 되돌린다.
// 되돌린 백업의 경로를 돌려주고 되돌린 게 없으면 빈 문자열을 돌려준다.
// target이 itsmydot이 건 symlink가 아니면 아무것도 지우지 않고 에러를 돌려준다.
func Unlink(target, copyPath string) (restored string, err error) {
	state, err := Check(target, copyPath)
	if err != nil {
		return "", err
	}
	if state != Synced {
		return "", fmt.Errorf("unlink %s: not linked by itsmydot (state %s)", target, state)
	}

	// symlink만 지워지고 가리키던 로컬 사본은 그대로 남는다.
	if err := os.Remove(target); err != nil {
		return "", fmt.Errorf("unlink %s: %w", target, err)
	}

	latest, err := latestBackup(target)
	if err != nil || latest == "" {
		return "", err
	}
	if err := os.Rename(latest, target); err != nil {
		return "", fmt.Errorf("restore %s: %w", latest, err)
	}
	return latest, nil
}

// Detach 는 target에 걸린 symlink를 로컬 사본과 같은 내용의 일반 파일로 바꾼다.
// 연결은 끊되 지금 설정은 그대로 남기고 싶을 때 쓴다. 백업은 건드리지 않는다.
// target이 itsmydot이 건 symlink가 아니면 아무것도 바꾸지 않고 에러를 돌려준다.
func Detach(target, copyPath string) error {
	state, err := Check(target, copyPath)
	if err != nil {
		return err
	}
	if state != Synced {
		return fmt.Errorf("detach %s: not linked by itsmydot (state %s)", target, state)
	}

	data, err := os.ReadFile(copyPath)
	if err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}
	info, err := os.Stat(copyPath)
	if err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}

	// target에 바로 쓰면 symlink를 따라가서 로컬 사본이 바뀐다.
	// 같은 폴더의 임시 파일에 쓴 뒤 이름을 바꿔서 symlink 자리를 통째로 교체한다.
	temp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".itsmydot-*.tmp")
	if err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("detach %s: %w", target, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}
	if err := os.Chmod(tempName, info.Mode().Perm()); err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}

	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("detach %s: %w", target, err)
	}
	return nil
}
