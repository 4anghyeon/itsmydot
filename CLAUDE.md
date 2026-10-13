# CLAUDE.md

`itsmydot` is a personal dotfiles TUI. Dotfile originals live in this public repo (`files/`, listed in
`manifest.yaml`); toggling an item downloads the file through the unauthenticated GitHub API into
`~/.itsmydot/` and symlinks the `$HOME` target to it. Work plan: `.claude/PLAN.md`.

## Design rules

- Sync is one-way (repo → machine). Dotfiles are edited on GitHub's web UI; this tool never writes back to the repo.
- No state store. Status is derived only from the filesystem (is the target a symlink to its local copy?), so record and reality can't drift.
- `~/.itsmydot` is not a cache: its files are the live symlink targets, and deleting them breaks `$HOME` dotfiles.
- Fetching happens only on explicit user action in the TUI — no background/scheduled sync. Single TUI entry point, no argv subcommands.
- A file already at the target is always backed up before linking (`<target>.itsmydot.bak`, timestamped on collision); the prompt only asks whether to proceed. The `.itsmydot.bak` suffix keeps unlink from restoring `.bak` files made by other tools.
- Unlink restores the newest itsmydot backup. With no backup it asks (keep current content / delete / cancel), because "target never existed" and "backup was lost" look the same on disk.

## TUI rules

- Read keys only through `normalizeKey()` (maps 두벌식 jamo to QWERTY). The macOS Korean IME holds a lone jamo until the next keystroke, so important actions use keys it never holds (digits, esc, arrows, space, enter) — prompts are numbered, not y/n.
- Status markers, spinner frames and help text use ASCII only (`[+]` `[!]` `[-]` `[.]`, "방향키"). Fonts lacking ✓ ↑ · or braille fall back to a CJK font that draws them two cells wide and breaks alignment. Box-drawing (`╭─│`) and block (`█░`) characters are fine.
- Colors come from `palette` in `internal/tui/styles.go` (dark/light picked via `tea.BackgroundColorMsg`), never the terminal's ANSI 16 colors — some themes render "green" as olive.

## Conventions

- Code comments are Korean and explain this project's domain or *why*, not Go/library basics. Doc comments put a space before the particle (`// Load 는 ...`), otherwise GoLand flags them.
- Spell names out in full (`animation`, not `anim`).
- No progress/status notes in code comments or this file.

## Verifying

- Never touch the real `$HOME`. Unit tests use `t.TempDir()`; to drive the real binary, run it in a pty with `HOME=<temp dir>` (add `COLORTERM=truecolor` to see the real colors).
- Before finishing: `gofmt -l .` (must print nothing), `go vet ./...`, `go test ./...`.
