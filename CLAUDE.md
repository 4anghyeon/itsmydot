# CLAUDE.md

이 공개 저장소는 `itsmydot` TUI이면서 소유자의 dotfile 원본(`files/`, `manifest.yaml`)을 보관하는 곳이다.

## 설계 원칙

- 동기화는 저장소에서 컴퓨터로 한 방향으로만 한다. dotfile은 GitHub 웹에서 고치고 이 도구는 저장소에 쓰지 않는다.
- 받기는 사용자가 TUI에서 직접 고를 때만 한다. 백그라운드나 예약 동기화는 없다. 진입점은 TUI 하나이고 명령줄 하위 명령은 두지 않는다.

## 구조

- `internal/tui`에는 진입 메뉴만 둔다. 메뉴 화면은 화면마다 패키지 하나(`dotfiles`, `brewfiles`)이고 `List`, `New`, 메뉴 카드에 쓰는 `Summary`를 공개한다. agentfiles 화면은 `dotfiles` 목록을 manifest의 `agentfiles` 섹션으로 하나 더 띄운 것이다.
- 두 화면이 같이 쓰는 코드는 화면 패키지가 아니라 `internal/tui/ui`에 둔다.
- `internal/tui` 밖의 패키지(`link`, `manifest`, `github` 등)는 화면을 모른다.

## TUI 규칙

- 키는 `ui.NormalizeKey()`로만 읽는다. macOS 한글 입력기는 자모 하나를 다음 키가 올 때까지 붙잡아 두므로 중요한 동작은 입력기가 붙잡지 않는 키(숫자, esc, 방향키, space, enter)를 쓴다. 확인 질문은 y/n이 아니라 번호로 고른다.
- 화면 글자의 기호는 ASCII로 쓴다("방향키"라고 쓰고 ↑↓는 쓰지 않는다). ✓ ↑ · 나 점자가 없는 폰트는 CJK 폰트로 대신 그리는데 이때 두 칸을 차지해 정렬이 깨진다. 상자 선(`╭─│`)과 블록(`█░`) 문자는 써도 된다.
- 화면은 전체 화면 모드가 아니라 인라인으로 그리므로 터미널보다 높아지면 위쪽 줄이 잘린다. 긴 목록은 터미널 높이에 맞춰 잘라 그리고 스크롤한다.

## 확인

- 실제 `$HOME`은 건드리지 않는다. 단위 테스트는 `t.TempDir()`와 가짜 저장소인 `githubtest.NewClient`를 쓴다. 실제 바이너리는 `HOME=<임시 폴더>`로 pty에서 띄운다. 실제 색을 보려면 `COLORTERM=truecolor`를 더한다.
- 실제 바이너리는 인증 없는 GitHub API를 쓰므로 시간당 60번까지만 요청할 수 있다. pty로 여러 번 띄우면 이 한도에 걸릴 수 있다.
- 끝내기 전에 `.github/workflows/ci.yml`의 검사를 돌린다.
