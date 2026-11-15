package terminal

import "github.com/egoist/mygo/plugins/terminal/internal/pty"

// ConPTYInfo says which ConPTY the terminals of this process are made in.
type ConPTYInfo = pty.ConPTYInfo

// ConPTY reports which ConPTY the terminals of this process are made in, for a
// report that has to name it.
func ConPTY() ConPTYInfo { return pty.ConPTY() }
