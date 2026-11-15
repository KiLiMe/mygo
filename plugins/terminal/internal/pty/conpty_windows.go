package pty

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// conPTY is a ConPTY's functions: Windows' own, in kernel32, or those of the
// conpty.dll this package carries, which runs the OpenConsole.exe beside it.
type conPTY struct {
	create, resize, close *syscall.LazyProc
	clear                 *syscall.LazyProc // Windows' own has none before 11 24H2
}

var systemConPTY = conPTY{
	create: procCreatePseudoConsole,
	resize: procResizePseudoConsole,
	close:  procClosePseudoConsole,
}

// own reports whether this is Windows' own console, the one in kernel32.
func (c conPTY) own() bool { return c.create == systemConPTY.create }

// KeepsReplies reports whether the console between the terminal and the program
// keeps to itself what the terminal writes into the program's input. Windows'
// own ConPTY does: it answers a query itself, and holds or drops replies, or
// hands the program their tails, as typing. The ConPTY this package carries
// hands them on as they are.
func KeepsReplies() bool { return conpty().own() }

// bundledConPTY returns the ConPTY of the conpty.dll in dir, when it and the
// OpenConsole.exe it runs are there and it loads.
func bundledConPTY(dir string) (conPTY, bool) {
	if _, err := os.Stat(filepath.Join(dir, "OpenConsole.exe")); err != nil {
		return conPTY{}, false
	}
	dll := syscall.NewLazyDLL(filepath.Join(dir, "conpty.dll"))
	c := conPTY{
		create: dll.NewProc("CreatePseudoConsole"),
		resize: dll.NewProc("ResizePseudoConsole"),
		close:  dll.NewProc("ClosePseudoConsole"),
		clear:  dll.NewProc("ClearPseudoConsole"),
	}
	for _, p := range []*syscall.LazyProc{c.create, c.resize, c.close, c.clear} {
		if p.Find() != nil {
			return conPTY{}, false
		}
	}
	return c, true
}

// conpty returns the ConPTY terminals are made in: the one this package
// carries, else Windows' own.
//
// Windows' own console keeps to itself what the terminal writes into a
// program's input while the console is not yet in virtual terminal input mode,
// and the mode comes after the program asked: pi asks for the colors and the
// mouse on its first frame, and heard neither, the wheel arriving as arrow
// keys. The console Windows Terminal runs passes both on whatever the mode,
// and this is that one, carried.
func conpty() conPTY {
	conptyOnce.Do(func() { conptyChosen = chooseConPTY() })
	return conptyChosen
}

var (
	conptyOnce   = &sync.Once{}
	conptyChosen conPTY
)

func chooseConPTY() conPTY {
	if dir := os.Getenv("MYGO_CONPTY"); dir != "" {
		if dir == "system" {
			return systemConPTY
		}
		if c, ok := bundledConPTY(dir); ok {
			return c
		}
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if c, ok := bundledConPTY(filepath.Dir(exe)); ok {
			return c
		}
	}
	if c, ok := carriedConPTY(); ok {
		return c
	}
	return systemConPTY
}

// unpackedConPTY unpacks the ConPTY in files into the user's cache, once per
// version, writing only what is missing or of another size, and returns it.
func unpackedConPTY(version string, files map[string][]byte) (conPTY, bool) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return conPTY{}, false
	}
	dir := filepath.Join(cache, "mygo", "conpty", version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return conPTY{}, false
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
			continue
		}
		if err := os.WriteFile(path, data, 0o755); err != nil {
			return conPTY{}, false
		}
	}
	return bundledConPTY(dir)
}
