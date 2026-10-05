package carry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const RejectExit = 75

type Process struct {
	ExitCode          int    `json:"exit_code"`
	Started           bool   `json:"started"`
	Timeout           bool   `json:"timeout"`
	StdoutTruncated   bool   `json:"stdout_truncated"`
	StderrTruncated   bool   `json:"stderr_truncated"`
	InputBytesWritten int    `json:"input_bytes_written"`
	Error             string `json:"error,omitempty"`
	Stdout            []byte `json:"-"`
	Stderr            []byte `json:"-"`
}

type stream struct {
	data      []byte
	truncated bool
	err       error
}

func runProcess(c Config, input []byte) Process {
	r := Process{ExitCode: -1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Limits.TimeoutMS)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Adapter[0], c.Adapter[1:]...)
	cmd.Dir = c.WorkingDir
	prepareProcess(cmd)
	cmd.Cancel = func() error { return killProcess(cmd) }
	cmd.WaitDelay = 250 * time.Millisecond
	inR, inW, err := os.Pipe()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer inR.Close()
	defer inW.Close()
	outR, outW, err := os.Pipe()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer outR.Close()
	defer outW.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer errR.Close()
	defer errW.Close()
	cmd.Stdin = inR
	cmd.Stdout = outW
	cmd.Stderr = errW
	if err = cmd.Start(); err != nil {
		r.Error = err.Error()
		return r
	}
	r.Started = true
	_ = inR.Close()
	_ = outW.Close()
	_ = errW.Close()
	var once sync.Once
	stop := func() { once.Do(cancel) }
	read := func(f *os.File, limit int, ch chan<- stream) {
		var b bytes.Buffer
		buf := make([]byte, 8192)
		s := stream{}
		for {
			n, e := f.Read(buf)
			if n > 0 {
				room := limit - b.Len()
				if n > room {
					s.truncated = true
					ncopy := room
					if ncopy > 0 {
						b.Write(buf[:ncopy])
					}
					stop()
				} else {
					b.Write(buf[:n])
				}
			}
			if e != nil {
				if !errors.Is(e, io.EOF) {
					s.err = e
				}
				break
			}
		}
		s.data = b.Bytes()
		ch <- s
	}
	outCh, errCh := make(chan stream, 1), make(chan stream, 1)
	go read(outR, c.Limits.MaxStdoutBytes, outCh)
	go read(errR, c.Limits.MaxStderrBytes, errCh)
	type written struct {
		n   int
		err error
	}
	inCh := make(chan written, 1)
	go func() { n, e := io.Copy(inW, bytes.NewReader(input)); _ = inW.Close(); inCh <- written{int(n), e} }()
	waitErr := cmd.Wait()
	// The parent has completed the one-message job. Reap remaining group members.
	_ = killProcess(cmd)
	r.ExitCode = cmd.ProcessState.ExitCode()
	r.Timeout = errors.Is(ctx.Err(), context.DeadlineExceeded)
	// Bound draining even when a detached descendant inherited the descriptors.
	drainTimer := time.AfterFunc(250*time.Millisecond, func() { _ = killProcess(cmd); _ = outR.Close(); _ = errR.Close() })
	out, logs := <-outCh, <-errCh
	drainTimer.Stop()
	_ = inW.Close()
	w := <-inCh
	r.InputBytesWritten = w.n
	r.Stdout = out.data
	r.Stderr = logs.data
	r.StdoutTruncated = out.truncated
	r.StderrTruncated = logs.truncated
	switch {
	case r.Timeout:
		r.Error = "adapter timeout"
	case out.truncated:
		r.Error = "stdout exceeded max_stdout_bytes"
	case logs.truncated:
		r.Error = "stderr exceeded max_stderr_bytes"
	case out.err != nil || logs.err != nil:
		r.Error = "incomplete pipe drain (possibly an inherited pipe)"
	case r.ExitCode == RejectExit: // A controlled refusal may occur before reading stdin.
	case waitErr != nil:
		r.Error = fmt.Sprintf("adapter did not complete successfully: %v", waitErr)
	case w.n != len(input) || w.err != nil:
		r.Error = "adapter exited before receiving complete input"
	case len(r.Stdout) == 0 && c.EmptyOutput != EmptyOutputMessage:
		r.Error = "empty stdout: no transport evidence"
	}
	return r
}
