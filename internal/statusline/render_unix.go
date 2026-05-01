//go:build !windows

package statusline

import (
	"os"

	"golang.org/x/term"
)

// widthFromControllingTTY abre /dev/tty (mesmo truque do `tput cols`) pra
// pegar largura quando o subprocess nao tem TTY nos fds padrao.
func widthFromControllingTTY() int {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return 0
	}
	defer tty.Close()
	if w, _, err := term.GetSize(int(tty.Fd())); err == nil && w > 0 {
		return w
	}
	return 0
}
