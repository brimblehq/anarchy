package output

import (
	"fmt"

	"github.com/brimbleapp/brimble-cli/internal/ui"
)

func Fail(err error) {
	ui.Fail(err)
}

func Failf(format string, args ...any) {
	ui.Fail(fmt.Errorf(format, args...))
}
