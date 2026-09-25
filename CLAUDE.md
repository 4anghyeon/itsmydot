# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`itsmydot` is a Go + bubbletea TUI for personal dotfiles management. It keeps dotfile originals in this
public GitHub repo and, when run, shows a checklist TUI where toggling an item creates/removes a symlink
from `$HOME` into a local cache of that file. Full design doc: `.claude/plans/PLAN.md`.

## Current status

Implementation has not started yet — the repo only contains the GoLand-scaffolded `main.go` at the root.
Build the project by following the step-by-step order in `.claude/plans/PLAN.md` section 7 (Step 0 project
init through Step 10 install flow); don't skip ahead, each step is meant to leave the tool in a working state.

## Development commands

- Run: `go run ./cmd/itsmydot` (once the entry point moves there per the planned layout below)
- Build: `go build ./...`
- Test all: `go test ./...`
- Test one: `go test ./internal/<pkg> -run TestName`
- Vet: `go vet ./...`
- Format: `gofmt -w .` (gofmt is non-configurable — there is no style debate to have)
- Install as a CLI: `go install github.com/<account>/itsmydot/cmd/itsmydot@latest` (requires `~/go/bin` on `PATH`)

## Architecture

**Core data flow** (no git binary, no auth token — public repo, unauthenticated GitHub API):

```
GitHub Contents API → base64 decode → ~/.itsmydot/files/... (cache, mirrors repo's source path) → symlink → $HOME target
```

**There is no separate state store.** A target counts as `synced` purely by checking the filesystem: it's a
symlink and it resolves to the matching path under `~/.itsmydot`. This is a deliberate choice so the record
and reality can never drift apart.

**Package layout** (one responsibility per package; `internal/tui` is the composition root that wires the rest together):

- `internal/manifest` — parses `manifest.yaml` into `Entry{name, source, target, description}`, expands `~` in `target`
- `internal/github` — calls the Contents API, decodes base64 content, handles 404/rate-limit errors
- `internal/cache` — owns `~/.itsmydot`, mirrors the repo's `source` path structure so no path-translation logic is needed elsewhere
- `internal/link` — symlink state detection (`Lstat`/`Readlink`), create/remove, conflict detection, mandatory backup (`<target>.bak`, timestamped on collision)
- `internal/tui` — bubbletea `Model`/`Update`/`View` (Elm architecture); async work (API calls) is wrapped in `tea.Cmd` and results come back into `Update()` as `tea.Msg` (e.g. `entryToggledMsg`)
- `cmd/itsmydot/main.go` — entry point; just calls `tea.NewProgram()`

**State model** — an entry is one of: `notSynced` (no symlink) / `synced` / `syncing` (API call in flight,
shown with a spinner) / `conflict` (a real file already sits at `target`) / `errored`.

**manifest.yaml schema:**
```yaml
entries:
  - name: claude-memory
    source: files/claude/CLAUDE.md   # repo-root-relative
    target: ~/.claude/CLAUDE.md      # ~ expands to $HOME
    description: Claude Code personal style guide
```

**Design decisions to preserve when extending this:**
- Sync is one-way (repo → machine). Editing dotfiles happens on GitHub's web UI, never through this tool; there is no push/write path back to the repo by design.
- Refresh happens on toggle, not on a schedule or separate `sync` command — re-toggling an entry re-fetches the latest version.
- Single entry point, no argv subcommands (no `itsmydot add <name>` style CLI is planned).
- Conflicts always get backed up before symlinking — this isn't optional/prompted away, only the confirm step is.
- Not yet implemented, intentionally deferred: remote-update detection (comparing Contents API `sha` against the local cache), `ITSMYDOT_TOKEN` support for private repos/higher rate limits, non-interactive subcommands, search/diff modes.

**Libraries:** `charmbracelet/bubbletea`, `charmbracelet/lipgloss`, `charmbracelet/bubbles`, `charmbracelet/harmonica`, `gopkg.in/yaml.v3`.
