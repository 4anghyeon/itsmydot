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

// backupSuffix 는 백업 파일 이름 끝에 붙인다. ~/.zshrc의 백업은 ~/.zshrc.itsmydot.bak 이다.
// .bak만 붙이면 사용자나 다른 도구가 만든 .bak과 구분할 수 없어서 [Unlink] 가 itsmydot이 만들지 않은 파일을 되돌려 버릴 수 있다.
const backupSuffix = ".itsmydot.bak"

// timestampLayout 은 백업 이름이 이미 있을 때 뒤에 붙이는 시각 형식이다. 예를 들면 .itsmydot.bak.20261013-103015 가 된다.
const timestampLayout = "20060102-150405"

// now 는 테스트에서 시각을 고정할 수 있게 변수로 둔다.
var now = time.Now

// HasBackup 은 target의 itsmydot 백업이 있는지 알려 준다.
// [Unlink] 전에 되돌릴 원래 파일이 있는지 확인할 때 쓴다.
func HasBackup(target string) (bool, error) {
	latest, err := latestBackup(target)
	return latest != "", err
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
