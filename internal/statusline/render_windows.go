//go:build windows

package statusline

import (
	"os"

	"golang.org/x/term"
)

// widthFromControllingTTY tenta CONIN$ (console input handle do Windows)
// como equivalente do /dev/tty: funciona mesmo quando o subprocess teve
// stdin/stdout/stderr redirecionados pra pipes.
func widthFromControllingTTY() int {
	conin, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer conin.Close()
	if w, _, err := term.GetSize(int(conin.Fd())); err == nil && w > 0 {
		return w
	}
	return 0
}
