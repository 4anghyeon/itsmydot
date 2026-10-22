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
	"sort"
	"strconv"
	"strings"
	"time"
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

// backupSuffix 는 백업 파일 이름 끝에 붙인다. ~/.zshrc의 백업은 ~/.zshrc.itsmydot.bak 이다.
// .bak만 붙이면 사용자나 다른 도구가 만든 .bak과 구분할 수 없어서
// [Unlink] 가 itsmydot이 만들지 않은 파일을 되돌려 버릴 수 있다.
const backupSuffix = ".itsmydot.bak"

// timestampLayout 은 백업 이름이 이미 있을 때 뒤에 붙이는 시각 형식이다. 예를 들면 .itsmydot.bak.20261013-103015 가 된다.
const timestampLayout = "20060102-150405"

// now 는 테스트에서 시각을 고정할 수 있게 변수로 둔다.
var now = time.Now

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

// HasBackup 은 target의 itsmydot 백업이 있는지 알려 준다.
// [Unlink] 전에 되돌릴 원래 파일이 있는지 확인할 때 쓴다.
func HasBackup(target string) (bool, error) {
	latest, err := latestBackup(target)
	return latest != "", err
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

// nextBackupPath 는 아직 쓰이지 않은 백업 경로를 고른다.
// 처음엔 .itsmydot.bak을 쓴다. 그 이름이 있으면 시각을 붙이고 같은 초에 또 백업하면 번호까지 붙인다.
//
//	~/.zshrc.itsmydot.bak
//	~/.zshrc.itsmydot.bak.20261013-103015
//	~/.zshrc.itsmydot.bak.20261013-103015-2
func nextBackupPath(target string) (string, error) {
	candidates := []string{target + backupSuffix}
	stamped := target + backupSuffix + "." + now().Format(timestampLayout)
	candidates = append(candidates, stamped)
	for i := 2; i <= 100; i++ {
		candidates = append(candidates, stamped+"-"+strconv.Itoa(i))
	}

	for _, c := range candidates {
		if _, err := os.Lstat(c); errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
	}
	return "", fmt.Errorf("back up %s: too many backups", target)
}

// latestBackup 은 target의 백업 중 가장 최근 것의 경로를 돌려준다. 없으면 빈 문자열이다.
//
// 백업 이름은 nextBackupPath가 정한 순서대로 생긴다. 그래서 시각이 붙은 백업이 있으면
// 그중 가장 나중 것이 최근이고 하나도 없을 때만 .itsmydot.bak이 최근이다.
func latestBackup(target string) (string, error) {
	dir, base := filepath.Split(target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("find backups of %s: %w", target, err)
	}

	plain := base + backupSuffix
	var stamped []string
	hasPlain := false
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == plain:
			hasPlain = true
		case strings.HasPrefix(name, plain+"."):
			stamped = append(stamped, name)
		}
	}

	if len(stamped) > 0 {
		sort.Slice(stamped, func(i, j int) bool { return backupLess(stamped[i], stamped[j]) })
		return filepath.Join(dir, stamped[len(stamped)-1]), nil
	}
	if hasPlain {
		return filepath.Join(dir, plain), nil
	}
	return "", nil
}

// backupLess 는 시각이 붙은 백업 이름 a가 b보다 먼저 만들어졌는지 알려 준다.
//
// 시각 부분은 글자 순서가 곧 시간 순서라서 문자열로 비교한다.
// 같은 초에 만든 백업은 뒤에 붙은 번호를 숫자로 비교한다. 글자로 비교하면 -10이 -2보다 앞에 오기 때문이다.
func backupLess(a, b string) bool {
	stampA, numberA := splitBackupName(a)
	stampB, numberB := splitBackupName(b)
	if stampA != stampB {
		return stampA < stampB
	}
	return numberA < numberB
}

// splitBackupName 은 백업 이름 끝에 붙은 -2 같은 번호를 떼어 나머지 이름과 번호를 돌려준다. 번호가 없으면 1이다.
func splitBackupName(name string) (string, int) {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return name, 1
	}
	n, err := strconv.Atoi(name[i+1:])
	// 시각의 -103015처럼 여섯 자리면 번호가 아니라 시각의 일부다.
	if err != nil || len(name[i+1:]) == len("150405") {
		return name, 1
	}
	return name[:i], n
}
