package model

import "log/slog"

type Primitives map[string][]float64

func (p Primitives) Linear(primitive string, percent float64) float64 {
	primitives, found := p[primitive]
	if !found {
		primitives = p["DEFAULT"]
	}

	slog.Debug("primitive linear calculation", "primitive", primitive, "values", primitives)
	if len(primitives) == 1 {
		return primitives[0]
	}

	min := primitives[0]
	max := primitives[1]

	return min + ((max - min) * percent / 100)
}
