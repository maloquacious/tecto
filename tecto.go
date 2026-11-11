// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tecto

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/hexmap"
	"github.com/maloquacious/tecto/internal/plates"
	"github.com/maloquacious/tecto/internal/sphere"
)

// The defaults for Params.
const (
	DefaultDetail  = 6
	DefaultColumns = 1920
	DefaultRows    = 800
)

// ErrInvalidParams is returned, wrapped, when Generate is given parameters
// it cannot use.
var ErrInvalidParams = errors.New("tecto: invalid parameters")

// Params are the inputs to Generate. A zero Detail, Columns, or Rows means
// its default.
type Params struct {
	// Seed chooses the world. The same parameters always give the same
	// result.
	Seed uint64
	// Myr is how long the plates run, in millions of years. Zero gives the
	// starting crust.
	Myr float64
	// Detail sets the size of the model's cells: the sphere has
	// 10·4^Detail + 2 of them. At 6 they are about 120 km apart.
	Detail int
	// Columns and Rows are the size of the hex map. Columns must be even,
	// so the map wraps east to west.
	Columns int
	Rows    int
}

// Result is a generated world.
type Result struct {
	Columns, Rows int
	// Heights has one height for each hex, in metres above the model's sea
	// level. The hexes are flat-topped, in odd-q offset coordinates, and
	// indexed by row*Columns + column. Row 0 is the north edge, and the map
	// wraps east to west. The largest landmass is in the middle of the map.
	Heights []float32
	Summary Summary
}

// Summary describes a run. Its figures are taken over the model's cells,
// not the hexes, so the poles count for no more than their share.
type Summary struct {
	// Cells is the number of cells on the sphere.
	Cells int
	// LandFraction is the share of the sphere above the model's sea level.
	LandFraction float64
	// Landmasses is the number of separate bodies of land.
	Landmasses int
	// The spread of height, in metres above sea level: the lowest cell,
	// the highest, and the heights below which 5%, 25%, 50%, 75%, and 95%
	// of the cells lie.
	Min, P5, P25, P50, P75, P95, Max float64
	// Plates is the number of plates the crust was divided into. It is
	// zero when the plates were not run.
	Plates int
	// GapFraction is the share of the sphere that no plate covered at the
	// end of the run, and OverlapFraction the share that more than one
	// did. The model does not yet make new crust where plates part, nor
	// consume it where they meet, so both grow as the plates run.
	GapFraction, OverlapFraction float64
}

func (p Params) withDefaults() (Params, error) {
	p.Detail = cmp.Or(p.Detail, DefaultDetail)
	p.Columns = cmp.Or(p.Columns, DefaultColumns)
	p.Rows = cmp.Or(p.Rows, DefaultRows)
	switch {
	case p.Detail < sphere.MinDetail || p.Detail > sphere.MaxDetail:
		return p, fmt.Errorf("%w: detail %d: want %d to %d", ErrInvalidParams, p.Detail, sphere.MinDetail, sphere.MaxDetail)
	case p.Columns < 2 || p.Columns > 16384:
		return p, fmt.Errorf("%w: %d columns: want 2 to 16384", ErrInvalidParams, p.Columns)
	case p.Columns%2 != 0:
		return p, fmt.Errorf("%w: %d columns: want an even number, so the map wraps east to west", ErrInvalidParams, p.Columns)
	case p.Rows < 1 || p.Rows > 16384:
		return p, fmt.Errorf("%w: %d rows: want 1 to 16384", ErrInvalidParams, p.Rows)
	case math.IsNaN(p.Myr) || math.IsInf(p.Myr, 0) || p.Myr < 0:
		return p, fmt.Errorf("%w: %v million years: want zero or more", ErrInvalidParams, p.Myr)
	}
	return p, nil
}

// Each use of random numbers has its own stream, so that changing how one
// part of the model draws its numbers leaves the others as they were.
const (
	streamLayout uint64 = iota + 1
	streamElevations
)

func stream(seed, id uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, id)) }

// Generate builds the world for p. It reads and writes nothing, and the
// same parameters always give the same result.
func Generate(p Params) (*Result, error) {
	p, err := p.withDefaults()
	if err != nil {
		return nil, err
	}
	g, err := sphere.NewGrid(p.Detail)
	if err != nil {
		return nil, err
	}

	columns := crust.Initial(g.Pos, stream(p.Seed, streamLayout), stream(p.Seed, streamElevations))
	if err := crust.Check(columns); err != nil {
		return nil, err
	}

	var summary Summary
	if p.Myr > 0 {
		world := plates.New(g, columns)
		if err := world.Advance(p.Myr); err != nil {
			return nil, err
		}
		merged := world.Merge()
		columns = merged.Columns
		if err := crust.Check(columns); err != nil {
			return nil, err
		}
		summary.Plates = len(world.Plates)
		summary.GapFraction = merged.Gap
		summary.OverlapFraction = merged.Overlap
	}

	height := crust.Elevations(columns)
	sea, err := crust.SeaLevel(height, g.Area, crust.OceanMeanDepth)
	if err != nil {
		return nil, err
	}
	for i := range height {
		height[i] -= sea
	}

	center, landmasses := largestLandmass(g, height)
	frame := hexmap.FrameAt(center)
	return &Result{
		Columns: p.Columns,
		Rows:    p.Rows,
		Heights: hexmap.Sample(g, height, frame, p.Columns, p.Rows),
		Summary: summarize(g, height, landmasses, summary),
	}, nil
}

// largestLandmass returns the middle of the largest connected body of
// cells above sea level, and the number of such bodies. With no land, or
// with land spread so evenly that it has no middle, it returns the point
// where the sphere's own equator meets its prime meridian.
func largestLandmass(g *sphere.Grid, height []float64) (center sphere.Vec3, count int) {
	label, count := g.Components(func(cell int32) bool { return height[cell] > 0 })
	area := make([]float64, count+1)
	sum := make([]sphere.Vec3, count+1)
	for cell, l := range label {
		if l != 0 {
			area[l] += g.Area[cell]
			sum[l] = sum[l].Add(g.Pos[cell].Scale(g.Area[cell]))
		}
	}
	// The first of equals wins.
	best := 0
	for l := 1; l <= count; l++ {
		if area[l] > area[best] {
			best = l
		}
	}
	if best == 0 || sum[best].Len() < 1e-9 {
		return sphere.Vec3{X: 1}, count
	}
	return sum[best].Normalize(), count
}

// summarize fills in the figures of s that come from the heights.
func summarize(g *sphere.Grid, height []float64, landmasses int, s Summary) Summary {
	var land float64
	for cell, h := range height {
		if h > 0 {
			land += g.Area[cell]
		}
	}
	sorted := slices.Sorted(slices.Values(height))
	at := func(share float64) float64 {
		return sorted[int(share*float64(len(sorted)-1))]
	}
	s.Cells = len(height)
	s.LandFraction = land / (4 * math.Pi)
	s.Landmasses = landmasses
	s.Min, s.Max = sorted[0], sorted[len(sorted)-1]
	s.P5, s.P25, s.P50, s.P75, s.P95 = at(0.05), at(0.25), at(0.50), at(0.75), at(0.95)
	return s
}
