package format

import (
	"math"
	"testing"
)

func TestTokens(t *testing.T) {
	cases := map[int64]string{999: "999", 1100: "1.1k", 38300000: "38.3M"}
	for in, want := range cases {
		if got := Tokens(in); got != want {
			t.Errorf("Tokens(%d) = %q, want %q", in, got, want)
		}
	}
	if got := Tokens(-1); got != Dash {
		t.Errorf("Tokens(-1) = %q, want dash", got)
	}
}

func TestSavedNormal(t *testing.T) {
	if got := Saved(1100, 3860, 1); got != "1.1k · 28.5%" {
		t.Errorf("got %q", got)
	}
}

func TestSavedImpossible(t *testing.T) {
	for _, c := range [][2]int64{{15000, 1000}, {-1, 100}, {5, 0}} {
		if got := Saved(c[0], c[1], 1); got != Dash {
			t.Errorf("Saved(%d, %d) = %q, want dash", c[0], c[1], got)
		}
	}
}

func TestPctRejectsNaN(t *testing.T) {
	if _, ok := Pct(math.NaN(), 100, 1); ok {
		t.Error("NaN part accepted")
	}
	if _, ok := Pct(1, math.Inf(1), 1); ok {
		t.Error("Inf whole accepted")
	}
}
