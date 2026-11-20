// Package homebrew 는 itsmydot이 부르는 brew 명령을 감싼다.
// 화면은 [Runner] 인터페이스만 보므로 테스트에서 실제 brew를 부르지 않고 가짜로 바꿀 수 있다.
package homebrew

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/4anghyeon/itsmydot/internal/brewfile"
)

// ErrNotInstalled 는 이 컴퓨터에서 brew 실행 파일을 찾지 못했을 때 [Find] 가 돌려준다.
var ErrNotInstalled = errors.New("homebrew is not installed")

// InstallGuide 는 Homebrew가 없을 때 안내하는 공식 설치 명령이다. itsmydot은 이 명령을 직접 실행하지 않는다.
const InstallGuide = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`

// Runner 는 화면이 쓰는 brew 명령 묶음이다.
type Runner interface {
	// Installed 는 지금 설치된 formula, cask, tap을 확인한다.
	Installed(ctx context.Context) (Installed, error)

	// InstallCommand 는 brewfile 경로의 Brewfile을 설치하는 명령을 만든다.
	// 화면은 이 명령에 터미널을 넘겨서 진행 상황과 sudo 비밀번호 입력을 brew가 직접 다루게 한다.
	InstallCommand(brewfile string) *exec.Cmd
}

// Installed 는 이 컴퓨터에 설치된 formula, cask, tap의 이름이다. 대소문자를 가리지 않도록 모두 소문자로 넣는다.
type Installed struct {
	Formulae, Casks, Taps map[string]bool
}

// Has 는 Brewfile 줄 p의 패키지가 설치돼 있는지 알려 준다.
// brew list는 tap 경로를 뗀 이름을 보여 주므로 formula와 cask는 그 이름으로 찾는다.
func (in Installed) Has(p brewfile.Package) bool {
	name := strings.ToLower(p.ShortName())
	switch p.Kind {
	case brewfile.Tap:
		return in.Taps[name]
	case brewfile.Formula:
		return in.Formulae[name]
	case brewfile.Cask:
		return in.Casks[name]
	}
	return false
}

// searchPaths 는 PATH에 brew가 없을 때 찾아볼 기본 설치 경로다. 앞은 Apple Silicon, 뒤는 Intel Mac의 경로다.
// 셸 설정에서 PATH를 잡기 전에 실행해도 Homebrew를 찾게 한다.
var searchPaths = []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"}

// Brew 는 실제 brew 실행 파일로 일하는 [Runner] 다.
type Brew struct {
	Path string
}

// Find 는 PATH와 기본 설치 경로에서 brew를 찾는다. 없으면 [ErrNotInstalled] 를 돌려준다.
func Find() (*Brew, error) {
	if path, err := exec.LookPath("brew"); err == nil {
		return &Brew{Path: path}, nil
	}
	for _, path := range searchPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return &Brew{Path: path}, nil
		}
	}
	return nil, ErrNotInstalled
}

// Installed 는 brew list와 brew tap으로 설치된 패키지를 확인한다.
func (b *Brew) Installed(ctx context.Context) (Installed, error) {
	var in Installed
	var err error
	if in.Formulae, err = b.names(ctx, "list", "--formula", "-1"); err != nil {
		return Installed{}, err
	}
	if in.Casks, err = b.names(ctx, "list", "--cask", "-1"); err != nil {
		return Installed{}, err
	}
	if in.Taps, err = b.names(ctx, "tap"); err != nil {
		return Installed{}, err
	}
	return in, nil
}

// InstallCommand 는 brew bundle로 brewfile을 설치하는 명령을 만든다.
func (b *Brew) InstallCommand(brewfile string) *exec.Cmd {
	return exec.Command(b.Path, "bundle", "--file", brewfile)
}

// names 는 brew 명령을 실행해 한 줄에 하나씩 나오는 이름을 소문자 집합으로 모은다.
// 목록만 보는 명령이라 brew가 스스로 업데이트하느라 느려지지 않게 자동 업데이트를 끈다.
func (b *Brew) names(ctx context.Context, args ...string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, b.Path, args...)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	names := make(map[string]bool)
	for line := range strings.SplitSeq(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names[strings.ToLower(name)] = true
		}
	}
	return names, nil
}
