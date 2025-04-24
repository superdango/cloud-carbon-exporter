package primitives

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/superdango/cloud-carbon-exporter/internal/must"
	"gonum.org/v1/gonum/interp"
)

//go:embed data/processors/processors.csv
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
var processorFullNames []string
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

		processorFullNames = append(processorFullNames, record[0]+" "+record[1])
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
		[]float64{0.0, 10., 50., 100.},
		[]float64{12., 33., 75., 102.},
	)

	return tdp * (tdpDivider.Predict(cpuUsage) / 100) * tdpToPowerRatio
}

func (p Processor) EstimatePowerUsageWithTDP(activeThreads float64, usage float64) (watts float64) {
	return tdpToPower(p.Tdp, usage) / p.Threads
}

// LookupProcessorByName fuzzy find the best suitable processor in the internal database
func LookupProcessorByName(processorName string) Processor {
	var bestProcessorIndex = 0
	for _, submatch := range submatches(processorName) {
		ranks := fuzzy.RankFindNormalizedFold(submatch, processorFullNames)
		if len(ranks) == 0 {
			continue
		}
		sort.Sort(ranks)
		bestProcessorIndex = ranks[0].OriginalIndex
	}

	slog.Debug("fuzzy found the closest processor", "source", processorName, "match", processors[bestProcessorIndex].Name)

	return processors[bestProcessorIndex]
}

// submatches splits string into subcomponents from small to entier string
// to help fuzzy matching finding the best option. For example, passing the
// string: "foo bar baz" returns {"foo", "foo bar", "foo bar baz", "bar", "baz"}
func submatches(s string) []string {
	splited := strings.Split(s, " ")
	submatches := make([]string, 0)
	for i, substr := range splited {
		if i > 0 {
			submatches = append(submatches, submatches[i-1]+" "+substr)
			continue
		}
		submatches = append(submatches, substr)
	}
	submatches = append(submatches, splited[1:]...)
	return submatches
}
