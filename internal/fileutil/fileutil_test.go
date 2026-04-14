package fileutil_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ydakilux/reforge/internal/fileutil"
)

func TestGetDriveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		tests := []struct {
			input string
			want  string
		}{
			{`C:\Videos\foo.mp4`, `C:\`},
			{`D:\`, `D:\`},
			{`C:\deep\path\file.mkv`, `C:\`},
		}
		for _, tc := range tests {
			got := fileutil.GetDriveRoot(tc.input)
			if got != tc.want {
				t.Errorf("GetDriveRoot(%q) = %q, want %q", tc.input, got, tc.want)
			}
		}
	} else {
		tests := []struct {
			input string
			want  string
		}{
			{"/mnt/d/Videos/foo.mp4", "/mnt/d/"},
			{"/mnt/c/Users/test/file.mkv", "/mnt/c/"},
			{"/media/user/USB/foo.mp4", "/media/user/USB/"},
			{"/home/user/file.mkv", "/home/user/"},
			{"/opt/data/file.mkv", "/opt/data/"},
			{"/", "/"},
		}
		for _, tc := range tests {
			got := fileutil.GetDriveRoot(tc.input)
			if got != tc.want {
				t.Errorf("GetDriveRoot(%q) = %q, want %q", tc.input, got, tc.want)
			}
		}
	}
	if got := fileutil.GetDriveRoot(""); got != "/" {
		t.Errorf("GetDriveRoot(\"\") = %q, want \"/\"", got)
	}
	if got := fileutil.GetDriveRoot("relative/path/file.mp4"); got != "/" {
		t.Errorf("GetDriveRoot(relative) = %q, want \"/\"", got)
	}
}

func TestGetParentFolderName(t *testing.T) {
	if runtime.GOOS == "windows" {
		tests := []struct {
			filePath  string
			driveRoot string
			want      string
		}{
			{`C:\Videos\movie.mp4`, `C:\`, "Videos"},
			{`C:\movie.mp4`, `C:\`, "ROOT"},
			{`C:\A\B\movie.mp4`, `C:\`, "B"},
			{`D:\Foo\file.mkv`, `D:\`, "Foo"},
		}
		for _, tc := range tests {
			got := fileutil.GetParentFolderName(tc.filePath, tc.driveRoot)
			if got != tc.want {
				t.Errorf("GetParentFolderName(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
			}
		}
	} else {
		tests := []struct {
			filePath  string
			driveRoot string
			want      string
		}{
			{"/mnt/videos/movie.mp4", "/mnt/videos", "ROOT"},
			{"/mnt/videos/sub/movie.mp4", "/mnt/videos", "sub"},
			{"/home/user/a/b/movie.mp4", "/home/user", "b"},
			{"/media/disk/Foo/file.mkv", "/media/disk", "Foo"},
		}
		for _, tc := range tests {
			got := fileutil.GetParentFolderName(tc.filePath, tc.driveRoot)
			if got != tc.want {
				t.Errorf("GetParentFolderName(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
			}
		}
	}
}

func TestGetRelativePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		tests := []struct {
			filePath  string
			driveRoot string
			want      string
		}{
			{`C:\Videos\foo.mp4`, `C:\`, "Videos"},
			{`C:\foo.mp4`, `C:\`, "ROOT"},
			{`C:\A\B\foo.mp4`, `C:\`, `A\B`},
		}
		for _, tc := range tests {
			got := fileutil.GetRelativePath(tc.filePath, tc.driveRoot)
			if got != tc.want {
				t.Errorf("GetRelativePath(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
			}
		}
	} else {
		tests := []struct {
			filePath  string
			driveRoot string
			want      string
		}{
			{"/mnt/videos/foo.mp4", "/mnt/videos", "ROOT"},
			{"/mnt/videos/sub/foo.mp4", "/mnt/videos", "sub"},
			{"/home/user/a/b/foo.mp4", "/home/user", filepath.Join("a", "b")},
		}
		for _, tc := range tests {
			got := fileutil.GetRelativePath(tc.filePath, tc.driveRoot)
			if got != tc.want {
				t.Errorf("GetRelativePath(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
			}
		}
	}
}

func TestSanitizeFolderName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Normal", "Normal"},
		{"My:Video", "My_Video"},
		{"A*B?C", "A_B_C"},
		{`foo"bar`, "foo_bar"},
		{"a<b>c", "a_b_c"},
		{"pipe|pipe", "pipe_pipe"},
		{"clean", "clean"},
	}
	for _, tc := range tests {
		got := fileutil.SanitizeFolderName(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeFolderName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tc := range tests {
		got := fileutil.FormatBytes(tc.bytes)
		if got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestFmtElapsed(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{45 * time.Second, "45s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 00s"},
		{90 * time.Second, "1m 30s"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1h 02m 03s"},
		{2*time.Hour + 0*time.Minute + 0*time.Second, "2h 00m 00s"},
	}
	for _, tc := range tests {
		got := fileutil.FmtElapsed(tc.d)
		if got != tc.want {
			t.Errorf("FmtElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"hello", 2, "he"},
		{"hello", 1, "h"},
		{"", 5, ""},
	}
	for _, tc := range tests {
		got := fileutil.TruncateString(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("TruncateString(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}

func TestGetFileHash_FullConsistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	data := []byte("deterministic content for hash test")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h1, err := fileutil.GetFileHash(path, false)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := fileutil.GetFileHash(path, false)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("full hash not consistent: %q vs %q", h1, h2)
	}
	if len(h1) == 0 {
		t.Error("full hash is empty")
	}
}

func TestGetFileHash_PartialConsistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	data := []byte("deterministic content for partial hash test")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h1, err := fileutil.GetFileHash(path, true)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := fileutil.GetFileHash(path, true)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("partial hash not consistent: %q vs %q", h1, h2)
	}
}

func TestGetFileHash_DifferentFilesDifferentHashes(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.bin")
	pathB := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(pathA, []byte("content A"), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(pathB, []byte("content B"), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	hA, err := fileutil.GetFileHash(pathA, false)
	if err != nil {
		t.Fatalf("hash A: %v", err)
	}
	hB, err := fileutil.GetFileHash(pathB, false)
	if err != nil {
		t.Fatalf("hash B: %v", err)
	}
	if hA == hB {
		t.Errorf("different files produced same hash: %q", hA)
	}
}

func TestGetFileHash_PartialVsFullDiffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	full, err := fileutil.GetFileHash(path, false)
	if err != nil {
		t.Fatalf("full hash: %v", err)
	}
	partial, err := fileutil.GetFileHash(path, true)
	if err != nil {
		t.Fatalf("partial hash: %v", err)
	}
	if full == partial {
		t.Error("partial and full hash should differ (partial includes file size in hash)")
	}
}

func TestGetFileHash_NonexistentFile(t *testing.T) {
	_, err := fileutil.GetFileHash(filepath.Join(t.TempDir(), "missing.bin"), false)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
