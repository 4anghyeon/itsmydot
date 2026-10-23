package tui

import (
	"path/filepath"
	"strings"
)

// tildify 는 홈 디렉토리 아래 경로를 ~로 줄인다. /Users/me/.zshrc 는 ~/.zshrc 가 된다.
func tildify(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}
