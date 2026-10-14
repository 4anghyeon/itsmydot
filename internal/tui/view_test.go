package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestView(t *testing.T) {
	loading := viewText(Model{})
	if !strings.Contains(loading, "불러오는 중") {
		t.Errorf("loading view = %q", loading)
	}

	failed := loadedModel()
	failed.loadErr = errors.New("boom")
	if v := viewText(failed); !strings.Contains(v, "boom") {
		t.Errorf("error view = %q", v)
	}

	m := loadedModel(
		item{entry: entry("gitconfig", "/home/me/.gitconfig"), state: synced},
		item{entry: entry("zshrc", "/home/me/.zshrc"), state: conflict},
		item{entry: entry("vimrc", "/home/me/.vimrc"), state: errored, err: errors.New("not found")},
		item{entry: entry("tmux", "/home/me/.tmux.conf"), state: syncing},
	)
	v := viewText(m)
	for _, want := range []string{
		"> [+] gitconfig  ~/.gitconfig",
		"  [!] zshrc      ~/.zshrc   (다른 파일이 있음)",
		"  [-] vimrc      ~/.vimrc   (not found)",
		"  [~] tmux       ~/.tmux.conf",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestTildify(t *testing.T) {
	tests := map[string]string{
		"/home/me":          "~",
		"/home/me/.zshrc":   "~/.zshrc",
		"/home/me/a/b":      "~/a/b",
		"/home/meow/.zshrc": "/home/meow/.zshrc",
		"/etc/x":            "/etc/x",
	}
	for in, want := range tests {
		if got := tildify(in, "/home/me"); got != want {
			t.Errorf("tildify(%q) = %q, want %q", in, got, want)
		}
	}
}
