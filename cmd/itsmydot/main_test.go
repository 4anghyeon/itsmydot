package main

import "testing"

func TestReleaseTag(t *testing.T) {
	tests := map[string]bool{
		"v0.6.0":  true,
		"v12.0.3": true,
		"(devel)": false,
		"":        false,
		"v0.6.1-0.20261022123456-abcdef123456+dirty": false,
	}
	for in, want := range tests {
		if got := releaseTag.MatchString(in); got != want {
			t.Errorf("releaseTag(%q) = %v, want %v", in, got, want)
		}
	}
}
