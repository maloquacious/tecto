// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"fmt"
	"math"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

const (
	// MaxStepMyr is the longest step the model takes, in millions of
	// years, so that crust does not age far between one reckoning of the
	// plates' speeds and the next.
	MaxStepMyr = 5.0
	// stepCells is the furthest the fastest plate may move in a step, in
	// cell spacings.
	stepCells = 0.5
)

// World is the plates of a world, on one grid.
type World struct {
	Grid   *sphere.Grid
	Plates []*Plate
	// Steps is the number of steps taken so far.
	Steps int
	// Fastest is the highest rate any plate has turned at, in radians per
	// million years. No part of a plate has moved faster than this.
	Fastest float64
}

// New divides the crust into plates.
func New(g *sphere.Grid, columns []crust.Column) *World {
	owner, count := Segment(g, columns)
	w := &World{Grid: g}
	for id := range int32(count) {
		member := make([]bool, len(owner))
		for cell, o := range owner {
			member[cell] = o == id
		}
		w.Plates = append(w.Plates, newPlate(g, member, columns))
	}
	return w
}

// Advance moves the plates for a time, in millions of years. It chooses its
// own steps: short enough that the fastest plate moves less than a cell in
// each.
func (w *World) Advance(myr float64) error {
	rates := make([]sphere.Vec3, len(w.Plates))
	for myr > 0 {
		var fastest float64
		for i, p := range w.Plates {
			rates[i] = p.AngularVelocity(w.Grid)
			fastest = max(fastest, rates[i].Len())
		}
		if math.IsNaN(fastest) || math.IsInf(fastest, 0) {
			return fmt.Errorf("plates: step %d: a plate's speed is not finite", w.Steps)
		}
		w.Fastest = max(w.Fastest, fastest)
		step := min(myr, MaxStepMyr)
		if fastest > 0 {
			step = min(step, stepCells*w.Grid.Spacing()/fastest)
		}
		for i, p := range w.Plates {
			p.turn(rates[i], step)
		}
		// The last step takes all that is left, so the loop ends whatever
		// rounding does to the subtraction.
		if step == myr {
			myr = 0
		} else {
			myr -= step
		}
		w.Steps++
	}
	return nil
}

// Merged is the crust of the world, put together from its plates.
type Merged struct {
	// Columns is the rock under each cell of the world.
	Columns []crust.Column
	// Top is the plate whose crust is uppermost at each cell, or -1 where
	// there is no plate.
	Top []int32
	// Count is the number of plates over each cell.
	Count []int32
	// Gap is the share of the sphere that no plate covers, and Overlap the
	// share that more than one does.
	Gap, Overlap float64
}

// Merge puts the world's crust together from the plates, as they lie now.
//
// Where plates overlap, the least dense is uppermost. The ocean floor and
// the age of the crust are the uppermost plate's, and the lower plate's
// ocean floor is taken to have gone down into the mantle; continental rock
// from every plate there is stacked. Where the plates have parted and none
// covers a cell, the cell is given new ocean crust.
func (w *World) Merge() *Merged {
	g := w.Grid
	n := len(g.Pos)
	m := &Merged{
		Columns: make([]crust.Column, n),
		Top:     make([]int32, n),
		Count:   make([]int32, n),
	}
	// For each plate, where its middle is in the world, and the rotation
	// from the world back to its grid.
	centers := make([]sphere.Vec3, len(w.Plates))
	back := make([]sphere.Mat3, len(w.Plates))
	for i, p := range w.Plates {
		centers[i] = p.Rot.Apply(p.center)
		back[i] = p.Rot.Transpose()
	}
	for cell, pos := range g.Pos {
		top, lightest := int32(-1), math.Inf(1)
		var column crust.Column
		for i, p := range w.Plates {
			if pos.Dot(centers[i]) < p.reach {
				continue
			}
			local := g.Nearest(back[i].Apply(pos))
			if !p.Member[local] {
				continue
			}
			c := p.Crust[local]
			m.Count[cell]++
			column.Sediment += c.Sediment
			column.Sedimentary += c.Sedimentary
			column.Metamorphic += c.Metamorphic
			column.FelsicPlutonic += c.FelsicPlutonic
			column.FelsicVolcanic += c.FelsicVolcanic
			if d := c.Density(); d < lightest {
				top, lightest = int32(i), d
				column.MaficVolcanic = c.MaficVolcanic
				column.MaficPlutonic = c.MaficPlutonic
				column.Age = c.Age
			}
		}
		m.Top[cell] = top
		switch m.Count[cell] {
		case 0:
			column = crust.NewOcean()
			m.Gap += g.Area[cell]
		case 1:
		default:
			m.Overlap += g.Area[cell]
		}
		m.Columns[cell] = column
	}
	m.Gap /= 4 * math.Pi
	m.Overlap /= 4 * math.Pi
	return m
}
