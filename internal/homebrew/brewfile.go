package homebrew

import (
	"strings"
)

// Kind 는 Brewfile 줄의 종류다. 값은 Brewfile에 적는 명령 이름 그대로다.
type Kind string

const (
	Tap     Kind = "tap"  // 서드파티 저장소
	Formula Kind = "brew" // 명령줄 도구
	Cask    Kind = "cask" // macOS 앱
)

// Package 는 Brewfile의 tap, brew, cask 줄 하나다.
type Package struct {
	Kind Kind
	Name string // 따옴표 안의 이름. PeonPing/tap/peon-ping처럼 tap 경로가 붙을 수 있다

	// Line 은 주석을 뗀 줄 전체다. 이름 뒤에 붙은 설치 옵션을 잃지 않도록 설치할 때 이 줄을 그대로 쓴다.
	Line string

	// Description 은 줄 끝 주석이다. 없으면 바로 위 섹션 주석을 쓰고 그것도 없으면 빈 문자열이다.
	Description string
}

// Key 는 같은 패키지를 가리키는 줄을 하나로 묶는 열쇠다.
// 프로필 여러 곳에 같은 패키지가 있어도 한 번만 고르고 한 번만 설치한다.
// Homebrew는 이름의 대소문자를 가리지 않으므로 소문자로 맞춘다.
func (p Package) Key() string {
	return string(p.Kind) + " " + strings.ToLower(p.Name)
}

// ShortName 은 tap 경로를 뗀 이름이다. brew list가 이 이름으로 보여 준다. tap은 user/repo 전체가 이름이라 그대로 둔다.
func (p Package) ShortName() string {
	if p.Kind == Tap {
		return p.Name
	}
	return p.Name[strings.LastIndex(p.Name, "/")+1:]
}

// ParseBrewfile 은 Brewfile에서 tap, brew, cask 줄을 순서대로 읽는다. 나머지 줄과 형식이 맞지 않는 줄은 건너뛴다.
// Brewfile은 Ruby 문법이지만 itsmydot이 다루는 파일은 한 줄에 패키지 하나를 적는 단순한 형태라 Ruby를 해석하지 않고 줄 단위로 읽는다.
//
// "# --- Shell ---" 같은 주석만 있는 줄은 섹션 이름으로 보고 다음 섹션 주석까지 이어지는 줄의 설명이 없을 때 대신 쓴다.
func ParseBrewfile(data []byte) []Package {
	var packages []Package
	section := ""
	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			section = sectionName(line)
			continue
		}
		statement, comment := splitComment(line)
		p, ok := parseStatement(statement)
		if !ok {
			continue
		}
		p.Description = comment
		if p.Description == "" {
			p.Description = section
		}
		packages = append(packages, p)
	}
	return packages
}

// sectionName 은 주석 줄에서 #과 양 끝의 - 장식을 걷어 낸 섹션 이름이다.
func sectionName(line string) string {
	return strings.Trim(strings.TrimLeft(line, "#"), " -")
}

// splitComment 는 따옴표 밖의 첫 # 에서 줄을 나눠 문장과 주석을 돌려준다.
func splitComment(line string) (statement, comment string) {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		}
	}
	return strings.TrimSpace(line), ""
}

// parseStatement 는 `cask "raycast"` 같은 문장에서 종류와 첫 따옴표 안의 이름을 읽는다.
func parseStatement(statement string) (Package, bool) {
	word, rest, ok := strings.Cut(statement, " ")
	if !ok {
		return Package{}, false
	}
	kind := Kind(word)
	if kind != Tap && kind != Formula && kind != Cask {
		return Package{}, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" || (rest[0] != '"' && rest[0] != '\'') {
		return Package{}, false
	}
	end := strings.IndexByte(rest[1:], rest[0])
	if end <= 0 {
		return Package{}, false
	}
	return Package{Kind: kind, Name: rest[1 : end+1], Line: statement}, true
}
