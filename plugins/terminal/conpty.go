package terminal

import "github.com/egoist/mygo/plugins/terminal/internal/pty"

// ConPTYInfo says which ConPTY the terminals of this process are made in.
type ConPTYInfo struct {
	// Path is the conpty.dll in use, empty when it is Windows' own.
	Path string
	// Reason says why Windows' own is used, when it is.
	Reason string
}

// ConPTY reports which ConPTY the terminals of this process are made in, for a
// report that has to name it.
func ConPTY() ConPTYInfo {
	info := pty.ConPTY()
	return ConPTYInfo{Path: info.Path, Reason: info.Reason}
}
