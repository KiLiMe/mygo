package pty

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32                              = syscall.NewLazyDLL("kernel32.dll") // a KnownDLL
	procCreatePseudoConsole               = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole               = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                = kernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
	procAttachConsole                     = kernel32.NewProc("AttachConsole")
	procFreeConsole                       = kernel32.NewProc("FreeConsole")
	procSetConsoleMode                    = kernel32.NewProc("SetConsoleMode")
	procRtlGetVersion                     = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion") // a KnownDLL
)

const (
	procThreadAttributePseudoConsole = 0x00020016
	extendedStartupInfoPresent       = 0x00080000
	createUnicodeEnvironment         = 0x00000400
	enableVirtualTerminalInput       = 0x0200
)

type startupInfoEx struct {
	syscall.StartupInfo
	attributes uintptr
}

type sys struct {
	mu      sync.Mutex
	conpty  conPTY  // the one start made this console in
	console uintptr // the HPCON, 0 once closed
	in      *os.File
	out     *os.File
	process syscall.Handle
	pid     int
}

// startConsole makes the console the terminal reads and writes through, and
// returns the ConPTY it was made in: the one this package carries answers
// nothing when the OpenConsole.exe beside it can't start there, and Windows'
// own console is then the next best thing.
func startConsole(cp conPTY, cols, rows int, in, out syscall.Handle) (uintptr, conPTY, error) {
	var console uintptr
	r, _, _ := cp.create.Call(coord(cols, rows), uintptr(in), uintptr(out), 0, uintptr(unsafe.Pointer(&console)))
	if r != 0 && !cp.own() {
		cp, console = systemConPTY, 0
		r, _, _ = cp.create.Call(coord(cols, rows), uintptr(in), uintptr(out), 0, uintptr(unsafe.Pointer(&console)))
	}
	if r != 0 {
		return 0, cp, syscall.Errno(r & 0xFFFF)
	}
	return console, cp, nil
}

func (p *PTY) start(o Options) error {
	cp := conpty()
	if err := cp.create.Find(); err != nil {
		return errors.New("pty: ConPTY needs Windows 10 1809 or later")
	}
	// The console reads what is typed from inR and writes the output to
	// outW; this side writes to inW and reads outR.
	var inR, inW, outR, outW syscall.Handle
	if err := syscall.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return fmt.Errorf("pty: %w", err)
	}
	if err := syscall.CreatePipe(&outR, &outW, nil, 0); err != nil {
		syscall.CloseHandle(inR)
		syscall.CloseHandle(inW)
		return fmt.Errorf("pty: %w", err)
	}
	console, cp, err := startConsole(cp, o.Cols, o.Rows, inR, outW)
	// The console has its own copies of its ends.
	syscall.CloseHandle(inR)
	syscall.CloseHandle(outW)
	if err != nil {
		syscall.CloseHandle(inW)
		syscall.CloseHandle(outR)
		return fmt.Errorf("pty: CreatePseudoConsole: %w", err)
	}
	p.conpty = cp
	p.console = console
	p.in, p.out = os.NewFile(uintptr(inW), "conpty-in"), os.NewFile(uintptr(outR), "conpty-out")
	if err := p.spawn(o); err != nil {
		p.Close()
		p.out.Close()
		return err
	}
	return nil
}

func (p *PTY) spawn(o Options) error {
	var size uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	list := make([]byte, size)
	if r, _, err := procInitializeProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&list[0])), 1, 0, uintptr(unsafe.Pointer(&size))); r == 0 {
		return fmt.Errorf("pty: InitializeProcThreadAttributeList: %w", err)
	}
	defer procDeleteProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&list[0])))
	if r, _, err := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(&list[0])), 0, procThreadAttributePseudoConsole, p.console, unsafe.Sizeof(p.console), 0, 0); r == 0 {
		return fmt.Errorf("pty: UpdateProcThreadAttribute: %w", err)
	}
	si := startupInfoEx{attributes: uintptr(unsafe.Pointer(&list[0]))}
	si.Cb = uint32(unsafe.Sizeof(si))
	// The console's own standard handles: without invalid ones, a child of
	// a process whose output is redirected would write there instead.
	si.Flags = syscall.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = syscall.InvalidHandle, syscall.InvalidHandle, syscall.InvalidHandle

	cmdline, err := syscall.UTF16PtrFromString(commandLine(o.Path, o.Args))
	if err != nil {
		return err
	}
	var dir *uint16
	if o.Dir != "" {
		if dir, err = syscall.UTF16PtrFromString(o.Dir); err != nil {
			return err
		}
	}
	var env *uint16
	if o.Env != nil {
		block := environmentBlock(o.Env)
		env = &block[0]
	}
	var pi syscall.ProcessInformation
	flags := uint32(extendedStartupInfoPresent | createUnicodeEnvironment)
	if err := syscall.CreateProcess(nil, cmdline, nil, nil, false, flags, env, dir, &si.StartupInfo, &pi); err != nil {
		return fmt.Errorf("pty: starting %s: %w", o.Path, err)
	}
	syscall.CloseHandle(pi.Thread)
	p.process, p.pid = pi.Process, int(pi.ProcessId)
	if p.conpty.own() {
		enableVTInput(p.pid)
	}
	return nil
}

// enableVTInput turns on virtual terminal input in the console the program just
// got, which Windows' own ConPTY needs: the one from Windows Terminal passes
// what a program asks for on its first frame as it is.
//
// conhost hands on the mouse modes a program asks for only while that bit is
// already on, and keeps them otherwise. pi asks for the mouse while bun has not
// turned the bit on yet, so its wheel arrived as alternate scroll, as arrow
// keys its prompt box takes for history. A Windows Terminal session starts with
// the bit on, a console made here starts without it. Best effort: a console
// that refuses loses nothing it had.
func enableVTInput(pid int) {
	// AttachConsole wants a process with no console of its own; this one may
	// have been started from a terminal. The console of the new process answers
	// only once it exists, which takes a few tries after CreateProcess.
	procFreeConsole.Call()
	attached := false
	for range 40 {
		if r, _, _ := procAttachConsole.Call(uintptr(pid)); r != 0 {
			attached = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !attached {
		return
	}
	defer procFreeConsole.Call()
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer in.Close()
	h := syscall.Handle(in.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil || mode&enableVirtualTerminalInput != 0 {
		return
	}
	procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalInput))
}

func (p *PTY) wait() (int, error) {
	if _, err := syscall.WaitForSingleObject(p.process, syscall.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	err := syscall.GetExitCodeProcess(p.process, &code)
	syscall.CloseHandle(p.process)
	return int(code), err
}

// Read reads the output of the process; it returns io.EOF after Close, once
// the console wrote what it had.
func (p *PTY) Read(b []byte) (int, error) { return p.out.Read(b) }

// Write types b into the terminal.
func (p *PTY) Write(b []byte) (int, error) {
	n, err := p.in.Write(b)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

// Resize sets the size of the terminal.
func (p *PTY) Resize(cols, rows, width, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return ErrClosed
	}
	if r, _, _ := p.conpty.resize.Call(p.console, coord(cols, rows)); r != 0 {
		return fmt.Errorf("pty: ResizePseudoConsole: %w", syscall.Errno(r&0xFFFF))
	}
	return nil
}

// ClearScreen tells the console that the terminal cleared its screen, as
// Windows Terminal's ClearPseudoConsole does: the console clears its
// buffer and repaints with the cursor's line at the top. Without it, the
// console keeps the prompt on its old row and, as it redraws with
// absolute cursor moves (PSReadLine's predictions), takes the cursor back
// down there. The ConPTY this package carries clears without repainting,
// and the terminal draws its own screen again anyway; Windows' own console
// does nothing before Windows 11 24H2, whose console may shut down on a
// signal it doesn't know, or once the console is closed.
func (p *PTY) ClearScreen() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return nil
	}
	if clear := p.conpty.clear; clear != nil {
		if r, _, _ := clear.Call(p.console); r != 0 {
			return fmt.Errorf("pty: ClearPseudoConsole: %w", syscall.Errno(r&0xFFFF))
		}
		return nil
	}
	if windowsBuild() < 26100 {
		return nil
	}
	// kernel32 doesn't export ClearPseudoConsole. The HPCON points at the
	// console's handles, first the pipe of its signals, which take a
	// 16-bit code: PTY_SIGNAL_CLEAR_WINDOW is 2.
	signals := **(**syscall.Handle)(unsafe.Pointer(&p.console))
	var n uint32
	if err := syscall.WriteFile(signals, []byte{2, 0}, &n, nil); err != nil {
		return fmt.Errorf("pty: clearing the console: %w", err)
	}
	return nil
}

// windowsBuild returns the build of Windows 10 or 11, the maximum on a
// later major version. RtlGetVersion tells the version whatever the
// executable's manifest declares.
var windowsBuild = sync.OnceValue(func() uint32 {
	var v struct {
		size, major, minor, build, platform uint32
		servicePack                         [128]uint16
	}
	v.size = uint32(unsafe.Sizeof(v))
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	switch {
	case v.major > 10:
		return ^uint32(0)
	case v.major < 10:
		return 0
	}
	return v.build
})

// Pid returns the process's ID.
func (p *PTY) Pid() int { return p.pid }

// Foreground returns 0: a console has no process groups in its foreground.
func (p *PTY) Foreground() int { return 0 }

// Close closes the console, which ends the processes attached to it. Read
// returns io.EOF once it wrote what it had.
func (p *PTY) Close() error {
	p.mu.Lock()
	console := p.console
	p.console = 0
	p.mu.Unlock()
	if console == 0 {
		return nil
	}
	p.in.Close()
	// ClosePseudoConsole waits for the output to be read on Windows
	// before 11 24H2: Read keeps reading meanwhile, until the end of the
	// output, then the pipe goes.
	closeConsole := p.conpty.close
	go func() {
		closeConsole.Call(console)
		p.out.Close()
	}()
	return nil
}

// Kill kills the process.
func (p *PTY) Kill() error { return syscall.TerminateProcess(p.process, 1) }

// coord packs a COORD, which ConPTY takes by value.
func coord(cols, rows int) uintptr {
	return uintptr(uint16(max(1, min(cols, 0x7FFF)))) | uintptr(uint16(max(1, min(rows, 0x7FFF))))<<16
}

// commandLine joins a program and its arguments as the C runtime splits
// them; args[0] is the program's name.
func commandLine(path string, args []string) string {
	parts := []string{syscall.EscapeArg(path)}
	if len(args) > 1 {
		for _, a := range args[1:] {
			parts = append(parts, syscall.EscapeArg(a))
		}
	}
	return strings.Join(parts, " ")
}

// environmentBlock encodes env as CreateProcess takes it: NUL-terminated
// UTF-16 strings, then another NUL.
func environmentBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}
