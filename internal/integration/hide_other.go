//go:build !windows

package integration

import "os/exec"

func hideWindow(*exec.Cmd) {}
