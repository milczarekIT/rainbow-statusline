package cache

import (
	"os"
	"os/exec"
)

// spawnDetached starts "rainbow-statusline refresh <key>" and does not wait.
func spawnDetached(key string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "refresh", key)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
