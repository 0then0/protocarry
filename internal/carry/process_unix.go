//go:build darwin || linux

package carry

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcess(c *exec.Cmd) error {
	if c.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return os.ErrProcessDone
	}
	return err
}
