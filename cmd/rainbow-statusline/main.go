package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/milczarekIT/rainbow-statusline/internal/adapter"
	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
	"golang.org/x/term"
)

func main() {
	// Drawing never crashes and never shows an error on the assistant's screen.
	defer func() { _ = recover() }()

	flags := flag.NewFlagSet("rainbow-statusline", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agent := flags.String("agent", "claude", "assistant whose input arrives on stdin")
	widthFlag := flags.Int("width", 0, "columns available to the status line")
	_ = flags.Parse(os.Args[1:])

	if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return
	}
	stdin, _ := io.ReadAll(os.Stdin)
	s := adapter.Parse(*agent, stdin)
	fillFromSystem(&s)
	width := resolveWidth(*widthFlag, os.Getenv("COLUMNS"), stderrWidth, controllingTerminalWidth)
	fmt.Print(render.ANSI(render.Draw(s, nil, config.View{}, width)))
}

func fillFromSystem(s *model.Session) {
	if u, err := user.Current(); err == nil {
		s.User = u.Username[strings.LastIndex(u.Username, `\`)+1:]
	}
	if h, err := os.Hostname(); err == nil {
		s.Host, _, _ = strings.Cut(h, ".")
	}
	if s.Cwd == "" {
		s.Cwd, _ = os.Getwd()
	}
}

// resolveWidth takes the first positive width: flag, COLUMNS, then each terminal
// source. 0 means no trimming.
func resolveWidth(flagWidth int, columns string, sources ...func() int) int {
	if flagWidth > 0 {
		return flagWidth
	}
	if n, err := strconv.Atoi(columns); err == nil && n > 0 {
		return n
	}
	for _, source := range sources {
		if n := source(); n > 0 {
			return n
		}
	}
	return 0
}

func stderrWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil {
		return 0
	}
	return w
}

func controllingTerminalWidth() int {
	f, err := os.OpenFile(controllingTerminal, os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer f.Close()
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}
