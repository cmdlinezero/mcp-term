//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package internal

import (
	"context"
	"time"
)

func executeTimed(ctx context.Context, c LabCommand, cwd string) CommandLog {
	return newLabRecord("sh", []string{"-c", c.Script}, time.Now(), 1, "", "timed interruption unsupported on this platform")
}
