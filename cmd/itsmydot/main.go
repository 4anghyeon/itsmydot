package main

import (
	"fmt"
	"os"

	"github.com/4anghyeon/itsmydot/internal/manifest"
)

func main() {
	m, err := manifest.Load("manifest.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, e := range m.Entries {
		fmt.Printf("%-12s %-22s -> %s  (%s)\n", e.Name, e.Source, e.Target, e.Description)
	}
}
