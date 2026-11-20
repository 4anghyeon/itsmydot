package manifest

import (
	"errors"
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

func TestParseBrewfiles(t *testing.T) {
	m, err := Parse([]byte(`
entries: [{name: a, source: x, target: y}]
brewfiles:
  - name: base
    source: files/brew/base.Brewfile
    description: 공통 앱
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Brewfile{{Name: "base", Source: "files/brew/base.Brewfile", Description: "공통 앱"}}
	if !reflect.DeepEqual(m.Brewfiles, want) {
		t.Errorf("brewfiles = %+v, want %+v", m.Brewfiles, want)
	}
}

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr []string
	}{
		{"empty", `entries: []`, []string{"manifest에 항목이 없어요"}},
		{"missing fields", `entries: [{name: a}]`, []string{"a 항목에 source가 없어요", "a 항목에 target이 없어요"}},
		{"missing name", `entries: [{source: a, target: b}]`, []string{"1번째 항목에 name이 없어요"}},
		{"duplicate", `entries: [{name: a, source: x, target: y}, {name: a, source: x, target: y}]`, []string{"a 이름을 쓰는 항목이 이미 있어요"}},
		{"escaping source", `entries: [{name: a, source: ../etc/passwd, target: y}]`, []string{`a 항목의 source는 저장소 기준 경로여야 하고 ..를 쓸 수 없어요 (지금 값 "../etc/passwd")`}},
		{"absolute source", `entries: [{name: a, source: /etc/passwd, target: y}]`, []string{"..를 쓸 수 없어요"}},
		{"bad yaml", `entries: [`, []string{"YAML 문법을 확인해 주세요 (yaml:"}},
		{"child missing fields", `entries: [{name: g, source: x, target: y, children: [{}]}]`, []string{
			"g 항목의 1번째 하위 항목에 name이 없어요", "하위 항목에 source가 없어요", "하위 항목에 target이 없어요",
		}},
		{"duplicate across group", `entries: [{name: a, source: x, target: y}, {name: g, source: x, target: y, children: [{name: a, source: x, target: y}]}]`, []string{"a 이름을 쓰는 항목이 이미 있어요"}},
		{"brewfile missing fields", `{entries: [{name: a, source: x, target: y}], brewfiles: [{}]}`, []string{
			"brewfiles의 1번째 항목에 name이 없어요", "brewfiles의 1번째 항목에 source가 없어요",
		}},
		{"duplicate brewfile", `{entries: [{name: a, source: x, target: y}], brewfiles: [{name: b, source: x}, {name: b, source: x}]}`, []string{"b 이름을 쓰는 Brewfile 항목이 이미 있어요"}},
		{"escaping brewfile source", `{entries: [{name: a, source: x, target: y}], brewfiles: [{name: b, source: ../x}]}`, []string{"b Brewfile 항목의 source는 저장소 기준 경로여야"}},
		{"nested children", `entries: [{name: g, source: x, target: y, children: [{name: c, source: x, target: y, children: [{name: d, source: x, target: y}]}]}]`, []string{"c 항목은 하위 항목을 한 단계까지만 둘 수 있어요"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			var invalid *InvalidError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidError", err)
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

// 저장소에 있는 실제 manifest.yaml은 항상 파싱에 성공해야 하고 Brewfile의 source는 저장소에 있어야 한다.
func TestRepoManifest(t *testing.T) {
	m, err := Load("../../manifest.yaml")
	if err != nil {
		t.Fatalf("repo manifest.yaml is invalid: %v", err)
	}
	for _, b := range m.Brewfiles {
		if _, err := os.Stat(filepath.Join("../..", b.Source)); err != nil {
			t.Errorf("brewfile %s source missing: %v", b.Name, err)
		}
	}
}
