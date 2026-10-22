// Package manifest 는 itsmydot이 동기화할 수 있는 dotfile 목록인 manifest.yaml을 읽고 검증한다.
package manifest

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry 는 동기화할 수 있는 dotfile 하나다. Target에 들어 있던 ~는 [Parse] 가 홈 디렉토리로 바꿔 둔다.
//
// Children이 있으면 그룹이다. 그룹도 자기 파일을 가진다. zsh라면 ~/.zshrc가 그룹 파일이고
// 그 파일이 불러오는 ~/.config/zsh/*.zsh가 하위 항목이다. 하위 항목은 다시 Children을 가질 수 없다.
type Entry struct {
	Name        string  `yaml:"name"`
	Source      string  `yaml:"source"`
	Target      string  `yaml:"target"`
	Description string  `yaml:"description"`
	Children    []Entry `yaml:"children"`
}

// Manifest 는 manifest.yaml 파일 전체다.
type Manifest struct {
	Entries []Entry `yaml:"entries"`
}

// InvalidError 는 manifest.yaml의 YAML 문법이 틀렸거나 검증을 통과하지 못했을 때 [Parse] 가 돌려준다.
// Problems에는 틀린 곳이 하나씩 화면에 그대로 보여 줄 한국어 문장으로 들어 있다.
type InvalidError struct {
	Problems []string
}

func (e *InvalidError) Error() string {
	return strings.Join(e.Problems, "\n")
}

// Load 는 디스크에 있는 manifest 파일을 읽어서 [Parse] 로 넘긴다.
func Load(filename string) (*Manifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return Parse(data)
}

// Parse 는 manifest YAML을 파싱하고 검증한 뒤 모든 target의 ~를 홈 디렉토리로 바꾼다.
// 문제가 하나라도 있으면 Manifest 없이 [*InvalidError] 를 돌려준다.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, &InvalidError{Problems: []string{"YAML 문법을 확인해 주세요 (" + err.Error() + ")"}}
	}

	if problems := m.validate(); len(problems) > 0 {
		return nil, &InvalidError{Problems: problems}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		e.Target = expandHome(e.Target, home)
		for j := range e.Children {
			e.Children[j].Target = expandHome(e.Children[j].Target, home)
		}
	}

	return &m, nil
}

// validate 는 첫 문제에서 멈추지 않고 모든 문제를 모아서 돌려준다.
// manifest를 고칠 때 틀린 곳을 한 번에 다 볼 수 있게 하려는 것이다.
func (m *Manifest) validate() []string {
	if len(m.Entries) == 0 {
		return []string{"manifest에 항목이 없어요"}
	}

	var problems []string
	// 그룹 안팎을 가리지 않고 이름이 겹치면 안 된다. TUI가 이름으로 항목을 찾는다.
	seen := make(map[string]bool)

	for i, e := range m.Entries {
		// 이름이 없는 항목은 몇 번째인지로 가리킨다.
		label := fmt.Sprintf("%d번째 항목", i+1)
		problems = append(problems, validateEntry(e, label, seen)...)

		for j, child := range e.Children {
			childLabel := fmt.Sprintf("%s의 %d번째 하위 항목", entryLabel(e, label), j+1)
			problems = append(problems, validateEntry(child, childLabel, seen)...)
			if len(child.Children) > 0 {
				problems = append(problems, entryLabel(child, childLabel)+"은 하위 항목을 한 단계까지만 둘 수 있어요")
			}
		}
	}
	return problems
}

// validateEntry 는 항목 하나의 이름과 경로를 검사한다. 하위 항목은 따로 넘겨받으므로 여기서 보지 않는다.
func validateEntry(e Entry, label string, seen map[string]bool) []string {
	label = entryLabel(e, label)
	var problems []string

	if e.Name == "" {
		problems = append(problems, label+"에 name이 없어요")
	} else if seen[e.Name] {
		problems = append(problems, e.Name+" 이름을 쓰는 항목이 이미 있어요")
	}
	seen[e.Name] = true

	if e.Source == "" {
		problems = append(problems, label+"에 source가 없어요")
	} else if !isSafeRelativePath(e.Source) {
		problems = append(problems, fmt.Sprintf("%s의 source는 저장소 기준 경로여야 하고 ..를 쓸 수 없어요 (지금 값 %q)", label, e.Source))
	}

	if e.Target == "" {
		problems = append(problems, label+"에 target이 없어요")
	}
	return problems
}

// entryLabel 은 문제를 알릴 때 항목을 가리키는 말이다. 이름이 없으면 fallback을 쓴다.
// 영문 이름 뒤에는 조사를 고를 수 없어서 언제나 "항목"으로 끝나게 만들어 뒤에 조사를 붙인다.
func entryLabel(e Entry, fallback string) string {
	if e.Name != "" {
		return e.Name + " 항목"
	}
	return fallback
}

// isSafeRelativePath 는 p가 저장소 안을 가리키는 상대 경로인지 확인한다.
// 절대 경로나 저장소 루트 자체, 루트 밖으로 나가는 경로는 거부한다.
func isSafeRelativePath(p string) bool {
	if path.IsAbs(p) || filepath.IsAbs(p) {
		return false
	}

	// files/../../x 처럼 중간에 .. 가 섞여 있어도 정리한 뒤에 판단한다.
	clean := path.Clean(p)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}

	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}

	return p
}
