package main

import "testing"

func TestResolveWidth(t *testing.T) {
	size := func(n int) func() int { return func() int { return n } }
	never := func() int { t.Error("source must not be read"); return 0 }
	cases := []struct {
		name    string
		flag    int
		columns string
		sources []func() int
		want    int
	}{
		{"flag wins", 50, "60", []func() int{never, never}, 50},
		{"columns next", 0, "60", []func() int{never, never}, 60},
		{"stderr next", 0, "", []func() int{size(70), never}, 70},
		{"tty next", 0, "", []func() int{size(0), size(80)}, 80},
		{"nothing means no trimming", 0, "", []func() int{size(0), size(0)}, 0},
		{"bad columns falls through", 0, "wide", []func() int{size(70)}, 70},
		{"zero columns falls through", 0, "0", []func() int{size(70)}, 70},
		{"negative values fall through", -1, "-5", []func() int{size(-3), size(80)}, 80},
	}
	for _, c := range cases {
		if got := resolveWidth(c.flag, c.columns, c.sources...); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
