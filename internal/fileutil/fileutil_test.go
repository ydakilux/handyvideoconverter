package fileutil_test

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"video-converter/internal/fileutil"
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
		for _, input := range []string{
			"/mnt/videos/foo.mp4",
			"/home/user/file.mkv",
			"/",
		} {
			got := fileutil.GetDriveRoot(input)
			if got != "/" {
				t.Errorf("GetDriveRoot(%q) = %q, want \"/\"", input, got)
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
		{45 * time.Second, "0m 45s"},
		{90 * time.Second, "1m 30s"},
		{60 * time.Second, "1m 00s"},
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
