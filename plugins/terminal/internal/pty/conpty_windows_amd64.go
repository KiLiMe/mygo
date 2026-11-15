//go:build windows && amd64

package pty

import _ "embed"

// The ConPTY of Windows Terminal: conpty.dll and the OpenConsole.exe it runs,
// of the Microsoft.Windows.Console.ConPTY package 1.25.260930003, MIT licensed
// (see conpty/LICENSE.txt). Windows' own console, in kernel32, hands a program
// neither the mouse modes it asks for before the console reads its input as
// virtual terminal input, nor what the terminal writes back, the answers to the
// questions programs ask about the colors; this one passes both on.
//
//go:embed conpty/amd64/conpty.dll
var conptyDLL []byte

//go:embed conpty/amd64/OpenConsole.exe
var openConsoleEXE []byte

func carriedConPTY() (conPTY, bool) {
	return unpackedConPTY("1.25.260930003-amd64", map[string][]byte{
		"conpty.dll":      conptyDLL,
		"OpenConsole.exe": openConsoleEXE,
	})
}
