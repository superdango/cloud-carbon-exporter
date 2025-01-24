package must

import (
	"fmt"
	"log/slog"
	"os"
)

func Assert(cond bool, failMessage string) {
	if !cond {
		slog.Error(failMessage)
		os.Exit(1)
	}
}

func Fail(message string) {
	Assert(false, fmt.Sprintf("assertion failed: %s", message))
}
