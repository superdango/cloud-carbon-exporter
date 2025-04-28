package cloudcarbonexporter

import (
	"context"
	"fmt"
	"io"
)

type Explorer interface {
	CollectMetrics(ctx context.Context, metrics chan *Metric, errors chan error)
	Init(ctx context.Context) error
	IsReady() bool
	SupportedServices() []string
	io.Closer
}

type ExplorerErr struct {
	Err       error
	Operation string
}

func (explorerErr *ExplorerErr) Error() string {
	return fmt.Sprintf("operation failed (op: %s): %s", explorerErr.Operation, explorerErr.Err.Error())
}

func (explorerErr *ExplorerErr) Unwrap() error {
	return explorerErr.Err
}

func MergeLabels(labels ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, l := range labels {
		for k, v := range l {
			if v == "" {
				continue
			}
			result[k] = v
		}
	}
	return result
}
