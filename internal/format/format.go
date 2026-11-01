// Package format renders numbers for the status line. Impossible values
// render as Dash instead of being clamped.
package format

import (
	"fmt"
	"math"
)

const Dash = "—"

// Tokens renders a token count as 999, 1.1k or 38.3M. Negative counts render as Dash.
func Tokens(n int64) string {
	switch {
	case n < 0:
		return Dash
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

// Pct renders part/whole as a percentage. ok is false when the input is not
// finite, part is negative, whole is not positive, or part exceeds whole.
func Pct(part, whole float64, digits int) (string, bool) {
	if math.IsNaN(part) || math.IsInf(part, 0) || math.IsNaN(whole) || math.IsInf(whole, 0) {
		return Dash, false
	}
	if part < 0 || whole <= 0 || part > whole {
		return Dash, false
	}
	return fmt.Sprintf("%.*f%%", digits, part*100/whole), true
}

// Saved renders "<tokens> · <pct>" or Dash when the pair is impossible.
func Saved(saved, whole int64, digits int) string {
	pct, ok := Pct(float64(saved), float64(whole), digits)
	if !ok {
		return Dash
	}
	return Tokens(saved) + " · " + pct
}
