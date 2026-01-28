package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxLogSize  = 10 * 1024 * 1024 // 10MB
	maxLogFiles = 5
)

// Logger handles file-based logging with rotation
type Logger struct {
	logDir      string
	currentFile *os.File
	mu          sync.Mutex
	writers     []io.Writer
}

// New creates a new Logger
func New(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	l := &Logger{
		logDir:  logDir,
		writers: []io.Writer{},
	}

	if err := l.openLogFile(); err != nil {
		return nil, err
	}

	return l, nil
}

// AddWriter adds an additional writer (e.g., UI widget)
func (l *Logger) AddWriter(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writers = append(l.writers, w)
}

// Log writes a log message with timestamp
func (l *Logger) Log(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s\n", timestamp, message)

	// Write to file
	if l.currentFile != nil {
		l.currentFile.WriteString(line)
		l.checkRotation()
	}

	// Write to additional writers
	for _, w := range l.writers {
		w.Write([]byte(line))
	}
}

// Logf writes a formatted log message
func (l *Logger) Logf(format string, args ...interface{}) {
	l.Log(fmt.Sprintf(format, args...))
}

// Error writes an error message
func (l *Logger) Error(message string) {
	l.Log("ERROR: " + message)
}

// Errorf writes a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// Close closes the current log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		return l.currentFile.Close()
	}
	return nil
}

// GetLogPath returns the current log file path
func (l *Logger) GetLogPath() string {
	if l.currentFile != nil {
		return l.currentFile.Name()
	}
	return ""
}

func (l *Logger) openLogFile() error {
	filename := fmt.Sprintf("tunnelmanager_%s.log", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(l.logDir, filename)

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.currentFile = file
	return nil
}

func (l *Logger) checkRotation() {
	if l.currentFile == nil {
		return
	}

	info, err := l.currentFile.Stat()
	if err != nil {
		return
	}

	if info.Size() >= maxLogSize {
		l.rotateLog()
	}
}

func (l *Logger) rotateLog() {
	if l.currentFile != nil {
		l.currentFile.Close()
	}

	// Rename current log with timestamp
	oldPath := l.currentFile.Name()
	newPath := oldPath + "." + time.Now().Format("150405")
	os.Rename(oldPath, newPath)

	// Open new log file
	l.openLogFile()

	// Cleanup old logs
	l.cleanupOldLogs()
}

func (l *Logger) cleanupOldLogs() {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "tunnelmanager_") && strings.Contains(entry.Name(), ".log") {
			logFiles = append(logFiles, filepath.Join(l.logDir, entry.Name()))
		}
	}

	if len(logFiles) <= maxLogFiles {
		return
	}

	// Sort by modification time (oldest first)
	sort.Slice(logFiles, func(i, j int) bool {
		infoI, _ := os.Stat(logFiles[i])
		infoJ, _ := os.Stat(logFiles[j])
		if infoI == nil || infoJ == nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest files
	for i := 0; i < len(logFiles)-maxLogFiles; i++ {
		os.Remove(logFiles[i])
	}
}

// UIWriter wraps a function to implement io.Writer
type UIWriter struct {
	WriteFunc func(string)
}

// Write implements io.Writer
func (w *UIWriter) Write(p []byte) (n int, err error) {
	if w.WriteFunc != nil {
		w.WriteFunc(string(p))
	}
	return len(p), nil
}
