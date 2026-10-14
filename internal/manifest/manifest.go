// Package manifest 는 itsmydot이 동기화할 수 있는 dotfile 목록인 manifest.yaml을 읽고 검증한다.
package manifest

import (
	"errors"
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

// Load 는 디스크에 있는 manifest 파일을 읽어서 [Parse] 로 넘긴다.
func Load(filename string) (*Manifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return Parse(data)
}

// Parse 는 manifest YAML을 파싱하고 검증한 뒤 모든 target의 ~를 홈 디렉토리로 바꾼다.
// 문제가 하나라도 있으면 Manifest를 돌려주지 않는다.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if err := m.validate(); err != nil {
		return nil, err
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
func (m *Manifest) validate() error {
	if len(m.Entries) == 0 {
		return errors.New("manifest has no entries")
	}

	var errs []error
	// 그룹 안팎을 가리지 않고 이름이 겹치면 안 된다. TUI가 이름으로 항목을 찾는다.
	seen := make(map[string]bool)

	for i, e := range m.Entries {
		// 이름이 없는 항목은 몇 번째인지로 가리킨다.
		label := fmt.Sprintf("entry #%d", i+1)
		errs = append(errs, validateEntry(e, label, seen)...)

		for j, child := range e.Children {
			childLabel := fmt.Sprintf("%s child #%d", entryLabel(e, label), j+1)
			errs = append(errs, validateEntry(child, childLabel, seen)...)
			if len(child.Children) > 0 {
				errs = append(errs, fmt.Errorf("%s: children cannot have children", entryLabel(child, childLabel)))
			}
		}
	}

	return errors.Join(errs...)
}

// validateEntry 는 항목 하나의 이름과 경로를 검사한다. 하위 항목은 따로 넘겨받으므로 여기서 보지 않는다.
func validateEntry(e Entry, label string, seen map[string]bool) []error {
	label = entryLabel(e, label)
	var errs []error

	if e.Name == "" {
		errs = append(errs, fmt.Errorf("%s: name is required", label))
	} else if seen[e.Name] {
		errs = append(errs, fmt.Errorf("%s: duplicate name", label))
	}
	seen[e.Name] = true

	if e.Source == "" {
		errs = append(errs, fmt.Errorf("%s: source is required", label))
	} else if !isSafeRelPath(e.Source) {
		errs = append(errs, fmt.Errorf("%s: source must be a repo-relative path without '..' (got %q)", label, e.Source))
	}

	if e.Target == "" {
		errs = append(errs, fmt.Errorf("%s: target is required", label))
	}
	return errs
}

// entryLabel 은 에러 메시지에서 항목을 가리킬 이름이다. 이름이 없으면 fallback을 쓴다.
func entryLabel(e Entry, fallback string) string {
	if e.Name != "" {
		return fmt.Sprintf("entry %q", e.Name)
	}
	return fallback
}

// isSafeRelPath 는 p가 리포 안을 가리키는 상대 경로인지 확인한다.
// 절대 경로나 리포 루트 자체, 루트 밖으로 나가는 경로는 거부한다.
func isSafeRelPath(p string) bool {
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
