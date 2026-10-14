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
type Entry struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	Description string `yaml:"description"`
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
		m.Entries[i].Target = expandHome(m.Entries[i].Target, home)
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
	seen := make(map[string]bool)

	for i, e := range m.Entries {
		// 이름이 없는 항목은 몇 번째인지로 가리킨다.
		label := fmt.Sprintf("entry #%d", i+1)
		if e.Name != "" {
			label = fmt.Sprintf("entry %q", e.Name)
		}

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
	}

	return errors.Join(errs...)
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
