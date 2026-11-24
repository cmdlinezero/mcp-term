package internal

import (
	"fmt"
	"regexp"
	"time"
)

var timeoutAttr = regexp.MustCompile(`(?:^|\s)timeout=(?:"([^"]*)"|([^\s]+))`)
var interruptAttr = regexp.MustCompile(`(?:^|\s)interrupt=(?:"([^"]*)"|([^\s]+))`)

func parseInterrupt(attrs string, c *LabCommand) error {
	t, i := timeoutAttr.FindStringSubmatch(attrs), interruptAttr.FindStringSubmatch(attrs)
	if t == nil && i == nil {
		return nil
	}
	if t == nil || i == nil || i[1]+i[2] != "ctrl-c" || c.File != "" {
		return fmt.Errorf("timed interruption requires shell fence, timeout and interrupt=ctrl-c")
	}
	d, err := time.ParseDuration(t[1] + t[2])
	if err != nil || d <= 0 || d > 24*time.Hour {
		return fmt.Errorf("timeout must be positive and at most 24h")
	}
	c.InterruptAfter = d
	return nil
}
