//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package internal

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTimedInterrupt(t *testing.T) {
	c := LabCommand{Script: "trap 'exit 0' INT; echo ready; while :; do sleep 1; done", InterruptAfter: 200 * time.Millisecond}
	e := executeTimed(context.Background(), c, t.TempDir())
	if e.ExitCode != 0 || e.CompletionReason != "timed_interrupt" || !strings.Contains(e.Stdout, "ready") || e.ProcessExitCode == nil {
		t.Fatalf("%+v", e)
	}
	c.Script = "exit 7"
	e = executeTimed(context.Background(), c, t.TempDir())
	if e.ExitCode != 7 || e.CompletionReason != "" {
		t.Fatalf("%+v", e)
	}
}
func TestInterruptAttributes(t *testing.T) {
	var c LabCommand
	if err := parseInterrupt(`timeout="10s" interrupt="ctrl-c"`, &c); err != nil || c.InterruptAfter != 10*time.Second {
		t.Fatal(c, err)
	}
	if parseInterrupt(`timeout="0s" interrupt="ctrl-c"`, &c) == nil {
		t.Fatal("accepted zero")
	}
}
