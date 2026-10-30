<div align="center">

# itsmydot

**Manage dotfiles, simply.**

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![Bubble Tea](https://img.shields.io/badge/Bubble%20Tea-v2-FF5F87)](https://github.com/charmbracelet/bubbletea)
![Platform](https://img.shields.io/badge/platform-macOS-lightgrey?logo=apple)

<img src="docs/screenshot-prompt.svg" alt="itsmydot 실행 화면: 연결된 gitconfig와 충돌 중인 zshrc" width="560">

</div>

여러 개발 환경에서 dotfile을 일관되게 관리하는 CLI 도구입니다. Git 저장소에서 필요한 설정 파일만 골라 연결하고, 기존 파일은 백업해 필요할 때 복원할 수 있습니다.

## 기능

- 원하는 dotfile만 선택해 연결하거나 해제
- 기존 파일 백업 및 복원
- 저장소에 업데이트가 있는 항목 표시와 최신본 받기
- 로컬 사본과 저장소 최신본의 차이 보기
- 관련 파일을 그룹으로 묶어 접고 펼치기

## 설치

Go가 필요합니다. macOS에서는 Homebrew로 설치하거나 [go.dev/dl](https://go.dev/dl/)에서 설치 파일을 받을 수 있습니다.

```bash
brew install go
```

Go를 설치한 뒤 itsmydot을 설치합니다.

```bash
go install github.com/4anghyeon/itsmydot/cmd/itsmydot@latest
```

설치 후 `itsmydot` 명령을 찾을 수 없다면 Go의 바이너리 설치 경로가 `PATH`에 포함되어 있는지 확인하세요. 일반적인 기본 경로는 `$(go env GOPATH)/bin`입니다.

## 사용법

```bash
itsmydot
```

목록에서 dotfile을 고르고 <kbd>space</kbd>로 연결하거나 해제합니다.

| 키 | 동작 |
|---|---|
| <kbd>↑</kbd> <kbd>↓</kbd> | 항목 이동 |
| <kbd>←</kbd> <kbd>→</kbd> | 그룹 접기 / 펼치기 |
| <kbd>space</kbd> | 선택한 항목 연결 / 해제 |
| <kbd>enter</kbd> | 항목 메뉴 열기 (최신본 받기, 차이 보기) |
| <kbd>tab</kbd> | 파일 미리보기. 업데이트가 있으면 <kbd>enter</kbd>로 차이 보기 |
| <kbd>?</kbd> | 도움말 |
| <kbd>esc</kbd> | 화면 닫기 / 종료 |

| 표시 | 상태 |
|---|---|
| `[+]` | 연결됨 |
| `[ ]` | 연결되지 않음 |
| `[!]` | 대상 경로에 기존 파일이 있어 충돌함 |
| `[-]` | 작업 실패 |
| `[.]` `[o]` `[O]` | 동기화 진행 중 |
| `^` | 저장소에 업데이트 있음 |
| `v` `>` | 펼친 그룹 / 접힌 그룹 |

## 동작 방식

`manifest.yaml`에 정의된 파일을 저장소에서 `~/.itsmydot/`로 가져온 뒤, `target` 경로에 로컬 사본을 가리키는 심볼릭 링크를 만듭니다.

```mermaid
flowchart LR
    repo[("Git 저장소<br/>files/ · manifest.yaml")]
    copy["~/.itsmydot/files/..."]
    target["~/.zshrc · ~/.gitconfig"]

    repo -->|"가져오기"| copy
    target -.->|"심볼릭 링크"| copy
```

동기화 방향은 저장소에서 로컬 사본으로 향하는 단방향입니다. 설정을 수정할 때는 저장소의 원본 파일을 편집하세요. 로컬 사본이나 대상 경로에서 한 변경은 저장소에 자동으로 반영되지 않습니다.

## dotfile 추가

`files/` 아래에 파일을 두고 `manifest.yaml`에 항목을 추가합니다.

```yaml
entries:
  - name: gitconfig               # 목록에 보일 이름
    source: files/git/.gitconfig  # 저장소 안의 경로
    target: ~/.gitconfig          # 연결할 위치
    description: git 전역 설정     # 설명 (선택)
```

포크해서 사용할 때는 [`cmd/itsmydot/main.go`](cmd/itsmydot/main.go)의 `repoOwner`, `repoName`을 자신의 저장소에 맞게 바꿉니다.
