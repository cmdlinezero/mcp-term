package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type CommandLog struct {
	ID        string   `json:"id"`
	Timestamp string   `json:"timestamp"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Stdout    string   `json:"stdout"`
	Stderr    string   `json:"stderr"`
	ExitCode  int      `json:"exit_code"`
	Duration  string   `json:"duration"`
}

type Logger struct {
	mu   sync.Mutex
	file *os.File
}

func OpenLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	return &Logger{file: f}, nil
}
func (l *Logger) Write(entry CommandLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := l.file.Write(b)
	if err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return l.file.Sync()
}
func (l *Logger) Close() error { l.mu.Lock(); defer l.mu.Unlock(); return l.file.Close() }

func ReadLogs(path string) ([]CommandLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var entries []CommandLog
	for scanner.Scan() {
		var e CommandLog
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("log line %d: %w", len(entries)+1, err)
		}
		if e.Command == "" {
			return nil, fmt.Errorf("log line %d: empty command", len(entries)+1)
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}
