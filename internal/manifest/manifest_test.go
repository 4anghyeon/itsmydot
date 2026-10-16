package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	m, err := Parse([]byte(`
entries:
  - name: zshrc
    source: files/zsh/.zshrc
    target: ~/.zshrc
    description: zsh 설정
  - name: abs
    source: files/x
    target: /etc/x
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(m.Entries))
	}

	got := m.Entries[0]
	want := Entry{Name: "zshrc", Source: "files/zsh/.zshrc", Target: filepath.Join(home, ".zshrc"), Description: "zsh 설정"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entry 0 = %+v, want %+v", got, want)
	}
	if m.Entries[1].Target != "/etc/x" {
		t.Errorf("absolute target changed: %q", m.Entries[1].Target)
	}
}

// 하위 항목의 target도 ~를 홈 디렉토리로 바꾼다.
func TestParseChildren(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	m, err := Parse([]byte(`
entries:
  - name: zshrc
    source: files/zsh/.zshrc
    target: ~/.zshrc
    children:
      - name: env
        source: files/zsh/00-env.zsh
        target: ~/.config/zsh/00-env.zsh
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	children := m.Entries[0].Children
	if len(children) != 1 {
		t.Fatalf("got %d children, want 1", len(children))
	}
	if want := filepath.Join(home, ".config/zsh/00-env.zsh"); children[0].Target != want {
		t.Errorf("child target = %q, want %q", children[0].Target, want)
	}
}

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr []string
	}{
		{"empty", `entries: []`, []string{"no entries"}},
		{"missing fields", `entries: [{name: a}]`, []string{"source is required", "target is required"}},
		{"missing name", `entries: [{source: a, target: b}]`, []string{"entry #1: name is required"}},
		{"duplicate", `entries: [{name: a, source: x, target: y}, {name: a, source: x, target: y}]`, []string{`"a": duplicate name`}},
		{"escaping source", `entries: [{name: a, source: ../etc/passwd, target: y}]`, []string{"without '..'"}},
		{"absolute source", `entries: [{name: a, source: /etc/passwd, target: y}]`, []string{"without '..'"}},
		{"bad yaml", `entries: [`, []string{"parse manifest"}},
		{"child missing fields", `entries: [{name: g, source: x, target: y, children: [{}]}]`, []string{
			`entry "g" child #1: name is required`, "source is required", "target is required",
		}},
		{"duplicate across group", `entries: [{name: a, source: x, target: y}, {name: g, source: x, target: y, children: [{name: a, source: x, target: y}]}]`, []string{`"a": duplicate name`}},
		{"nested children", `entries: [{name: g, source: x, target: y, children: [{name: c, source: x, target: y, children: [{name: d, source: x, target: y}]}]}]`, []string{`"c": children cannot have children`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	tests := map[string]string{
		"~":          "/h",
		"~/.zshrc":   "/h/.zshrc",
		"~/a/b":      "/h/a/b",
		"/abs":       "/abs",
		"~other/x":   "~other/x",
		"rel/~/path": "rel/~/path",
	}
	for in, want := range tests {
		if got := expandHome(in, "/h"); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

// 저장소에 있는 실제 manifest.yaml은 항상 파싱에 성공해야 한다.
func TestRepoManifest(t *testing.T) {
	if _, err := Load("../../manifest.yaml"); err != nil {
		t.Fatalf("repo manifest.yaml is invalid: %v", err)
	}
}
