# mise로 설치한 go도 찾을 수 있도록 mise.zsh 다음에 불러온다.
command -v go >/dev/null && export PATH="$PATH:$(go env GOPATH)/bin"
