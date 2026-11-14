package pty

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
)

// TestConPTYClearScreen clears the screen of PowerShell, whose prompt sat
// on the console's last row: the console repaints it on the first, which
// PSReadLine's redraws then keep to.
func TestConPTYClearScreen(t *testing.T) {
	if windowsBuild() < 26100 {
		t.Skip("ClearScreen needs Windows 11 24H2")
	}
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		if shell, err = exec.LookPath("powershell"); err != nil {
			t.Skip("no PowerShell")
		}
	}
	p, err := Start(Options{Path: shell, Args: []string{"pwsh", "-NoLogo", "-NoProfile"}, Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.Kill()
		p.Close()
	})
	var mu sync.Mutex
	var out bytes.Buffer
	go io.Copy(writerFunc(func(b []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(b)
	}), p)
	since := func(n int) string {
		mu.Lock()
		defer mu.Unlock()
		return out.String()[n:]
	}
	length := func() int {
		mu.Lock()
		defer mu.Unlock()
		return out.Len()
	}

	wait.Until(t, "the prompt", func() bool { return strings.Contains(since(0), "PS ") })
	n := length()
	if _, err := p.Write([]byte("1..20\r")); err != nil {
		t.Fatal(err)
	}
	// The numbers, then the prompt again, on the last row.
	wait.Until(t, "the numbers and the prompt", func() bool {
		s := since(n)
		i := strings.Index(s, "20")
		return i >= 0 && strings.Contains(s[i:], "PS ")
	})

	n = length()
	if err := p.ClearScreen(); err != nil {
		t.Fatal(err)
	}
	top := regexp.MustCompile(`\x1b\[1;\d+H`)
	wait.Until(t, "the prompt repainted on the first row", func() bool { return top.MatchString(since(n)) })
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// TestConPTYForwardsMouse starts a program that asks for the mouse the way pi
// does: on the screen it switches to, before anything turns virtual terminal
// input on. A console that has that bit off keeps the modes to itself and the
// terminal hears nothing of the wheel, which then comes back as alternate
// scroll, as arrow keys.
func TestConPTYForwardsMouse(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("the test executable is not known")
	}
	p, err := Start(Options{
		Path: exe,
		Args: []string{exe, "-test.run=TestHelperProcess"},
		Env:  append(os.Environ(), "MYGO_PTY_HELPER=mouse"),
		Cols: 80,
		Rows: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.Kill()
		p.Close()
	})
	var mu sync.Mutex
	var out bytes.Buffer
	go io.Copy(writerFunc(func(b []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(b)
	}), p)
	wait.Until(t, "the mouse modes", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(out.String(), "\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1006h")
	})
}

// TestHelperProcess asks for the mouse for TestConPTYForwardsMouse.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("MYGO_PTY_HELPER") != "mouse" {
		return
	}
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1006h")
	time.Sleep(time.Second)
}
