// Package rcfile 은 ~/.zshrc 같은 셸 설정 파일 끝에 itsmydot 설정을 불러오는 한 줄을 넣고 뺀다.
//
// rc 파일은 Mac마다 다른 사용자 파일이라 통째로 바꾸지 않고 itsmydot이 넣은 줄만 다룬다.
// 줄 끝의 표시 주석까지 똑같은 줄만 itsmydot이 넣은 줄로 보고 사용자가 고친 줄은 건드리지 않는다.
package rcfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlink 는 rc 파일이 symlink일 때 [Add] 와 [Remove] 가 [fs.PathError] 로 감싸 돌려준다.
// 가리키는 곳이 다른 도구가 관리하는 파일일 수 있어서 따라가서 고치지 않는다.
var ErrSymlink = errors.New("is a symlink")

// marker 는 itsmydot이 넣은 줄 끝에 붙는 표시 주석이다.
const marker = "# itsmydot"

// Line 은 target 파일을 불러오는 한 줄을 만든다. target은 홈 디렉토리 아래면 ~로 줄여 쓴다.
// 파일이 없어도 셸이 에러 없이 뜨도록 읽을 수 있을 때만 불러온다.
func Line(target, home string) string {
	path := target
	if rest, ok := strings.CutPrefix(target, home+string(filepath.Separator)); ok && home != "" {
		path = "~/" + rest
	}
	return "[[ -r " + path + " ]] && source " + path + "   " + marker
}

// Has 는 rc 파일에 line이 있는지 알려 준다. rc 파일이 없으면 없다고 본다.
func Has(rc, line string) (bool, error) {
	data, err := os.ReadFile(rc)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", rc, err)
	}
	return containsLine(string(data), line), nil
}

// Add 는 rc 파일 맨 끝에 line을 넣는다. 이미 있으면 그대로 두고 false를 돌려준다.
// 맨 끝에 두어서 PATH나 alias가 겹치면 itsmydot 설정이 나중에 적용되게 한다.
// rc 파일이 없으면 line 한 줄짜리 파일을 만든다.
func Add(rc, line string) (added bool, err error) {
	data, mode, err := read(rc)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
			return false, fmt.Errorf("create dir for %s: %w", rc, err)
		}
		return true, write(rc, []byte(line+"\n"), 0o644)
	}
	if err != nil {
		return false, err
	}

	content := string(data)
	if containsLine(content, line) {
		return false, nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return true, write(rc, []byte(content+line+"\n"), mode)
}

// Remove 는 rc 파일에서 line과 똑같은 줄을 모두 지운다. 지운 줄이 없으면 파일을 다시 쓰지 않고 false를 돌려준다.
func Remove(rc, line string) (removed bool, err error) {
	data, mode, err := read(rc)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lines := strings.SplitAfter(string(data), "\n")
	kept := lines[:0]
	for _, l := range lines {
		if trimLine(l) == line {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return false, nil
	}
	return true, write(rc, []byte(strings.Join(kept, "")), mode)
}

// read 는 고치려는 rc 파일의 내용과 권한을 읽는다. symlink면 [ErrSymlink] 를 돌려준다.
func read(rc string) ([]byte, fs.FileMode, error) {
	info, err := os.Lstat(rc)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, err
		}
		return nil, 0, fmt.Errorf("check %s: %w", rc, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, 0, &fs.PathError{Op: "edit", Path: rc, Err: ErrSymlink}
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("edit %s: not a regular file", rc)
	}

	data, err := os.ReadFile(rc)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", rc, err)
	}
	return data, info.Mode().Perm(), nil
}

// write 는 rc 파일을 data로 바꾼다.
// 새 셸이 반쯤 쓰인 rc 파일을 읽으면 안 되므로 같은 폴더의 임시 파일에 다 쓴 뒤 이름을 바꾼다.
func write(rc string, data []byte, mode fs.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(rc), "."+filepath.Base(rc)+".itsmydot-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", rc, err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write %s: %w", rc, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", rc, err)
	}
	if err := os.Chmod(tempName, mode); err != nil {
		return fmt.Errorf("write %s: %w", rc, err)
	}
	if err := os.Rename(tempName, rc); err != nil {
		return fmt.Errorf("write %s: %w", rc, err)
	}
	return nil
}

func containsLine(content, line string) bool {
	for l := range strings.SplitSeq(content, "\n") {
		if trimLine(l) == line {
			return true
		}
	}
	return false
}

// trimLine 은 줄 끝의 공백과 줄바꿈을 떼어 낸다. 에디터가 줄 끝 공백을 지우거나 CRLF로 저장해도 같은 줄로 본다.
func trimLine(l string) string {
	return strings.TrimRight(l, " \t\r\n")
}
