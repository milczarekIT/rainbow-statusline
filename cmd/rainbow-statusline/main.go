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
)

func main() {
	// Drawing never crashes and never shows an error on the assistant's screen.
	defer func() { _ = recover() }()

	flags := flag.NewFlagSet("rainbow-statusline", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agent := flags.String("agent", "claude", "assistant whose input arrives on stdin")
	_ = flags.Parse(os.Args[1:])

	if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return
	}
	stdin, _ := io.ReadAll(os.Stdin)
	s := adapter.Parse(*agent, stdin)
	fillFromSystem(&s)
	width, _ := strconv.Atoi(os.Getenv("COLUMNS"))
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
