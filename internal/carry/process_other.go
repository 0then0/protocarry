//go:build !darwin && !linux

package carry

import "os/exec"

func prepareProcess(c *exec.Cmd)    {}
func killProcess(c *exec.Cmd) error { return c.Process.Kill() }
