# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

`itsmydot` is a Go + bubbletea TUI for personal dotfiles management. It keeps dotfile originals in this
public GitHub repo and, when run, shows a checklist TUI where toggling an item creates/removes a symlink
from `$HOME` into a local copy of that file. Full design doc: `.claude/PLAN.md`.

## Implementation order

Build the project by following the step-by-step order in `.claude/PLAN.md` section 7 (Step 0 project
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
GitHub Contents API → base64 decode → ~/.itsmydot/files/... (local copy, mirrors repo's source path) → symlink → $HOME target
```

**There is no separate state store.** A target counts as `synced` purely by checking the filesystem: it's a
symlink and it resolves to the matching path under `~/.itsmydot`. This is a deliberate choice so the record
and reality can never drift apart.

**Package layout** (one responsibility per package; `internal/tui` is the composition root that wires the rest together):

- `internal/manifest` — parses `manifest.yaml` into `Entry{name, source, target, description}`, expands `~` in `target`
- `internal/github` — calls the Contents API, decodes base64 content, handles 404/rate-limit errors
- `internal/localcopy` — owns `~/.itsmydot`, mirrors the repo's `source` path structure so no path-translation logic is needed elsewhere. Not a disposable cache: these files are the live symlink targets, so deleting them breaks `$HOME` dotfiles
- `internal/link` — symlink state detection (`Lstat`/`Readlink`), create/remove, conflict detection, mandatory backup (`<target>.itsmydot.bak`, timestamped on collision). Unlink restores the most recent itsmydot backup, so toggling off returns the target to its pre-toggle state
- `internal/tui` — bubbletea `Model`/`Update`/`View` (Elm architecture); async work (API calls) is wrapped in `tea.Cmd` and results come back into `Update()` as `tea.Msg` (e.g. `entrySyncedMsg`)
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
- TUI shortcuts must also work while a Korean IME is active: always read keys through `normalizeKey()` in `internal/tui/keys.go` (never `msg.String()` directly). It maps the full 두벌식 layout to QWERTY, so new letter shortcuts need no extra registration. This is internal only — help text shows the English keys. Caveat: the macOS Korean IME holds a lone jamo until the next keystroke, so letter keys arrive late; important actions should use keys the IME never holds (digits, esc, arrows, space, enter) — that's why the conflict prompt uses `1`/`2` (Claude Code/Codex style) instead of `y`/`n`. (Switching the macOS input source instead was rejected: needs cgo/external tools, changes system-wide state, and CJK switching is unreliable.)
- Toggling off with no itsmydot backup to restore asks first (keep current content as a regular file via `link.Detach` / delete the file / cancel). Without a state store we can't tell "target never existed" from "backup was lost", so the user decides.
- TUI glyphs must render the same in any font: use ASCII for status markers, spinner frames and help text (`[+]`, `[!]`, `[-]`, `[.]`/`[o]`/`[O]` spinner, "방향키"), not ✓/✗/↑↓/·/braille — fonts lacking those fall back to a CJK font that draws them two cells wide and breaks alignment. Box-drawing (`╭─│`) and block (`█░`) characters are fine. Colors come from the `palette` in `internal/tui/styles.go` (dark/light chosen via `tea.BackgroundColorMsg`), not the terminal's ANSI 16 colors, whose "green" can look olive in some themes.
- Backups use the `.itsmydot.bak` suffix (not plain `.bak`) so unlink never restores a `.bak` file that some other tool or the user created.
- Not yet implemented, intentionally deferred: remote-update detection (comparing Contents API `sha` against the local copy), `ITSMYDOT_TOKEN` support for private repos/higher rate limits, non-interactive subcommands, search/diff modes.

**Libraries:** `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`, `github.com/charmbracelet/harmonica`, `gopkg.in/yaml.v3`. Use the v2 `charm.land/...` import paths, not the v1 `github.com/charmbracelet/...` ones — v2's `View()` returns `tea.View` (via `tea.NewView`) and keys arrive as `tea.KeyPressMsg`.
