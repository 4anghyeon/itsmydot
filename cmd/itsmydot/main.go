package main

import (
	"context"
	"fmt"
	"os"

	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	client := github.NewClient("4anghyeon", "itsmydot")
	copies, err := localcopy.Default()
	if err != nil {
		return err
	}

	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return err
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return err
	}

	for _, e := range m.Entries {
		content, err := client.Fetch(ctx, e.Source)
		if err != nil {
			return err
		}
		saved, err := copies.Write(e.Source, content)
		if err != nil {
			return err
		}
		fmt.Printf("%-12s %-22s -> %s  (%d bytes)\n", e.Name, e.Source, saved, len(content))
	}
	return nil
}
