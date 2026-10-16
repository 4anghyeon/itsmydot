package diff

import (
	"strings"
	"unicode"

	"github.com/aymanbagabas/go-udiff"
)

// Range 는 한 줄 안에서 바뀐 부분의 바이트 위치다. Start는 포함하고 End는 포함하지 않는다.
type Range struct {
	Start, End int
}

// Words 는 지운 줄과 추가한 줄을 짝지었을 때 각 줄에서 바뀐 단어의 위치를 돌려준다.
// 글자 단위로 비교하면 label과 fieldLabel처럼 겹치는 글자가 군데군데 남아서 단어 단위로 비교한다.
// 같은 부분이 절반도 안 되면 줄 전체를 바꾼 것이라 단어를 집어 보여 줘도 읽기만 어려워서 nil을 돌려준다.
func Words(before, after string) (removed, added []Range) {
	if before == after {
		return nil, nil
	}
	beforeWords, afterWords := splitWords(before), splitWords(after)
	beforeStarts, afterStarts := wordStarts(beforeWords), wordStarts(afterWords)

	// 단어 하나를 한 줄로 이어 붙여 줄 단위 diff를 단어 단위 diff로 쓴다.
	// edit의 위치는 이어 붙인 문자열 기준이라 몇 번째 단어인지로 바꿔 둔다.
	wordAt := make(map[int]int, len(beforeWords)+1)
	offset := 0
	for i, word := range beforeWords {
		wordAt[offset] = i
		offset += len(word) + 1
	}
	wordAt[offset] = len(beforeWords)

	same := len(before)
	shift := 0 // 앞선 edit들 때문에 after 쪽 단어 번호가 before보다 밀린 만큼
	for _, edit := range udiff.Lines(joinWords(beforeWords), joinWords(afterWords)) {
		start, end := wordAt[edit.Start], wordAt[edit.End]
		inserted := strings.Count(edit.New, "\n")
		if end > start {
			removed = append(removed, Range{beforeStarts[start], beforeStarts[end]})
			same -= beforeStarts[end] - beforeStarts[start]
		}
		if inserted > 0 {
			first := start + shift
			added = append(added, Range{afterStarts[first], afterStarts[first+inserted]})
		}
		shift += inserted - (end - start)
	}

	if same*2 < max(len(before), len(after)) {
		return nil, nil
	}
	return removed, added
}

// splitWords 는 줄을 단어로 나눈다. 글자와 숫자가 이어진 묶음과 공백이 이어진 묶음은 한 단어로,
// 그 밖의 기호는 한 글자씩 따로 센다. 나눈 단어를 이어 붙이면 원래 줄이 된다.
func splitWords(line string) []string {
	var words []string
	start := 0
	kind := func(r rune) int {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_':
			return 1
		case unicode.IsSpace(r):
			return 2
		}
		return 0
	}
	previous := -1
	for i, r := range line {
		current := kind(r)
		if i > start && (current == 0 || current != previous) {
			words = append(words, line[start:i])
			start = i
		}
		previous = current
	}
	if start < len(line) {
		words = append(words, line[start:])
	}
	return words
}

// wordStarts 는 각 단어가 원래 줄에서 시작하는 바이트 위치다. 끝에 줄 길이를 하나 더 붙여 마지막 단어의 끝도 찾을 수 있게 한다.
func wordStarts(words []string) []int {
	starts := make([]int, len(words)+1)
	for i, word := range words {
		starts[i+1] = starts[i] + len(word)
	}
	return starts
}

func joinWords(words []string) string {
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, "\n") + "\n"
}
