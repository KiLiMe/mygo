package pty

// KeepsReplies reports whether the console between the terminal and the program
// keeps to itself what the terminal writes into the program's input. Windows'
// own ConPTY does: it answers a query itself, and holds or drops replies, or
// hands the program their tails, as typing. The ConPTY this package carries
// hands them on as they are.
func KeepsReplies() bool { return conpty().create == systemConPTY.create }
