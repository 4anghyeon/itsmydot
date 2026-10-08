// Package localcopy 는 GitHub에서 받아 온 dotfile 원본의 로컬 사본을 ~/.itsmydot에 저장한다.
// 리포의 source 경로를 그대로 미러링한다.
//
//	리포:  files/zsh/.zshrc
//	사본:  ~/.itsmydot/files/zsh/.zshrc
//
// 전체 흐름:
//
//	Write(source, 데이터)
//	  ├─ 1. Path(source)로 저장 위치 계산 (루트 밖이면 거부)
//	  ├─ 2. 중간 디렉토리 생성 (mkdir -p)
//	  ├─ 3. 같은 디렉토리에 임시 파일을 만들어 데이터를 씀
//	  └─ 4. 임시 파일을 최종 경로로 rename
package localcopy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName 은 홈 디렉토리 아래에 만드는 로컬 사본 폴더 이름이다.
const DirName = ".itsmydot"

// Dir 은 로컬 사본을 보관하는 루트 디렉토리 하나를 관리한다.
type Dir struct {
	Root string
}

// New 는 root를 루트로 쓰는 Dir을 만든다.
func New(root string) *Dir {
	return &Dir{Root: root}
}

// Default 는 ~/.itsmydot을 루트로 쓰는 Dir을 만든다.
// 디렉토리는 여기서 만들지 않고, 처음 Write할 때 필요한 만큼 만든다.
func Default() (*Dir, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return New(filepath.Join(home, DirName)), nil
}

// Path 는 리포 기준 source 경로(예: "files/zsh/.zshrc")에 해당하는 사본 파일의 절대 경로를 돌려준다.
func (d *Dir) Path(source string) (string, error) {
	p := filepath.Join(d.Root, filepath.FromSlash(source))

	rel, err := filepath.Rel(d.Root, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local copy path %q escapes %s", source, d.Root)
	}
	return p, nil
}

// Write 는 data를 source에 해당하는 사본 파일에 저장하고, 저장한 절대 경로를 돌려준다.
// 이미 파일이 있으면 새 내용으로 바꾼다.
//
// 예: Write("files/zsh/.zshrc", 내용)
//
//	→ ~/.itsmydot/files/zsh/.zshrc 에 저장
//	→ "/Users/<사용자>/.itsmydot/files/zsh/.zshrc" 반환
//
// 파일에 바로 덮어쓰지 않고 "임시 파일에 다 쓴 다음 이름을 바꾸는" 방식을 쓴다.
// 그래서 중간에 실패해도 기존 파일은 이전 내용 그대로 남는다.
func (d *Dir) Write(source string, data []byte) (string, error) {
	// 1. 어디에 저장할지 계산한다.
	//    "files/zsh/.zshrc" → "~/.itsmydot/files/zsh/.zshrc"
	dst, err := d.Path(source)
	if err != nil {
		return "", err
	}

	// 2. 파일을 담을 폴더를 만든다.
	//    ~/.itsmydot, ~/.itsmydot/files, ~/.itsmydot/files/zsh 중 없는 것만 만든다.
	//    0o755 = 일반적인 폴더 권한 (나는 읽기/쓰기, 다른 사람은 읽기만)
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create local copy dir: %w", err)
	}

	// 3. 진짜 이름(.zshrc)이 아니라 임시 이름(.itsmydot-123456.tmp)으로 파일을 만든다.
	tmp, err := os.CreateTemp(dir, ".itsmydot-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// 함수가 끝날 때 임시 파일을 지운다(defer = 함수 끝에 실행 예약).
	// 중간 실페 -> 남은 임시 파일 정리
	// 성공 -> 임시 파일은 이미 .zshrc로 이름이 바뀌어 없으므로 아무 일도 일어나지 않는다.
	defer func() { _ = os.Remove(tmpName) }()

	// 3-1. 임시 파일에 내용을 쓰고 닫는다. 기존 .zshrc는 아직 이전 내용 그대로다.
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// 3-2. 권한을 일반 설정 파일과 같게 맞춘다.
	//      임시 파일은 "나만 읽기 가능(0o600)"으로 만들어지므로 "모두 읽기 가능(0o644)"으로 바꾼다.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	// 4. 임시 파일 이름을 진짜 이름(.zshrc)으로 바꾼다.
	//    기존 파일이 있으면 새 파일로 한 번에 바뀐다.
	if err := os.Rename(tmpName, dst); err != nil {
		return "", fmt.Errorf("move into local copy: %w", err)
	}
	return dst, nil
}
