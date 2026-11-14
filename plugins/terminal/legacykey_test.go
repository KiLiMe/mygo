package terminal

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

// A ConPTY keeps only what it can turn into keys, so the keys the Kitty
// keyboard protocol encodes as CSI u have to go as their plain forms instead:
// the arrows in both cursor key modes, Home, End, Shift+Tab and Ctrl+letter.
func TestLegacyKey(t *testing.T) {

	cases := []struct {
		name string
		key  vt.Key
		mods vt.Mods
		want string
	}{
		{"up", vt.KeyArrowUp, 0, "\x1b[A"},
		{"down", vt.KeyArrowDown, 0, "\x1b[B"},
		{"right", vt.KeyArrowRight, 0, "\x1b[C"},
		{"left", vt.KeyArrowLeft, 0, "\x1b[D"},
		{"home", vt.KeyHome, 0, "\x1b[H"},
		{"end", vt.KeyEnd, 0, "\x1b[F"},
		{"pageup", vt.KeyPageUp, 0, "\x1b[5~"},
		{"insert", vt.KeyInsert, 0, "\x1b[2~"},
		{"ctrl c", vt.KeyC, vt.ModCtrl, "\x03"},
		{"ctrl u", vt.KeyU, vt.ModCtrl, "\x15"},
		{"ctrl shift c", vt.KeyC, vt.ModCtrl | vt.ModShift, "\x03"},
		{"shift tab", vt.KeyTab, vt.ModShift, "\x1b[Z"},
	}
	for _, c := range cases {
		if got := legacyKey(nil, vt.KeyEvent{Action: vt.KeyPress, Key: c.key, Mods: c.mods}); string(got) != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The arrows read as ESC O once the program asked for application cursor keys,
// and Home and End with them, as xterm sends them.
func TestLegacyKeyApplicationCursorKeys(t *testing.T) {
	loadLib(t)
	term, err := vt.NewTerminal(80, 24, vt.Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	term.SetMode(vt.ModeCursorKeys, true)

	cases := []struct {
		key  vt.Key
		want string
	}{
		{vt.KeyArrowUp, "\x1bOA"},
		{vt.KeyArrowDown, "\x1bOB"},
		{vt.KeyArrowRight, "\x1bOC"},
		{vt.KeyArrowLeft, "\x1bOD"},
		{vt.KeyHome, "\x1bOH"},
		{vt.KeyEnd, "\x1bOF"},
		// The rest keep their forms.
		{vt.KeyPageUp, "\x1b[5~"},
	}
	for _, c := range cases {
		if got := legacyKey(term, vt.KeyEvent{Action: vt.KeyPress, Key: c.key}); string(got) != c.want {
			t.Errorf("%v: got %q, want %q", c.key, got, c.want)
		}
	}
}

// Nil leaves the protocol's own form standing: typing, releases and the
// combinations the plain forms cannot express.
func TestLegacyKeyLeavesTheRest(t *testing.T) {
	for _, k := range []vt.KeyEvent{
		{Action: vt.KeyPress, Key: vt.KeyA, Text: "a"},
		{Action: vt.KeyRelease, Key: vt.KeyArrowUp},
		{Action: vt.KeyPress, Key: vt.KeyA, Mods: vt.ModAlt},
		{Action: vt.KeyPress, Key: vt.KeyA, Mods: vt.ModCtrl | vt.ModAlt},
		{Action: vt.KeyPress, Key: vt.KeyArrowUp, Mods: vt.ModCtrl},
		{Action: vt.KeyPress, Key: vt.KeyEnter, Mods: vt.ModShift},
		{Action: vt.KeyPress, Key: vt.KeyTab},
	} {
		if got := legacyKey(nil, k); got != nil {
			t.Errorf("%+v: got %q, want nil", k, got)
		}
	}
}
