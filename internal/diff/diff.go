// Package diff 는 비교 보기 화면에 보여 줄 두 파일의 차이를 계산한다.
// git 바이너리 없이 Go 라이브러리로 계산한다.
package diff

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// MaxSize 는 비교할 수 있는 파일 하나의 최대 크기다. dotfile은 보통 수 KB라서
// 이보다 크면 화면에서 읽을 만한 diff가 나오지 않고 계산만 오래 걸린다.
const MaxSize = 1 << 20

var (
	ErrBinary   = errors.New("binary file")
	ErrTooLarge = errors.New("file too large")
)

// Kind 는 diff 한 줄이 어느 쪽 파일에 있는 줄인지다.
type Kind int

const (
	Context Kind = iota // 양쪽에 같은 내용으로 있는 줄
	Removed             // before에만 있는 줄
	Added               // after에만 있는 줄
)

// Line 은 diff 한 줄이다. 줄 번호는 1부터 세고 그 파일에 없는 줄이면 0이다.
type Line struct {
	Kind                      Kind
	BeforeNumber, AfterNumber int
	Text                      string  // 줄바꿈은 뺀 내용
	NoNewline                 bool    // 파일 마지막 줄인데 줄바꿈으로 끝나지 않으면 true
	Changes                   []Range // 짝지은 줄과 비교해 바뀐 단어의 위치. 짝이 없으면 nil이다
}

// Hunk 는 바뀐 줄과 그 앞뒤 문맥 줄을 묶은 덩어리다. Start는 덩어리 첫 줄의 번호이고 Count는 그 파일 쪽 줄 수다.
type Hunk struct {
	BeforeStart, BeforeCount int
	AfterStart, AfterCount   int
	Lines                    []Line
}

// Compare 는 before를 after로 바꾸는 diff를 hunk 단위로 돌려준다. 두 내용이 같으면 nil이다.
// 줄 단위 비교가 의미 없는 파일은 diff 대신 [ErrBinary] 나 [ErrTooLarge] 를 돌려준다.
func Compare(before, after []byte) ([]Hunk, error) {
	for _, data := range [][]byte{before, after} {
		// 크기를 먼저 봐야 큰 파일에서 NUL 바이트를 찾느라 전체를 훑지 않는다.
		if len(data) > MaxSize {
			return nil, ErrTooLarge
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil, ErrBinary
		}
	}

	content := string(before)
	unified, err := udiff.ToUnifiedDiff("", "", content, udiff.Lines(content, string(after)), udiff.DefaultContextLines)
	if err != nil {
		return nil, err
	}

	var hunks []Hunk
	for _, h := range unified.Hunks {
		hunk := Hunk{BeforeStart: h.FromLine, AfterStart: h.ToLine}
		beforeNumber, afterNumber := h.FromLine, h.ToLine
		for _, l := range h.Lines {
			line := Line{Text: strings.TrimSuffix(l.Content, "\n"), NoNewline: !strings.HasSuffix(l.Content, "\n")}
			switch l.Kind {
			case udiff.Delete:
				line.Kind, line.BeforeNumber = Removed, beforeNumber
				beforeNumber++
			case udiff.Insert:
				line.Kind, line.AfterNumber = Added, afterNumber
				afterNumber++
			default:
				line.Kind, line.BeforeNumber, line.AfterNumber = Context, beforeNumber, afterNumber
				beforeNumber++
				afterNumber++
			}
			hunk.Lines = append(hunk.Lines, line)
		}
		hunk.BeforeCount, hunk.AfterCount = beforeNumber-h.FromLine, afterNumber-h.ToLine
		markWordChanges(hunk.Lines)
		hunks = append(hunks, hunk)
	}
	return hunks, nil
}

// Header 는 @@ -1,3 +1,4 @@ 꼴의 hunk 머리줄이다.
func (h Hunk) Header() string {
	return "@@ " + hunkRange("-", h.BeforeStart, h.BeforeCount) + " " + hunkRange("+", h.AfterStart, h.AfterCount) + " @@"
}

// hunkRange 는 머리줄의 한쪽 범위다. 빈 파일 쪽은 GNU diff처럼 0,0으로 쓴다.
func hunkRange(sign string, start, count int) string {
	switch {
	case count > 1:
		return fmt.Sprintf("%s%d,%d", sign, start, count)
	case count == 0 && start == 1:
		return sign + "0,0"
	}
	return fmt.Sprintf("%s%d", sign, start)
}

// markWordChanges 는 연달아 지운 줄 바로 뒤에 추가한 줄이 오면 순서대로 한 줄씩 짝지어 바뀐 단어를 표시한다.
// 짝이 모자란 줄은 줄 전체가 바뀐 것으로 둔다.
func markWordChanges(lines []Line) {
	for i := 0; i < len(lines); {
		removedEnd := i
		for removedEnd < len(lines) && lines[removedEnd].Kind == Removed {
			removedEnd++
		}
		if removedEnd == i {
			i++
			continue
		}
		addedEnd := removedEnd
		for addedEnd < len(lines) && lines[addedEnd].Kind == Added {
			addedEnd++
		}
		for k := 0; k < min(removedEnd-i, addedEnd-removedEnd); k++ {
			removed, added := &lines[i+k], &lines[removedEnd+k]
			removed.Changes, added.Changes = words(removed.Text, added.Text)
		}
		i = addedEnd
	}
}
