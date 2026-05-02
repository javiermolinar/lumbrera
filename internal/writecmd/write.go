package writecmd

import (
	"fmt"
	"io"
)

func Run(args []string, stdin io.Reader) error {
	_ = args
	_ = stdin
	return fmt.Errorf("lumbrera write is not implemented yet")
}
