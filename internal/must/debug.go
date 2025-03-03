package must

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

func PrintDebugJSON(a any) {
	jsn, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		slog.Error("failed to print debug json", "err", err)
		return
	}

	fmt.Println(string(jsn))
}
