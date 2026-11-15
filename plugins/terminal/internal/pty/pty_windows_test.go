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
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
)

// useConPTY makes the tests that follow run in the ConPTY MYGO_CONPTY names,
// "system" being Windows' own.
func useConPTY(t *testing.T, dir string) {
	t.Setenv("MYGO_CONPTY", dir)
	t.Cleanup(func() {
		conptyOnce = &sync.Once{}
		conptyChosen = conPTY{}
	})
	conptyOnce = &sync.Once{}
	conptyChosen = conPTY{}
}

// TestConPTYClearScreen clears the screen of PowerShell, whose prompt sat
// on the console's last row: the console repaints it on the first, which
// PSReadLine's redraws then keep to. The ConPTY this package carries
// repaints nothing on a clear, so this asks for Windows' own.
func TestConPTYClearScreen(t *testing.T) {
	useConPTY(t, "system")
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

// TestHelperProcess asks for the mouse for TestConPTYForwardsMouse, and what a
// terminal's background is for TestConPTYPassesTheReply.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv("MYGO_PTY_HELPER") {
	case "mouse":
		// A program asks for the mouse once it draws its first frame, well after
		// the console gave it a mode; the bit is set here a few milliseconds in.
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1006h")
		time.Sleep(time.Second)
	case "reply":
		// pi asks its console for the colors, and reads the terminal's answer
		// as input: it reads as virtual terminal input, as bun makes it.
		var mode uint32
		in := syscall.Handle(os.Stdin.Fd())
		if syscall.GetConsoleMode(in, &mode) == nil {
			procSetConsoleMode.Call(uintptr(in), uintptr((mode&^0x0007)|0x0080|enableVirtualTerminalInput))
		}
		fmt.Fprint(os.Stdout, "\x1b]11;?\x07")
		buf := make([]byte, 256)
		for range 100 {
			n, _ := os.Stdin.Read(buf)
			if n > 0 {
				fmt.Fprintf(os.Stdout, "heard:%q", buf[:n])
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(time.Second)
	}
}

// TestConPTYPassesTheReply has a program ask for the background color the way
// pi does, and answers as a terminal does. Windows' own console keeps the
// answer to itself, the program hearing nothing; the ConPTY this package
// carries passes it on.
func TestConPTYPassesTheReply(t *testing.T) {
	if conpty().own() {
		t.Skip("this build runs in Windows' own console, which drops what the terminal writes")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip("the test executable is not known")
	}
	p, err := Start(Options{
		Path: exe,
		Args: []string{exe, "-test.run=TestHelperProcess"},
		Env:  append(os.Environ(), "MYGO_PTY_HELPER=reply"),
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
	since := func(n int) string {
		mu.Lock()
		defer mu.Unlock()
		return out.String()[n:]
	}
	wait.Until(t, "the question", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(out.String(), "\x1b]11;?\x07")
	})
	mu.Lock()
	n := out.Len()
	mu.Unlock()
	if _, err := p.Write([]byte("\x1b]11;rgb:0c0c/0c0c/0c0c\x07")); err != nil {
		t.Fatal(err)
	}
	wait.Until(t, "the program reading the answer", func() bool {
		return strings.Contains(since(n), "heard:\"\\x1b]11;rgb:0c0c/0c0c/0c0c\\a\"")
	})
}
