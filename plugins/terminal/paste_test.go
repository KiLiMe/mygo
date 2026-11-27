package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// GTK reads the clipboard in a nested event loop, which delivers the input
// that comes while it waits before the read returns: the release of the key
// that pastes, and a key pressed meanwhile. The release is what clears the
// pending key a keybind took, and the key pressed meanwhile is the key after
// the paste, so both wait for the paste instead of going in ahead of it.
func TestPasteDuringClipboardRead(t *testing.T) {
	loadLib(t)
	p := newPipe()
	term := shell(t, Options{Conn: p})
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 480, 240)
	tt.Frame()
	term.Feed([]byte("\x1b[?2004h\x1b[>7u"))
	tt.SetClipboard("hello")

	tt.ReadClipboardDuring(func() {
		tt.KeyUp(ui.KeyV) // the release of the key that pastes, before it is applied
		tt.KeyDown(ui.KeySpace)
		tt.Type(" ")
		tt.KeyUp(ui.KeySpace)
	})
	tt.KeyRepeat(ui.Ctrl|ui.Shift, ui.KeyV)

	if got, want := p.take(term), "\x1b[200~hello\x1b[201~ \x1b[32;1:3u"; got != want {
		t.Errorf("paste = %q, want %q", got, want)
	}
}
