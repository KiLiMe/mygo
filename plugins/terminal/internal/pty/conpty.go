package pty

// ConPTYInfo says which ConPTY terminals are made in.
type ConPTYInfo struct {
	// Path is the conpty.dll in use, empty when it is Windows' own in kernel32.
	Path string
	// Reason says why Windows' own is used, when it is.
	Reason string
}
