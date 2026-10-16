// Package diff 는 비교 보기 화면에 보여 줄 두 파일의 차이를 unified diff로 만든다.
// git 바이너리 없이 Go 라이브러리로 계산한다.
package diff

import (
	"bytes"
	"errors"

	"github.com/aymanbagabas/go-udiff"
)

// MaxSize 는 비교할 수 있는 파일 하나의 최대 크기다. dotfile은 보통 수 KB라서
// 이보다 크면 화면에서 읽을 만한 diff가 나오지 않고 계산만 오래 걸린다.
const MaxSize = 1 << 20

var (
	ErrBinary   = errors.New("binary file")
	ErrTooLarge = errors.New("file too large")
)

// Unified 는 before를 after로 바꾸는 unified diff를 돌려준다. 두 내용이 같으면 빈 문자열이다.
// beforeName과 afterName은 diff 머리줄의 --- 와 +++ 뒤에 붙는 이름이다.
// 줄 단위 비교가 의미 없는 파일은 diff 대신 [ErrBinary] 나 [ErrTooLarge] 를 돌려준다.
func Unified(beforeName, afterName string, before, after []byte) (string, error) {
	for _, data := range [][]byte{before, after} {
		// 크기를 먼저 봐야 큰 파일에서 NUL 바이트를 찾느라 전체를 훑지 않는다.
		if len(data) > MaxSize {
			return "", ErrTooLarge
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return "", ErrBinary
		}
	}
	return udiff.Unified(beforeName, afterName, string(before), string(after)), nil
}
