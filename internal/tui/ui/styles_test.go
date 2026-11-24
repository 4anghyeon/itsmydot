package ui

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// 비교 보기 배경은 256색 터미널에서도 바뀌지 않도록 256색표에 있는 색만 쓴다.
// 옅은 분홍이나 초록은 256색으로 바뀌면서 노랑이나 하늘색이 된다.
func TestDiffTonesSurviveANSI256(t *testing.T) {
	for name, p := range map[string]Palette{"dark": darkPalette, "light": lightPalette} {
		tones := []color.Color{
			p.DiffRemoved.Line, p.DiffRemoved.Number, p.DiffRemoved.Word,
			p.DiffAdded.Line, p.DiffAdded.Number, p.DiffAdded.Word,
			p.DiffHunk,
		}
		for _, c := range tones {
			converted := colorprofile.ANSI256.Convert(c)
			r1, g1, b1, _ := c.RGBA()
			r2, g2, b2, _ := converted.RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				t.Errorf("%s: %v becomes %v in 256 colors", name, c, converted)
			}
		}
	}
}
