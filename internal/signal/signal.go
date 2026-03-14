package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const signalDir = ".crit"
const signalFile = "review.signal"

// SignalData is the JSON content of the signal file.
type SignalData struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

// SignalFile represents an active review signal.
type SignalFile struct {
	Path string
}

// Create creates a new signal file at .crit/review.signal.
// If a stale signal file exists (dead PID), it is cleaned up first.
func Create() (*SignalFile, error) {
	path := filepath.Join(signalDir, signalFile)

	if err := CleanStale(); err != nil {
		return nil, fmt.Errorf("cleaning stale signal: %w", err)
	}

	// Check if a live signal already exists
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("review signal already exists at %s (another review is in progress)", path)
	}

	if err := os.MkdirAll(signalDir, 0755); err != nil {
		return nil, fmt.Errorf("creating %s directory: %w", signalDir, err)
	}

	data := SignalData{
		PID:       os.Getpid(),
		CreatedAt: time.Now().UTC(),
	}

	content, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return nil, fmt.Errorf("writing signal file: %w", err)
	}

	return &SignalFile{Path: path}, nil
}

// Exists reports whether a signal file exists.
func Exists() bool {
	_, err := os.Stat(filepath.Join(signalDir, signalFile))
	return err == nil
}

// Path returns the path to the signal file.
func Path() string {
	return filepath.Join(signalDir, signalFile)
}

// Remove deletes the signal file.
func Remove() error {
	return os.Remove(filepath.Join(signalDir, signalFile))
}

// WaitForRemoval polls until the signal file is removed or the context is cancelled.
func (s *SignalFile) WaitForRemoval(interval time.Duration) error {
	for {
		if _, err := os.Stat(s.Path); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		time.Sleep(interval)
	}
}

// CleanStale removes the signal file if it exists and the PID it references is dead.
func CleanStale() error {
	path := filepath.Join(signalDir, signalFile)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no signal file, nothing to clean
		}
		return err
	}

	var data SignalData
	if err := json.Unmarshal(content, &data); err != nil {
		// Corrupt signal file — remove it
		return os.Remove(path)
	}

	if !processAlive(data.PID) {
		return os.Remove(path)
	}

	return nil
}

// processAlive checks if a process with the given PID is still running.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Signal 0 tests if process exists.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
