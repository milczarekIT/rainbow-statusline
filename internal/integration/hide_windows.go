package integration

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console program started from a console-less process from
// opening its own window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
