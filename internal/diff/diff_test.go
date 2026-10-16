package diff

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

// 줄 번호는 hunk 시작 번호부터 세고, 지운 줄은 before에만 추가한 줄은 after에만 번호가 붙는다.
// 멀리 떨어진 변경은 hunk가 나뉘고 줄 수가 1인 쪽은 머리줄에서 쉼표 부분이 빠진다.
func TestCompare(t *testing.T) {
	before := []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n")
	after := []byte("a\nB\nc\nd\ne\nf\ng\nh\ni\nj\nk\n")

	hunks, err := Compare(before, after)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	want := []Hunk{
		{
			BeforeStart: 1, BeforeCount: 5, AfterStart: 1, AfterCount: 5,
			Lines: []Line{
				{Kind: Context, BeforeNumber: 1, AfterNumber: 1, Text: "a"},
				{Kind: Removed, BeforeNumber: 2, Text: "b"},
				{Kind: Added, AfterNumber: 2, Text: "B"},
				{Kind: Context, BeforeNumber: 3, AfterNumber: 3, Text: "c"},
				{Kind: Context, BeforeNumber: 4, AfterNumber: 4, Text: "d"},
				{Kind: Context, BeforeNumber: 5, AfterNumber: 5, Text: "e"},
			},
		},
		{
			BeforeStart: 8, BeforeCount: 3, AfterStart: 8, AfterCount: 4,
			Lines: []Line{
				{Kind: Context, BeforeNumber: 8, AfterNumber: 8, Text: "h"},
				{Kind: Context, BeforeNumber: 9, AfterNumber: 9, Text: "i"},
				{Kind: Context, BeforeNumber: 10, AfterNumber: 10, Text: "j"},
				{Kind: Added, AfterNumber: 11, Text: "k"},
			},
		},
	}
	if !reflect.DeepEqual(hunks, want) {
		t.Fatalf("hunks =\n%+v\nwant\n%+v", hunks, want)
	}
	if got := hunks[0].Header(); got != "@@ -1,5 +1,5 @@" {
		t.Errorf("first header = %q", got)
	}
	if got := hunks[1].Header(); got != "@@ -8,3 +8,4 @@" {
		t.Errorf("second header = %q", got)
	}
}

func TestCompareSameContent(t *testing.T) {
	hunks, err := Compare([]byte("same\n"), []byte("same\n"))
	if err != nil || hunks != nil {
		t.Errorf("hunks = %+v err = %v, want nil", hunks, err)
	}
}

// 빈 파일과 비교하면 모든 줄이 추가로 나오고 머리줄의 빈 쪽은 0,0이다.
func TestCompareEmptyBefore(t *testing.T) {
	hunks, err := Compare(nil, []byte("a\nb\n"))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(hunks) != 1 || hunks[0].Header() != "@@ -0,0 +1,2 @@" {
		t.Fatalf("hunks = %+v", hunks)
	}
	for _, line := range hunks[0].Lines {
		if line.Kind != Added {
			t.Errorf("line %+v should be added", line)
		}
	}
}

// 파일 끝 줄바꿈만 다르면 같은 내용의 줄이 지워졌다가 추가된 것으로 나오고, 줄바꿈이 없는 쪽에 NoNewline이 붙는다.
// 내용이 같으니 바뀐 단어는 없다.
func TestCompareMissingNewline(t *testing.T) {
	hunks, err := Compare([]byte("a\nlast"), []byte("a\nlast\n"))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	want := []Line{
		{Kind: Context, BeforeNumber: 1, AfterNumber: 1, Text: "a"},
		{Kind: Removed, BeforeNumber: 2, Text: "last", NoNewline: true},
		{Kind: Added, AfterNumber: 2, Text: "last"},
	}
	if len(hunks) != 1 || !reflect.DeepEqual(hunks[0].Lines, want) {
		t.Fatalf("hunks = %+v", hunks)
	}
}

// 연달아 지운 줄과 바로 뒤에 추가한 줄을 순서대로 짝지어 바뀐 단어를 표시한다. 짝이 없는 줄에는 표시하지 않는다.
func TestCompareMarksWordChanges(t *testing.T) {
	before := []byte("<SearchField className=\"w-42\" label={label ?? name} isActive={isActive}>\n" +
		"placeholder={placeholder}\n")
	after := []byte("<SearchField className=\"w-42\" label={fieldLabel} isActive={isActive}>\n" +
		"placeholder={placeholder ?? fieldLabel}\n" +
		"extra\n")
	hunks, err := Compare(before, after)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	changed := func(line Line) []string {
		var parts []string
		for _, c := range line.Changes {
			parts = append(parts, line.Text[c.Start:c.End])
		}
		return parts
	}
	want := [][]string{
		{"label ?? name"},
		nil,
		{"fieldLabel"},
		{" ?? fieldLabel"},
		nil,
	}
	lines := hunks[0].Lines
	if len(lines) != len(want) {
		t.Fatalf("lines = %+v", lines)
	}
	for i := range want {
		if got := changed(lines[i]); !reflect.DeepEqual(got, want[i]) {
			t.Errorf("line %d %q: changes = %q, want %q", i, lines[i].Text, got, want[i])
		}
	}
}

func TestCompareBinary(t *testing.T) {
	text := []byte("text\n")
	binary := []byte("PNG\x00\x01")

	if _, err := Compare(binary, text); !errors.Is(err, ErrBinary) {
		t.Errorf("before가 바이너리일 때 err = %v, want ErrBinary", err)
	}
	if _, err := Compare(text, binary); !errors.Is(err, ErrBinary) {
		t.Errorf("after가 바이너리일 때 err = %v, want ErrBinary", err)
	}
}

func TestCompareTooLarge(t *testing.T) {
	limit := bytes.Repeat([]byte("a"), MaxSize)
	if _, err := Compare(limit, nil); err != nil {
		t.Errorf("MaxSize와 같은 크기에서 err = %v, want nil", err)
	}

	// 크기가 넘으면 NUL 바이트가 있어도 크기 쪽 에러가 나와야 한다.
	large := append(bytes.Repeat([]byte("a"), MaxSize), 0)
	if _, err := Compare(nil, large); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}
