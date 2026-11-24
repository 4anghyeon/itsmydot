// Package manifest 는 itsmydot이 다루는 dotfile과 에이전트 파일, Brewfile 목록인 manifest.yaml을 읽고 검증한다.
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
// Children이 있으면 그룹이다. 그룹도 자기 파일을 가진다. zsh라면 ~/.config/zsh/itsmydot.zsh가 그룹 파일이고
// 그 파일이 불러오는 ~/.config/zsh/*.zsh가 하위 항목이다. 하위 항목은 다시 Children을 가질 수 없다.
// Source와 Target 없이 Children만 있으면 자기 파일 없이 하위 항목을 묶기만 하는 폴더 그룹이다.
//
// LoadedBy가 있으면 연결할 때 그 셸 설정 파일 끝에 Target을 불러오는 한 줄을 넣고 끊을 때 그 줄을 지운다.
// ~는 Target처럼 홈 디렉토리로 바뀐다.
type Entry struct {
	Name        string  `yaml:"name"`
	Source      string  `yaml:"source"`
	Target      string  `yaml:"target"`
	LoadedBy    string  `yaml:"loaded_by"`
	Description string  `yaml:"description"`
	Children    []Entry `yaml:"children"`
}

// IsFolder 는 자기 파일 없이 하위 항목만 묶는 폴더 그룹인지 알려 준다.
func (e Entry) IsFolder() bool {
	return e.Source == "" && e.Target == ""
}

// Brewfile 은 Homebrew 패키지 목록 파일 하나다. 파일 하나가 base나 dev 같은 프로필 하나다.
type Brewfile struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Description string `yaml:"description"`
}

// Manifest 는 manifest.yaml 파일 전체다.
type Manifest struct {
	Entries    []Entry    `yaml:"entries"`
	Agentfiles []Entry    `yaml:"agentfiles"`
	Brewfiles  []Brewfile `yaml:"brewfiles"`
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
	expandEntries(m.Entries, home)
	expandEntries(m.Agentfiles, home)

	return &m, nil
}

// expandEntries 는 항목과 하위 항목의 target과 loaded_by에 들어 있는 ~를 home으로 바꾼다.
func expandEntries(entries []Entry, home string) {
	for i := range entries {
		e := &entries[i]
		e.Target, e.LoadedBy = expandHome(e.Target, home), expandHome(e.LoadedBy, home)
		for j := range e.Children {
			child := &e.Children[j]
			child.Target, child.LoadedBy = expandHome(child.Target, home), expandHome(child.LoadedBy, home)
		}
	}
}

// validate 는 첫 문제에서 멈추지 않고 모든 문제를 모아서 돌려준다.
// manifest를 고칠 때 틀린 곳을 한 번에 다 볼 수 있게 하려는 것이다.
func (m *Manifest) validate() []string {
	if len(m.Entries) == 0 {
		return []string{"manifest에 항목이 없어요"}
	}

	problems := validateEntries(m.Entries, "")
	problems = append(problems, validateEntries(m.Agentfiles, "agentfiles의 ")...)

	// Brewfile은 dotfile과 다른 목록에 보여 주므로 이름도 따로 겹치지 않으면 된다.
	seenBrewfiles := make(map[string]bool)
	for i, b := range m.Brewfiles {
		problems = append(problems, validateBrewfile(b, fmt.Sprintf("brewfiles의 %d번째 항목", i+1), seenBrewfiles)...)
	}
	return problems
}

// validateEntries 는 한 섹션의 항목과 하위 항목을 검사한다. section은 이름 없는 항목을 가리킬 때 앞에 붙이는 섹션 이름이다.
// 섹션마다 다른 목록에 보여 주므로 이름은 섹션 안에서만 겹치지 않으면 된다.
func validateEntries(entries []Entry, section string) []string {
	var problems []string
	// 그룹 안팎을 가리지 않고 이름이 겹치면 안 된다. TUI가 이름으로 항목을 찾는다.
	seen := make(map[string]bool)

	for i, e := range entries {
		label := fmt.Sprintf("%s%d번째 항목", section, i+1)
		// children 없이 source와 target만 빠진 항목은 폴더 그룹이 아니라 경로를 빠뜨린 것으로 알린다.
		problems = append(problems, validateEntry(e, label, e.IsFolder() && len(e.Children) > 0, seen)...)

		for j, child := range e.Children {
			childLabel := fmt.Sprintf("%s의 %d번째 하위 항목", entryLabel(e, label), j+1)
			problems = append(problems, validateEntry(child, childLabel, false, seen)...)
			if len(child.Children) > 0 {
				problems = append(problems, entryLabel(child, childLabel)+"은 하위 항목을 한 단계까지만 둘 수 있어요")
			}
		}
	}
	return problems
}

// validateBrewfile 은 Brewfile 항목 하나의 이름과 source를 검사한다.
func validateBrewfile(b Brewfile, label string, seen map[string]bool) []string {
	if b.Name != "" {
		label = b.Name + " Brewfile 항목"
	}
	var problems []string

	if b.Name == "" {
		problems = append(problems, label+"에 name이 없어요")
	} else if seen[b.Name] {
		problems = append(problems, b.Name+" 이름을 쓰는 Brewfile 항목이 이미 있어요")
	}
	seen[b.Name] = true

	if b.Source == "" {
		problems = append(problems, label+"에 source가 없어요")
	} else if !isSafeRelativePath(b.Source) {
		problems = append(problems, fmt.Sprintf("%s의 source는 저장소 기준 경로여야 하고 ..를 쓸 수 없어요 (지금 값 %q)", label, b.Source))
	}
	return problems
}

// validateEntry 는 항목 하나의 이름과 경로를 검사한다. 하위 항목은 따로 넘겨받으므로 여기서 보지 않는다.
// 폴더 그룹이면 source와 target이 없어야 하므로 경로를 검사하지 않는다.
func validateEntry(e Entry, label string, folder bool, seen map[string]bool) []string {
	label = entryLabel(e, label)
	var problems []string

	if e.Name == "" {
		problems = append(problems, label+"에 name이 없어요")
	} else if seen[e.Name] {
		problems = append(problems, e.Name+" 이름을 쓰는 항목이 이미 있어요")
	}
	seen[e.Name] = true

	if folder {
		if e.LoadedBy != "" {
			problems = append(problems, label+"은 폴더 그룹이라 loaded_by를 둘 수 없어요")
		}
		return problems
	}

	if e.Source == "" {
		problems = append(problems, label+"에 source가 없어요")
	} else if !isSafeRelativePath(e.Source) {
		problems = append(problems, fmt.Sprintf("%s의 source는 저장소 기준 경로여야 하고 ..를 쓸 수 없어요 (지금 값 %q)", label, e.Source))
	}

	if e.Target == "" {
		problems = append(problems, label+"에 target이 없어요")
	}
	if e.LoadedBy != "" && path.Clean(e.LoadedBy) == path.Clean(e.Target) {
		problems = append(problems, label+"의 loaded_by는 target과 다른 파일이어야 해요")
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
