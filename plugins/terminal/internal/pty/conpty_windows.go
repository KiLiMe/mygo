package pty

import (
	"crypto/sha256"
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

	// What this console does with what the terminal writes into a program's
	// input, and with the mouse modes a program asks for before that input
	// reads as virtual terminal input. Written down once per console so that
	// the terminal and the pty follow from these two rather than from which
	// console it is: Windows' own may learn to pass replies without everything
	// above it having to be told again.
	passesReplies bool
	needsVTInput  bool
}

var systemConPTY = conPTY{
	create: procCreatePseudoConsole,
	resize: procResizePseudoConsole,
	close:  procClosePseudoConsole,
	// It answers a query itself, and holds the replies or hands the program
	// their tails as typing; and it forwards the mouse a program asks for only
	// while its input already reads as virtual terminal input.
	needsVTInput: true,
}

// own reports whether this is Windows' own console, the one in kernel32: the
// last to fall back to, never tried twice.
func (c conPTY) own() bool { return c.create == systemConPTY.create }

// PassesReplies reports whether the console between the terminal and the
// program hands the program what the terminal writes into its input. Windows'
// own ConPTY does not: it answers a query itself, and holds or drops replies,
// or hands the program their tails, as typing. The ConPTY this package carries
// hands them on as they are.
func PassesReplies() bool { return conpty().passesReplies }

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
		// Windows Terminal's console passes on what a program asks for and
		// what the terminal answers it, whatever the console's input mode.
		passesReplies: true,
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
// version, and returns it.
func unpackedConPTY(version string, files map[string][]byte) (conPTY, bool) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return conPTY{}, false
	}
	dir := filepath.Join(cache, "mygo", "conpty", version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return conPTY{}, false
	}
	if err := writeFiles(dir, files); err != nil {
		return conPTY{}, false
	}
	return bundledConPTY(dir)
}

// writeFiles writes what dir does not already hold: a file of the same bytes is
// left alone, and one that is missing or holds anything else is written beside
// its name and renamed onto it, so that a console never loads half a file.
//
// The bytes are compared, not their size: the directory is the user's, and a
// conpty.dll replaced there is loaded as code.
func writeFiles(dir string, files map[string][]byte) error {
	for name, data := range files {
		path := filepath.Join(dir, name)
		if have, err := os.ReadFile(path); err == nil && sha256.Sum256(have) == sha256.Sum256(data) {
			continue
		}
		if err := writeAtomically(path, data); err != nil {
			return err
		}
	}
	return nil
}

// writeAtomically writes data to a file beside path, then renames it onto path.
func writeAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // nothing left once the rename took the file
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// A console's dll and its exe are 0755; CreateTemp makes 0600.
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
