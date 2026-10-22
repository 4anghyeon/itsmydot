// Package localcopy 는 GitHub에서 받은 dotfile을 ~/.itsmydot에 저장한다.
// 저장소의 source 경로를 그대로 따르므로 files/zsh/.zshrc 는 ~/.itsmydot/files/zsh/.zshrc 에 놓인다.
//
// 이 파일들은 $HOME의 symlink가 가리키는 실제 파일이다. 지우면 연결된 dotfile이 깨진다.
package localcopy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName 은 홈 디렉토리 아래에 만드는 로컬 사본 폴더 이름이다.
const DirName = ".itsmydot"

// Dir 은 로컬 사본을 보관하는 폴더 하나다.
type Dir struct {
	Root string
}

// New 는 root를 루트로 쓰는 Dir을 만든다.
func New(root string) *Dir {
	return &Dir{Root: root}
}

// Default 는 ~/.itsmydot을 루트로 쓰는 Dir을 만든다.
// 폴더는 여기서 만들지 않고 처음 [Dir.Write] 를 할 때 필요한 만큼 만든다.
func Default() (*Dir, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return New(filepath.Join(home, DirName)), nil
}

// Path 는 저장소의 source 경로에 해당하는 사본 파일의 절대 경로를 돌려준다. Root 밖으로 나가면 에러다.
func (d *Dir) Path(source string) (string, error) {
	p := filepath.Join(d.Root, filepath.FromSlash(source))

	rel, err := filepath.Rel(d.Root, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local copy path %q escapes %s", source, d.Root)
	}
	return p, nil
}

// Write 는 data를 source에 해당하는 사본 파일에 저장하고 그 절대 경로를 돌려준다.
// 이미 파일이 있으면 새 내용으로 바꾼다.
//
// 사본은 $HOME의 symlink가 바로 가리키는 파일이라 반쯤 쓰인 채로 남으면 안 된다.
// 그래서 임시 파일에 다 쓴 다음 이름을 바꾼다. 중간에 실패해도 기존 파일은 이전 내용 그대로다.
func (d *Dir) Write(source string, data []byte) (string, error) {
	dst, err := d.Path(source)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create local copy dir: %w", err)
	}

	// 이름 바꾸기가 한 번에 끝나려면 임시 파일이 같은 폴더에 있어야 한다.
	tmp, err := os.CreateTemp(dir, ".itsmydot-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// 성공하면 이름이 바뀌어 지울 것이 없고 실패했을 때만 남은 임시 파일이 지워진다.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// 임시 파일은 0600으로 만들어지므로 일반 설정 파일처럼 0644로 맞춘다.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return "", fmt.Errorf("move into local copy: %w", err)
	}
	return dst, nil
}
