package diff

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	before := []byte("export LANG=ko_KR.UTF-8\nexport EDITOR=vim\n")
	after := []byte("export LANG=ko_KR.UTF-8\nexport EDITOR=nvim\n")

	got, err := Unified("사본", "리포", before, after)
	if err != nil {
		t.Fatalf("Unified: %v", err)
	}

	for _, want := range []string{
		"--- 사본\n",
		"+++ 리포\n",
		"@@ -1,2 +1,2 @@\n",
		" export LANG=ko_KR.UTF-8\n",
		"-export EDITOR=vim\n",
		"+export EDITOR=nvim\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diff에 %q 가 없다\n%s", want, got)
		}
	}
}

func TestUnifiedSameContent(t *testing.T) {
	data := []byte("same\n")
	got, err := Unified("사본", "리포", data, data)
	if err != nil {
		t.Fatalf("Unified: %v", err)
	}
	if got != "" {
		t.Errorf("diff = %q, want empty", got)
	}
}

// 리포에만 있는 파일을 볼 때처럼 한쪽이 비어 있으면 모든 줄이 추가로 나와야 한다.
func TestUnifiedEmptyBefore(t *testing.T) {
	got, err := Unified("사본", "리포", nil, []byte("a\nb\n"))
	if err != nil {
		t.Fatalf("Unified: %v", err)
	}
	if !strings.Contains(got, "+a\n+b\n") {
		t.Errorf("diff에 추가된 줄이 없다\n%s", got)
	}
}

func TestUnifiedBinary(t *testing.T) {
	text := []byte("text\n")
	binary := []byte("PNG\x00\x01")

	if _, err := Unified("사본", "리포", binary, text); !errors.Is(err, ErrBinary) {
		t.Errorf("before가 바이너리일 때 err = %v, want ErrBinary", err)
	}
	if _, err := Unified("사본", "리포", text, binary); !errors.Is(err, ErrBinary) {
		t.Errorf("after가 바이너리일 때 err = %v, want ErrBinary", err)
	}
}

func TestUnifiedTooLarge(t *testing.T) {
	limit := bytes.Repeat([]byte("a"), MaxSize)
	if _, err := Unified("사본", "리포", limit, nil); err != nil {
		t.Errorf("MaxSize와 같은 크기에서 err = %v, want nil", err)
	}

	// 크기가 넘으면 NUL 바이트가 있어도 크기 쪽 에러가 나와야 한다.
	large := append(bytes.Repeat([]byte("a"), MaxSize), 0)
	if _, err := Unified("사본", "리포", nil, large); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}
