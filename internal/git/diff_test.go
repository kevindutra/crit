package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseNameStatusZ(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []FileChange
	}{
		{
			name: "empty",
			out:  "",
			want: nil,
		},
		{
			name: "modified with spaces",
			out:  "M\x00docs/user guide.md\x00",
			want: []FileChange{
				{Path: "docs/user guide.md", Status: StatusModified},
			},
		},
		{
			name: "added and deleted",
			out:  "A\x00new file.txt\x00D\x00old file.txt\x00",
			want: []FileChange{
				{Path: "new file.txt", Status: StatusAdded},
				{Path: "old file.txt", Status: StatusDeleted},
			},
		},
		{
			name: "rename with spaces",
			out:  "R100\x00old name.txt\x00new name.txt\x00M\x00other.txt\x00",
			want: []FileChange{
				{Path: "new name.txt", OldPath: "old name.txt", Status: StatusRenamed},
				{Path: "other.txt", Status: StatusModified},
			},
		},
		{
			name: "copy consumes both paths",
			out:  "C75\x00src file.txt\x00copy file.txt\x00A\x00added.txt\x00",
			want: []FileChange{
				{Path: "copy file.txt", OldPath: "src file.txt", Status: StatusRenamed},
				{Path: "added.txt", Status: StatusAdded},
			},
		},
		{
			name: "non-ASCII path",
			out:  "M\x00\u30c6\u30b9\u30c8 \u30e1\u30e2.txt\x00",
			want: []FileChange{
				{Path: "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", Status: StatusModified},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNameStatusZ(tt.out)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseNumstatZ(t *testing.T) {
	out := "3\t1\ttext file.txt\x00" +
		"-\t-\tbin file.dat\x00" +
		"-\t-\t\x00old bin.dat\x00new bin.dat\x00" +
		"2\t2\t\x00old name.txt\x00new name.txt\x00"

	got := parseNumstatZ(out)
	want := map[string]bool{
		"bin file.dat": true,
		"new bin.dat":  true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for path := range want {
		if !got[path] {
			t.Errorf("missing binary path %q in %v", path, got)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFilesWithSpecialPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, dir, "docs/user guide.md", []byte("# Guide\n"))
	writeFile(t, dir, "old name.txt", []byte("hello\nworld\n"))
	writeFile(t, dir, "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", []byte("hello\n"))
	writeFile(t, dir, "bin file.dat", []byte{0x00, 0x01, 0x02})
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "docs/user guide.md", []byte("# User Guide\n"))
	writeFile(t, dir, "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", []byte("goodbye\n"))
	writeFile(t, dir, "bin file.dat", []byte{0x03, 0x04, 0x05, 0x00})
	runGit(t, dir, "mv", "old name.txt", "new name.txt")
	writeFile(t, dir, "untracked file.txt", []byte("new\n"))

	t.Chdir(dir)
	files, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}

	byPath := make(map[string]FileChange, len(files))
	for _, fc := range files {
		byPath[fc.Path] = fc
	}

	assertStatus := func(path string, status ChangeStatus) {
		t.Helper()
		fc, ok := byPath[path]
		if !ok {
			t.Errorf("missing %q in %+v", path, files)
			return
		}
		if fc.Status != status {
			t.Errorf("%q: got status %v, want %v", path, fc.Status, status)
		}
	}

	assertStatus("docs/user guide.md", StatusModified)
	assertStatus("\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", StatusModified)
	assertStatus("bin file.dat", StatusBinary)
	assertStatus("untracked file.txt", StatusUntracked)

	fc, ok := byPath["new name.txt"]
	if !ok {
		t.Fatalf("missing rename target in %+v", files)
	}
	if fc.Status != StatusRenamed || fc.OldPath != "old name.txt" {
		t.Errorf("rename: got %+v, want Status=StatusRenamed OldPath=%q", fc, "old name.txt")
	}
}
