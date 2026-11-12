package internal

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const DefaultTimeout = 30 * time.Second
const MaxOutputBytes = 1024 * 1024

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if remaining > n {
			remaining = n
		}
		_, _ = b.Buffer.Write(p[:remaining])
	}
	return n, nil
}

// Execute runs an executable directly, without a shell. This is not a sandbox.
func Execute(ctx context.Context, command string, args []string, timeout time.Duration) CommandLog {
	start := time.Now().UTC()
	if args == nil {
		args = []string{}
	}
	e := CommandLog{Timestamp: start.Format(time.RFC3339Nano), Command: command, Args: append([]string{}, args...), ExitCode: -1}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout = &limitedBuffer{limit: MaxOutputBytes}
	var stderr = &limitedBuffer{limit: MaxOutputBytes}
	if command == "" {
		e.Stderr = "empty command"
	} else {
		c := exec.CommandContext(ctx, command, args...)
		c.Stdout = stdout
		c.Stderr = stderr
		err := c.Run()
		e.Stdout = stdout.String()
		e.Stderr = stderr.String()
		if err == nil {
			e.ExitCode = 0
		} else {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				e.ExitCode = exit.ExitCode()
			} else if e.Stderr == "" {
				e.Stderr = err.Error()
			}
			if ctx.Err() != nil {
				e.ExitCode = -1
				e.Stderr += fmt.Sprintf("\nexecution interrupted: %v", ctx.Err())
			}
		}
	}
	e.Duration = time.Since(start).String()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil { // Extremely unlikely; preserve uniqueness using monotonic clock.
		nonce = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	material, _ := json.Marshal([]any{e.Command, e.Args, e.Timestamp, e.ExitCode, e.Stdout, hex.EncodeToString(nonce)})
	digest := sha256.Sum256(material)
	e.ID = hex.EncodeToString(digest[:8])
	return e
}
