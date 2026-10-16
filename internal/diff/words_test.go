package diff

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitWords(t *testing.T) {
	line := `label={label ?? name}  x_1`
	want := []string{"label", "=", "{", "label", " ", "?", "?", " ", "name", "}", "  ", "x_1"}
	got := splitWords(line)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitWords = %q, want %q", got, want)
	}
	if strings.Join(got, "") != line {
		t.Error("joined words should be the original line")
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		name           string
		before, after  string
		removed, added []string
	}{
		{
			name:    "단어 하나 바꿈",
			before:  `<SearchField className="w-42" label={label ?? name} isActive={isActive}>`,
			after:   `<SearchField className="w-42" label={fieldLabel} isActive={isActive}>`,
			removed: []string{"label ?? name"},
			added:   []string{"fieldLabel"},
		},
		{
			name:   "끝에 덧붙임",
			before: `placeholder={placeholder}`,
			after:  `placeholder={placeholder ?? fieldLabel}`,
			added:  []string{" ?? fieldLabel"},
		},
		{
			name:   "한글 단어",
			before: "export LANG=ko_KR.UTF-8 # 한국어 설정",
			after:  "export LANG=en_US.UTF-8 # 한국어 설정",
			// 밑줄은 단어에 들어가므로 ko_KR이 통째로 바뀐다.
			removed: []string{"ko_KR"},
			added:   []string{"en_US"},
		},
		{
			name:   "같은 부분이 절반도 안 되면 표시하지 않음",
			before: "alias ll='ls -al'",
			after:  "export EDITOR=nvim",
		},
		{
			name:   "내용이 같음",
			before: "same",
			after:  "same",
		},
	}
	for _, tt := range tests {
		removed, added := words(tt.before, tt.after)
		if got := slicesOf(tt.before, removed); !reflect.DeepEqual(got, tt.removed) {
			t.Errorf("%s: removed = %q, want %q", tt.name, got, tt.removed)
		}
		if got := slicesOf(tt.after, added); !reflect.DeepEqual(got, tt.added) {
			t.Errorf("%s: added = %q, want %q", tt.name, got, tt.added)
		}
	}
}

func slicesOf(line string, ranges []Range) []string {
	var parts []string
	for _, r := range ranges {
		parts = append(parts, line[r.Start:r.End])
	}
	return parts
}
