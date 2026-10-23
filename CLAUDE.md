# CLAUDE.md

This public repo is both the `itsmydot` TUI and the store of the owner's dotfile originals (`files/`, `manifest.yaml`).

## Design rules

- Sync is one-way (repo → machine). Dotfiles are edited on GitHub's web UI; this tool never writes back to the repo.
- Fetching happens only on explicit user action in the TUI — no background/scheduled sync. Single TUI entry point, no argv subcommands.

## TUI rules

- Read keys only through `normalizeKey()`. The macOS Korean IME holds a lone jamo until the next keystroke, so important actions use keys it never holds (digits, esc, arrows, space, enter) — prompts are numbered, not y/n.
- Symbols in UI text are ASCII ("방향키", not ↑↓). Fonts lacking ✓ ↑ · or braille fall back to a CJK font that draws them two cells wide and breaks alignment. Box-drawing (`╭─│`) and block (`█░`) characters are fine.

## Conventions

- Code comments are Korean and explain this project's domain or *why*, not Go/library basics. Doc comments put a space before the particle (`// Load 는 ...`), otherwise GoLand flags them.
  Doc links need the same space (`[Parse] 가`), otherwise go/doc does not treat them as links.
- Write comments as plain Korean sentences: no arrows, `·`, tree drawings, colon labels or parenthetical asides, and few commas.
- Spell names out in full (`animation`, not `anim`).
- No progress/status notes in code comments or this file.

## Verifying

- Never touch the real `$HOME`. Unit tests use `t.TempDir()`; to drive the real binary, run it in a pty with `HOME=<temp dir>` (add `COLORTERM=truecolor` to see the real colors).
- Before finishing, run the checks in `.github/workflows/ci.yml`.
