package must

import (
	"fmt"
)

func Assert(cond bool, failMessage string) {
	if !cond {
		panic(failMessage)
	}
}

func Fail(message string) {
	Assert(false, message)
}

func NoError(err error) {
	Assert(err == nil, fmt.Sprintf("error catched: %s", err))
}
