package primitives

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"io"
	"log/slog"
	"sort"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"gonum.org/v1/gonum/interp"
)

//go:embed processors.csv
var processorsCSV []byte

type Processor struct {
	Name    string
	Family  string
	Tdp     float64
	Cores   float64
	Threads float64
}

// Tdp constant ratio, we need to adjust this number when
// we enough data is collected
var tdpToPowerRatio float64 = 1.6
var processorNames []string
var processors []Processor

func init() {
	csvProcessors := csv.NewReader(bytes.NewReader(processorsCSV))
	csvProcessors.Read() // skip header line
	for {
		record, err := csvProcessors.Read()
		if err == io.EOF {
			break
		}
		must.NoError(err)

		processorNames = append(processorNames, record[0])
		processors = append(processors, Processor{
			Name:    record[0],
			Family:  record[1],
			Tdp:     must.CastFloat64(record[2]),
			Cores:   must.CastFloat64(record[3]),
			Threads: must.CastFloat64(record[4]),
		})
	}
}

func tdpToPower(tdp float64, cpuUsage float64) (watts float64) {
	// Extracted from Boavista API. TDP divider relative to CPU load
	var tdpDivider interp.FritschButland
	tdpDivider.Fit(
		[]float64{0.0, 10., 50., 100},
		[]float64{8.5, 3.2, 1.4, 1.0},
	)

	return (tdp / tdpDivider.Predict(cpuUsage)) * tdpToPowerRatio
}

func (p Processor) EstimatePowerUsageWithTDP(activeThreads float64, usage float64) (watts float64) {
	return tdpToPower(p.Tdp, usage) / p.Threads
}

func FuzzyFindBestProcessor(processorName string) Processor {
	ranks := fuzzy.RankFindNormalizedFold(processorName, processorNames)
	if len(ranks) == 0 {
		slog.Warn("no processor found in fuzzy finding")
		return processors[0]
	}

	sort.Sort(ranks)

	slog.Debug("fuzzy found a close processor", "source", processorName, "found", ranks[0].Target, "distance", ranks[0].Distance)

	return processors[ranks[0].OriginalIndex]
}
