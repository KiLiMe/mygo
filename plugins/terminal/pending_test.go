package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A keybind takes its key's press and release. The release is what tells the
// view the key went up; where it never arrives — an input method takes it on
// Linux — the key stays pending, and the text that follows, which comes with
// no press of its own there, is the next key's, not the binding's.
func TestBindingKeyWithoutRelease(t *testing.T) {
	loadLib(t)
	p := newPipe()
	term := shell(t, Options{Conn: p})
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 480, 240)
	tt.Frame()
	term.Feed([]byte("\x1b[?2004h"))
	tt.SetClipboard("hello")

	tt.KeyRepeat(ui.Shift, ui.KeyInsert) // pressed, never released
	if got, want := p.take(term), "\x1b[200~hello\x1b[201~"; got != want {
		t.Fatalf("paste = %q, want %q", got, want)
	}

	// One text event a letter, as an input method sends them there: the
	// pending key goes with the first.
	for _, r := range "ss" {
		tt.Type(string(r))
	}
	if got, want := p.take(term), "ss"; got != want {
		t.Errorf("typed %q, want %q", got, want)
	}
}
