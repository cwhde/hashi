package words_test

import (
	"testing"

	"git.juzo.io/juzo/hashi/internal/words"
)

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 changes", 1: "1 change", 2: "2 changes"} {
		if got := words.Count(n, "change", "changes"); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}
