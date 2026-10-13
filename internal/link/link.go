// Package link 는 $HOME의 target(예: ~/.zshrc)에 로컬 사본을 가리키는 symlink를 걸고 뗀다.
// 별도의 상태 파일 없이, target 자리에 무엇이 있는지만 보고 상태를 판단한다.
//
//	~/.zshrc  ──symlink──→  ~/.itsmydot/files/zsh/.zshrc
//	(target)                 (copyPath: 로컬 사본)
//
// 전체 흐름:
//
//	Check(target, copyPath)   지금 상태 확인 (디스크 변경 없음)
//	Link(target, copyPath)    symlink 걸기
//	  ├─ 1. 상태 확인. 이미 연결돼 있으면 끝
//	  ├─ 2. target 자리에 다른 것이 있으면 백업 이름으로 옮김 (~/.zshrc → ~/.zshrc.itsmydot.bak)
//	  ├─ 3. target의 폴더가 없으면 만듦
//	  └─ 4. symlink 생성
//	Unlink(target, copyPath)  symlink 떼기
//	  ├─ 1. 우리가 건 symlink가 맞는지 확인. 아니면 아무것도 지우지 않고 에러
//	  ├─ 2. symlink 삭제
//	  └─ 3. 백업이 있으면 가장 최근 것을 원래 자리로 되돌림 (~/.zshrc.itsmydot.bak → ~/.zshrc)
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
	// NotSynced 는 target 자리에 아무것도 없는 상태다. [ ]
	NotSynced State = iota
	// Synced 는 target이 로컬 사본을 가리키는 symlink인 상태다. [x]
	Synced
	// Conflict 는 target 자리에 다른 것(일반 파일, 폴더, 다른 곳을 가리키는 symlink)이 있는 상태다. [!]
	Conflict
)

// String 은 상태 이름을 돌려준다.
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

// backupSuffix 는 백업 파일 이름 뒤에 붙는 표시다. ~/.zshrc → ~/.zshrc.itsmydot.bak
// 그냥 ".bak"으로 하면 사용자나 다른 도구가 만든 .bak과 구분할 수 없어서,
// Unlink할 때 itsmydot이 만들지 않은 파일을 되돌려 버릴 수 있다.
const backupSuffix = ".itsmydot.bak"

// timestampLayout 은 백업이 이미 있을 때 붙이는 시각 형식이다. ~/.zshrc.itsmydot.bak.20261013-103015
const timestampLayout = "20060102-150405"

// now 는 현재 시각을 돌려준다. 테스트에서 시각을 고정하려고 변수로 둔다.
var now = time.Now

// Check 는 target 자리를 보고 상태를 판단한다. 디스크는 바꾸지 않는다.
// copyPath는 symlink가 가리켜야 할 로컬 사본의 절대 경로다.
func Check(target, copyPath string) (State, error) {
	// 1. symlink를 따라가지 않고 target 자리 자체를 본다.
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return NotSynced, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check %s: %w", target, err)
	}

	// 2. symlink가 아니면(일반 파일, 폴더) 충돌.
	if info.Mode()&fs.ModeSymlink == 0 {
		return Conflict, nil
	}

	// 3. symlink면 어디를 가리키는지 읽어서 로컬 사본과 같은지 비교한다.
	dest, err := os.Readlink(target)
	if err != nil {
		return 0, fmt.Errorf("check %s: %w", target, err)
	}
	// symlink가 "../x"처럼 상대 경로로 되어 있으면, symlink가 있는 폴더 기준으로 절대 경로를 만든다.
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(target), dest)
	}
	if filepath.Clean(dest) == filepath.Clean(copyPath) {
		return Synced, nil
	}
	return Conflict, nil
}

// Link 는 target에 copyPath를 가리키는 symlink를 건다.
// target 자리에 다른 것이 있으면 먼저 백업하고, 백업한 경로를 돌려준다(백업이 없으면 "").
func Link(target, copyPath string) (backup string, err error) {
	// 1. 상태 확인. 이미 연결돼 있으면 할 일이 없다.
	state, err := Check(target, copyPath)
	if err != nil {
		return "", err
	}
	if state == Synced {
		return "", nil
	}

	// 2. 충돌이면 기존 것을 백업 이름으로 옮긴다. 백업은 선택이 아니라 항상 한다.
	if state == Conflict {
		backup, err = nextBackupPath(target)
		if err != nil {
			return "", err
		}
		if err := os.Rename(target, backup); err != nil {
			return "", fmt.Errorf("back up %s: %w", target, err)
		}
	}

	// 3. target이 들어갈 폴더가 없으면 만든다. 예: 새 Mac에서 ~/.claude가 없을 때
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return backup, fmt.Errorf("create dir for %s: %w", target, err)
	}

	// 4. symlink 생성
	if err := os.Symlink(copyPath, target); err != nil {
		return backup, fmt.Errorf("link %s: %w", target, err)
	}
	return backup, nil
}

// Unlink 는 target에 걸린 symlink를 떼고, 백업이 있으면 가장 최근 것을 원래 자리로 되돌린다.
// 되돌린 백업의 경로를 돌려준다(되돌린 게 없으면 "").
// target이 우리가 건 symlink가 아니면 아무것도 지우지 않고 에러를 돌려준다.
func Unlink(target, copyPath string) (restored string, err error) {
	// 1. 우리가 건 symlink인지 확인. 사용자의 실제 파일을 지우는 일이 없도록 막는다.
	state, err := Check(target, copyPath)
	if err != nil {
		return "", err
	}
	if state != Synced {
		return "", fmt.Errorf("unlink %s: not linked by itsmydot (state %s)", target, state)
	}

	// 2. symlink 삭제. symlink만 지워지고, 가리키던 로컬 사본은 그대로 남는다.
	if err := os.Remove(target); err != nil {
		return "", fmt.Errorf("unlink %s: %w", target, err)
	}

	// 3. 가장 최근 백업이 있으면 원래 이름으로 되돌린다.
	latest, err := latestBackup(target)
	if err != nil || latest == "" {
		return "", err
	}
	if err := os.Rename(latest, target); err != nil {
		return "", fmt.Errorf("restore %s: %w", latest, err)
	}
	return latest, nil
}

// nextBackupPath 는 아직 쓰이지 않은 백업 경로를 정한다.
//
//	~/.zshrc.itsmydot.bak                     ← 처음엔 이 이름
//	~/.zshrc.itsmydot.bak.20261013-103015     ← 위 이름이 이미 있으면 시각을 붙임
//	~/.zshrc.itsmydot.bak.20261013-103015-2   ← 같은 초에 또 백업하면 번호를 붙임
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

// latestBackup 은 target의 백업 중 가장 최근 것의 경로를 돌려준다. 없으면 "".
//
// 백업은 .itsmydot.bak → .itsmydot.bak.<시각> → .itsmydot.bak.<시각>-2 순서로 만들어지므로,
// 시각이 붙은 백업이 있으면 그중 가장 나중 것이, 없으면 .itsmydot.bak이 가장 최근이다.
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

// backupLess 는 시각이 붙은 백업 이름 a가 b보다 먼저 만들어졌으면 true를 돌려준다.
//
// 시각 부분(20261013-103015)은 글자 순서가 곧 시간 순서라서 문자열로 비교한다.
// 같은 초에 만든 백업은 뒤의 번호를 숫자로 비교한다.
// 글자로 비교하면 "-10"이 "-2"보다 앞에 오기 때문이다. 번호가 없는 것은 1번으로 본다.
func backupLess(a, b string) bool {
	stampA, nA := splitBackupName(a)
	stampB, nB := splitBackupName(b)
	if stampA != stampB {
		return stampA < stampB
	}
	return nA < nB
}

// splitBackupName 은 ".zshrc.itsmydot.bak.20261013-103015-2"를 (".zshrc.itsmydot.bak.20261013-103015", 2)로 나눈다.
func splitBackupName(name string) (string, int) {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return name, 1
	}
	n, err := strconv.Atoi(name[i+1:])
	// 시각의 "-103015"처럼 6자리면 번호가 아니라 시각의 일부다.
	if err != nil || len(name[i+1:]) == len("150405") {
		return name, 1
	}
	return name[:i], n
}
