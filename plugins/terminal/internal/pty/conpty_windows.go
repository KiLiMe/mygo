package pty

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// conPTY is a ConPTY's functions: Windows' own, in kernel32, or those of a
// conpty.dll the app ships beside itself, which runs the OpenConsole.exe
// beside it.
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
// or hands the program their tails, as typing. A ConPTY shipped beside the app
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

// chosen is the ConPTY terminals are made in, and what it is.
type chosen struct {
	console conPTY
	info    ConPTYInfo
}

var chosenConPTY = sync.OnceValue(chooseConPTY)

// ConPTY returns which ConPTY terminals are made in.
func ConPTY() ConPTYInfo { return chosenConPTY().info }

// conpty returns the ConPTY terminals are made in: the one the app ships beside
// itself, else Windows' own.
//
// Windows' own console keeps to itself what the terminal writes into a
// program's input while the console is not yet in virtual terminal input mode,
// and the mode comes after the program asked: pi asks for the colors and the
// mouse on its first frame, and heard neither, the wheel arriving as arrow
// keys. The console Windows Terminal runs passes both on whatever the mode, and
// an app that ships it gets it here.
func conpty() conPTY { return chosenConPTY().console }

// chooseConPTY takes the ConPTY of MYGO_CONPTY, then the one beside the
// executable, then Windows' own, and says which it took and why.
func chooseConPTY() chosen {
	named := os.Getenv("MYGO_CONPTY")
	if named == "system" {
		return chosen{systemConPTY, ConPTYInfo{Reason: "MYGO_CONPTY=system"}}
	}
	if named != "" {
		if c, ok := bundledConPTY(named); ok {
			return chosen{c, ConPTYInfo{Path: filepath.Join(named, "conpty.dll")}}
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return chosen{systemConPTY, ConPTYInfo{Reason: whyNot(named, "")}}
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	dir := filepath.Dir(exe)
	if c, ok := bundledConPTY(dir); ok {
		return chosen{c, ConPTYInfo{Path: filepath.Join(dir, "conpty.dll")}}
	}
	return chosen{systemConPTY, ConPTYInfo{Reason: whyNot(named, dir)}}
}

// whyNot says why Windows' own console is used: what MYGO_CONPTY named and
// could not load, before what is not beside the executable.
func whyNot(named, dir string) string {
	switch {
	case named != "":
		return "MYGO_CONPTY=" + named + " has no conpty.dll that loads"
	case dir == "":
		return "the executable's directory is unknown"
	}
	return "no conpty.dll that loads beside " + dir
}
