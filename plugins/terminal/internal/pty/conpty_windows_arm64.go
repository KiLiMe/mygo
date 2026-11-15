//go:build windows && arm64

package pty

import _ "embed"

// The ConPTY of Windows Terminal, as conpty_windows_amd64.go carries it.
//
//go:embed conpty/arm64/conpty.dll
var conptyDLL []byte

//go:embed conpty/arm64/OpenConsole.exe
var openConsoleEXE []byte

func carriedConPTY() (conPTY, bool) {
	return unpackedConPTY("1.25.260930003-arm64", map[string][]byte{
		"conpty.dll":      conptyDLL,
		"OpenConsole.exe": openConsoleEXE,
	})
}
