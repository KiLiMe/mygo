//go:build windows && !amd64 && !arm64

package pty

// carriedConPTY returns nothing: this package carries the ConPTY of Windows
// Terminal for amd64 and arm64 only, and the others run in Windows' own.
func carriedConPTY() (conPTY, bool) { return conPTY{}, false }
