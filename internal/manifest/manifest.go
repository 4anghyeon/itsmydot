// Package manifest 는 itsmydot이 동기화할 수 있는 dotfile 목록인 manifest.yaml을 파싱한다.
//
// 전체 흐름:
//
//	Load(파일 경로)
//	  └─ 1. 파일을 바이트로 읽음
//	  └─ 2. Parse(바이트)
//	         ├─ 2-1. YAML → Manifest 구조체로 변환
//	         ├─ 2-2. validate: 필수 필드 / 이름 중복 / 위험한 경로 검사
//	         └─ 2-3. 각 target의 ~를 홈 디렉토리로 치환
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

// Entry 는 동기화 가능한 dotfile 하나를 나타낸다.
type Entry struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	Description string `yaml:"description"`
}

// Manifest 는 manifest.yaml 파일 전체를 나타낸다.
type Manifest struct {
	Entries []Entry `yaml:"entries"`
}

// Load 는 디스크의 manifest.yaml 파일을 읽어서 파싱한다.
func Load(filename string) (*Manifest, error) {
	// 1. 파일 전체를 바이트 슬라이스([]byte)로 읽는다. 파일이 없거나 권한이 없으면 err가 채워진다.
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// 2. 읽은 바이트를 Parse에 넘겨서 나머지(변환/검증/치환)를 맡긴다.
	return Parse(data)
}

// Parse 는 manifest YAML을 파싱하고 검증한 뒤, 모든 target의 ~를 홈 디렉토리로 바꾼다.
func Parse(data []byte) (*Manifest, error) {
	// 2-1. YAML → 구조체 변환.
	//      빈 Manifest를 만들고, 그 주소(&m)를 넘겨야 Unmarshal이 원본 m에 값을 채운다.
	//      YAML 문법이 깨졌거나 타입이 안 맞으면 에러.
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// 2-2. 내용 검증. 문제가 하나라도 있으면 Manifest를 돌려주지 않는다.
	if err := m.validate(); err != nil {
		return nil, err
	}

	// 2-3. ~ 치환. 홈 디렉토리는 한 번만 조회해서 모든 엔트리에 재사용한다.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	for i := range m.Entries {
		m.Entries[i].Target = expandHome(m.Entries[i].Target, home)
	}

	return &m, nil
}

// validate 는 첫 오류에서 멈추지 않고 모든 문제를 모아서 돌려준다.
func (m *Manifest) validate() error {
	// 1. 엔트리가 하나도 없으면 더 볼 것이 없으므로 바로 실패.
	if len(m.Entries) == 0 {
		return errors.New("manifest has no entries")
	}

	// 2. 발견한 에러를 모아 둘 슬라이스와, 이미 나온 이름을 기록할 맵(Set 용도)을 준비한다.
	var errs []error
	seen := make(map[string]bool)

	// 3. 엔트리를 하나씩 돌면서 검사한다.
	for i, e := range m.Entries {
		// 3-1. 에러 메시지에 쓸 라벨. 이름이 있으면 `entry "zshrc"`,
		//      없으면 순번으로 `entry #2` 처럼 표시한다.
		label := fmt.Sprintf("entry #%d", i+1)
		if e.Name != "" {
			label = fmt.Sprintf("entry %q", e.Name)
		}

		// 3-2. name: 비어 있으면 누락, 이미 본 이름이면 중복.
		if e.Name == "" {
			errs = append(errs, fmt.Errorf("%s: name is required", label))
		} else if seen[e.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate name", label))
		}
		seen[e.Name] = true

		// 3-3. source: 비어 있으면 누락, 리포 밖을 가리키면 거부.
		if e.Source == "" {
			errs = append(errs, fmt.Errorf("%s: source is required", label))
		} else if !isSafeRelPath(e.Source) {
			errs = append(errs, fmt.Errorf("%s: source must be a repo-relative path without '..' (got %q)", label, e.Source))
		}

		// 3-4. target: 비어 있으면 누락.
		if e.Target == "" {
			errs = append(errs, fmt.Errorf("%s: target is required", label))
		}
	}

	// 4. 모은 에러를 하나로 합쳐서 돌려준다.
	return errors.Join(errs...)
}

// isSafeRelPath 는 절대 경로나 리포 루트 밖으로 나가는 경로를 거부한다.
func isSafeRelPath(p string) bool {
	// 1. 절대 경로(/etc/passwd 등)는 거부.
	if path.IsAbs(p) || filepath.IsAbs(p) {
		return false
	}

	// 2. 경로를 정규화한다. 예: "files/../../x" → "../x", "./a/b" → "a/b"
	clean := path.Clean(p)

	// 3. 정규화 결과가 리포 루트 자체(".")이거나, 루트 밖("..", "../...")이면 거부.
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
