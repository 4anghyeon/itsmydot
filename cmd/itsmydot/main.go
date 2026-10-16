package main

import (
	"fmt"
	"os"

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

	_, err = tea.NewProgram(tui.New(client, copies)).Run()
	return err
}
