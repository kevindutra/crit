package signal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndExists(t *testing.T) {
	setupTestDir(t)

	if Exists() {
		t.Fatal("signal should not exist before Create()")
	}

	sig, err := Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if !Exists() {
		t.Error("signal should exist after Create()")
	}

	// Verify signal file content
	content, err := os.ReadFile(sig.Path)
	if err != nil {
		t.Fatalf("reading signal file: %v", err)
	}

	var data SignalData
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("unmarshaling signal data: %v", err)
	}

	if data.PID != os.Getpid() {
		t.Errorf("signal PID = %d, want %d", data.PID, os.Getpid())
	}

	if time.Since(data.CreatedAt) > 5*time.Second {
		t.Errorf("signal CreatedAt is too old: %v", data.CreatedAt)
	}
}

func TestRemove(t *testing.T) {
	setupTestDir(t)

	_, err := Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := Remove(); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	if Exists() {
		t.Error("signal should not exist after Remove()")
	}
}

func TestCreateFailsWhenSignalExists(t *testing.T) {
	setupTestDir(t)

	_, err := Create()
	if err != nil {
		t.Fatalf("first Create() error: %v", err)
	}

	_, err = Create()
	if err == nil {
		t.Error("second Create() should fail when signal already exists")
	}
}

func TestCleanStaleRemovesDeadPID(t *testing.T) {
	setupTestDir(t)

	path := filepath.Join(signalDir, signalFile)
	os.MkdirAll(signalDir, 0755)

	// Write a signal with a PID that doesn't exist
	data := SignalData{
		PID:       999999999, // very unlikely to be a real process
		CreatedAt: time.Now().UTC(),
	}
	content, _ := json.Marshal(data)
	os.WriteFile(path, content, 0644)

	if err := CleanStale(); err != nil {
		t.Fatalf("CleanStale() error: %v", err)
	}

	if Exists() {
		t.Error("stale signal should have been removed")
	}
}

func TestCleanStalePreservesLivePID(t *testing.T) {
	setupTestDir(t)

	// Create a signal with our own PID (definitely alive)
	_, err := Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := CleanStale(); err != nil {
		t.Fatalf("CleanStale() error: %v", err)
	}

	if !Exists() {
		t.Error("signal for live PID should not be removed")
	}
}

func TestCleanStaleHandlesCorruptFile(t *testing.T) {
	setupTestDir(t)

	path := filepath.Join(signalDir, signalFile)
	os.MkdirAll(signalDir, 0755)
	os.WriteFile(path, []byte("not valid json"), 0644)

	if err := CleanStale(); err != nil {
		t.Fatalf("CleanStale() error: %v", err)
	}

	if Exists() {
		t.Error("corrupt signal file should have been removed")
	}
}

func TestCleanStaleNoFileIsNoop(t *testing.T) {
	setupTestDir(t)

	if err := CleanStale(); err != nil {
		t.Fatalf("CleanStale() with no file should not error: %v", err)
	}
}

func TestWaitForRemoval(t *testing.T) {
	setupTestDir(t)

	sig, err := Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Remove the signal file after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Remove(sig.Path)
	}()

	done := make(chan error, 1)
	go func() {
		done <- sig.WaitForRemoval(50 * time.Millisecond)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForRemoval() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForRemoval() timed out")
	}
}

func TestPath(t *testing.T) {
	expected := filepath.Join(".crit", "review.signal")
	if got := Path(); got != expected {
		t.Errorf("Path() = %q, want %q", got, expected)
	}
}

// setupTestDir changes to a temp directory for test isolation and restores on cleanup.
func setupTestDir(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	os.Chdir(tmp)
	t.Cleanup(func() { os.Chdir(orig) })
}
