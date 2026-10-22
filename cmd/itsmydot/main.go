package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/tui"
)

// dotfile 원본이 있는 GitHub 저장소
const (
	repoOwner = "4anghyeon"
	repoName  = "itsmydot"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	copies, err := localcopy.Default()
	if err != nil {
		return err
	}
	client := github.NewClient(repoOwner, repoName)

	_, err = tea.NewProgram(tui.New(client, copies, version())).Run()
	return err
}

// releaseTag 는 v1.2.3 꼴의 릴리스 태그다.
var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// version 은 화면에 보여 줄 버전이다. go install 로 태그를 받았으면 그 태그이고 아니면 dev다.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && releaseTag.MatchString(info.Main.Version) {
		return info.Main.Version
	}
	return "dev"
}
