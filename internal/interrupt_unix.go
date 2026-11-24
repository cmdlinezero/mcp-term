//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package internal

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

func executeTimed(ctx context.Context, c LabCommand, cwd string) CommandLog {
	start := time.Now()
	args := []string{"-c", c.Script}
	cmd := exec.Command("sh", args...)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	stdout, stderr := &limitedBuffer{limit: MaxOutputBytes}, &limitedBuffer{limit: MaxOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return newLabRecord("sh", args, start, -1, "", err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(c.InterruptAfter)
	defer timer.Stop()
	reason := ""
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		reason = "context_cancelled"
	case <-timer.C:
		reason = "timed_interrupt"
	}
	if reason != "" {
		// Prefer a completed process over a racing timer.
		select {
		case err = <-done:
			reason = ""
		default:
			if signalErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); signalErr != nil {
				reason = "interrupt_failed"
			}
			grace := time.NewTimer(2 * time.Second)
			select {
			case err = <-done:
			case <-grace.C:
				reason = "forced_termination"
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				err = <-done
			}
			grace.Stop()
			// Remove remaining group members even if the shell exited first.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	exit := cmd.ProcessState.ExitCode()
	if err != nil && exit == 0 {
		exit = -1
	}
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		exit = 128 + int(status.Signal())
	}
	logical := exit
	if reason == "timed_interrupt" && (exit == 0 || exit == 130) {
		logical = 0
	} else if reason != "" {
		logical = 1
	}
	e := newLabRecord("sh", args, start, logical, stdout.String(), stderr.String())
	if reason != "" {
		e.ProcessExitCode = &exit
		e.CompletionReason = reason
	}
	return e
}
