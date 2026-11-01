package render

import "strings"

const reset = "\x1b[0m"

// ANSI prints lines with terminal colours, without a trailing newline.
func ANSI(lines []Line) string {
	rows := make([]string, len(lines))
	for i, l := range lines {
		var b strings.Builder
		for _, s := range l {
			if code := sgr(s); code != "" {
				b.WriteString("\x1b[" + code + "m" + s.Text + reset)
			} else {
				b.WriteString(s.Text)
			}
		}
		rows[i] = b.String()
	}
	return strings.Join(rows, "\n")
}

func sgr(s Segment) string {
	var codes []string
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Dim {
		codes = append(codes, "2")
	}
	if s.FG != "" {
		codes = append(codes, s.FG)
	}
	return strings.Join(codes, ";")
}
