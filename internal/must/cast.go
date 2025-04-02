package must

import (
	"log/slog"
	"strconv"
)

func CastFloat64(n string) float64 {
	slog.Debug(n)
	f, err := strconv.ParseFloat(n, 64)
	NoError(err)
	return f
}
