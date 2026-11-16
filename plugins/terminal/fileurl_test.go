package terminal

import (
	"runtime"
	"testing"
)

// TestFileURLPath covers the shapes a shell reports its directory in, and
// the drive a file URL of a Windows one holds.
func TestFileURLPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"/tmp/x", "/tmp/x"},
		{"file:///tmp/x", "/tmp/x"},
		{"file://host/tmp/x", "/tmp/x"},
		{"file:///tmp/a%20b", "/tmp/a b"},
	}
	// A Windows drive is no directory named C:, so the slash before it goes
	// there and stays elsewhere, where the path really starts with one.
	drive, want := "file:///C:/src", "/C:/src"
	if runtime.GOOS == "windows" {
		want = "C:/src"
	}
	tests = append(tests, struct{ in, want string }{drive, want})
	for _, tt := range tests {
		if got := fileURLPath(tt.in); got != tt.want {
			t.Errorf("fileURLPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
