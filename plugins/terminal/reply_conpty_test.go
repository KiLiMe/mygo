package terminal

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/plugins/terminal/internal/pty"
)

// batch is what a program opening asks for: the colors, the palette's, and the
// device's attributes, answered in one write, as the library does.
var batch = []byte("\x1b]10;rgb:2020/2020/2020\x07\x1b]11;rgb:fafa/fafa/fafa\x07\x1b]4;1;rgb:cdcd/3131/3131\x1b\\\x1b[?62;22c\x1b[?997;2n")

// TestWriteReply pins what a ConPTY is written: Windows' own keeps the answers
// to itself, so nothing goes there; the ConPTY this package carries hands them
// on as they are, and a Conn has no console in between at all.
func TestWriteReply(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows puts a ConPTY between the terminal and the program")
	}
	direct := &Terminal{}
	direct.writeReply(batch)
	got := queuedReplies(direct)
	if !pty.PassesReplies() {
		if len(got) != 0 {
			t.Errorf("writeReply behind Windows' own ConPTY queued %q, want nothing", got)
		}
	} else if len(got) != 1 || string(got[0]) != string(batch) {
		t.Errorf("writeReply behind the ConPTY this package carries queued %q, want the batch in one write", got)
	}
	piped := &Terminal{opts: Options{Conn: newPipe()}}
	piped.writeReply(batch)
	if got := queuedReplies(piped); len(got) != 1 || string(got[0]) != string(batch) {
		t.Errorf("writeReply with a Conn queued %q, want the batch in one write", got)
	}
}

// queuedReplies returns what the terminal queued for the program.
func queuedReplies(t *Terminal) [][]byte {
	t.in.mu.Lock()
	defer t.in.mu.Unlock()
	out := make([][]byte, 0, len(t.in.queue))
	for _, q := range t.in.queue {
		out = append(out, q.p)
	}
	return out
}
