// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/erosion"
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
	// CycleMyr is how long a set of plates lasts, in millions of years.
	// When it is over the plates are discarded and drawn afresh from the
	// crust as it then is. It is the length tectonics.js gives its
	// supercontinent cycle.
	CycleMyr = 150.0
)

// Budget is an account of the world's continental rock, in kilograms. The
// plates' rock is Start − Subducted + Accreted + Erupted + Resampled at any time.
type Budget struct {
	// Start is the continental rock the world began with.
	Start float64
	// Subducted is the continental rock that went down with a plate's
	// edge, and Accreted the part of it that was added to the plate above.
	// The rest, if there is any, is lost to the mantle.
	Subducted, Accreted float64
	// Erupted is the continental rock made new in volcanic arcs, above
	// ocean floor that went down.
	Erupted float64
	// Resampled is the rock gained, or if below zero lost, when the
	// plates' crust was read onto the world's grid: each time the plates
	// were drawn afresh, and at the end. A cell of the world takes the
	// crust of the nearest cell of a plate, which does not keep the total
	// exactly.
	Resampled float64
	// End is the continental rock in the world's crust at the end.
	End float64
}

// World is the plates of a world, on one grid.
type World struct {
	Grid   *sphere.Grid
	Plates []*Plate
	// Steps is the number of steps taken so far.
	Steps int
	// Fastest is the highest speed any part of a plate has moved at, in
	// radians per million years.
	Fastest float64
	// Time is how long the plates have run, in millions of years, and
	// Cycles the number of times they have been drawn afresh.
	Time   float64
	Cycles int

	// drawn is the time at which the present plates were drawn.
	drawn  float64
	budget Budget
}

// New divides the crust into plates.
func New(g *sphere.Grid, columns []crust.Column) *World {
	w := &World{Grid: g}
	w.draw(columns)
	w.budget.Start = continental(g, columns, nil)
	return w
}

// draw replaces the plates with ones made from the world's crust.
func (w *World) draw(columns []crust.Column) {
	owner, count := Segment(w.Grid, columns)
	w.Plates = w.Plates[:0]
	for id := range int32(count) {
		member := make([]bool, len(owner))
		for cell, o := range owner {
			member[cell] = o == id
		}
		w.Plates = append(w.Plates, newPlate(w.Grid, member, columns))
	}
	w.drawn = w.Time
}

// redraw ends a cycle: it puts the world's crust together and divides it
// into plates again.
func (w *World) redraw() error {
	m := w.Merge()
	if err := crust.Check(m.Columns); err != nil {
		return fmt.Errorf("plates: step %d: %w", w.Steps, err)
	}
	before := w.continental()
	w.draw(m.Columns)
	w.budget.Resampled += w.continental() - before
	w.Cycles++
	return nil
}

// metres2 is the area, in square metres, of a steradian of the world.
const metres2 = sphere.RadiusKm * sphere.RadiusKm * 1e6

// continental returns the continental rock in the cells chosen, or in all
// of them if member is nil, in kilograms.
func continental(g *sphere.Grid, columns []crust.Column, member []bool) float64 {
	var sum float64
	for cell, c := range columns {
		if member == nil || member[cell] {
			sum += c.Continental() * g.Area[cell]
		}
	}
	return sum * metres2
}

// continental returns the continental rock in all the plates, in kilograms.
func (w *World) continental() float64 {
	var sum float64
	for _, p := range w.Plates {
		sum += continental(w.Grid, p.Crust, p.Member)
	}
	return sum
}

// Count returns the number of plates that have any crust left.
func (w *World) Count() int {
	n := 0
	for _, p := range w.Plates {
		if p.Area > 0 {
			n++
		}
	}
	return n
}

// Budget returns the account of the world's continental rock, closed with
// the crust that Merge has just put together.
func (w *World) Budget(m *Merged) Budget {
	b := w.budget
	b.End = continental(w.Grid, m.Columns, nil)
	b.Resampled += b.End - w.continental()
	return b
}

// Advance runs the plates for a time, in millions of years. It chooses its
// own steps: short enough that the fastest plate moves less than a cell in
// each. After each step it makes new ocean floor where plates have parted
// and takes away the edges of plates that have sunk under others, and every
// CycleMyr it draws the plates afresh.
func (w *World) Advance(myr float64) error { return w.advance(myr, true) }

// drift moves the plates as Advance does, and does nothing else: no crust
// is made or taken away, and the plates are never drawn afresh.
func (w *World) drift(myr float64) error { return w.advance(myr, false) }

func (w *World) advance(myr float64, margins bool) error {
	// Steps end exactly on the end of the run and of each cycle, so that
	// neither depends on how the steps' lengths round when summed.
	end := w.Time + myr
	var rates []sphere.Vec3
	// The world's crust as it was last put together, while the plates are
	// still the ones it was put together from.
	var merged *Merged
	for w.Time < end {
		until := end
		if margins {
			if w.Time >= w.drawn+CycleMyr {
				if err := w.redraw(); err != nil {
					return err
				}
				merged = nil
			}
			until = min(until, w.drawn+CycleMyr)
		}
		step := min(until-w.Time, MaxStepMyr)
		rates = rates[:0]
		for _, p := range w.Plates {
			rates = append(rates, p.AngularVelocity(w.Grid))
		}
		if merged != nil {
			w.brake(merged, rates)
		}
		var fastest float64
		for i, p := range w.Plates {
			fastest = max(fastest, p.speed(w.Grid, rates[i]))
		}
		if math.IsNaN(fastest) || math.IsInf(fastest, 0) {
			return fmt.Errorf("plates: step %d: a plate's speed is not finite", w.Steps)
		}
		w.Fastest = max(w.Fastest, fastest)
		if fastest > 0 {
			step = min(step, stepCells*w.Grid.Spacing()/fastest)
		}
		for i, p := range w.Plates {
			p.turn(rates[i], step)
		}
		if margins {
			merged = w.Merge()
			if err := w.erode(merged, step); err != nil {
				return fmt.Errorf("plates: step %d: %w", w.Steps, err)
			}
			w.margins(merged)
			if err := w.check(); err != nil {
				return err
			}
		}
		if next := w.Time + step; step == until-w.Time || next >= until {
			w.Time = until
		} else {
			w.Time = next
		}
		w.Steps++
	}
	return nil
}

// erode wears down the land for a time, in millions of years, given the
// world's crust as Merge has just put it together.
//
// The rock that leaves a cell of the world is taken from every plate that
// has crust there, each giving the same share of what it has, and lands on
// the uppermost plate of the cell it goes to.
func (w *World) erode(m *Merged, myr float64) error {
	g := w.Grid
	sea, err := crust.SeaLevel(crust.Elevations(m.Columns), g.Area, crust.OceanMeanDepth)
	if err != nil {
		return err
	}
	for _, f := range erosion.Flows(g, m.Columns, sea, myr) {
		top := w.Plates[m.Top[f.To]]
		onto := m.local[m.Top[f.To]][f.To]
		for i, p := range w.Plates {
			local := m.local[i][f.From]
			if local < 0 {
				continue
			}
			// The share is of the rock under the cell of the world, which
			// is not the size of the plate's cell.
			share := f.Share * g.Area[f.From] / g.Area[local]
			erosion.Move(p.Crust[local], share, &p.Crust[local], g.Area[local], &top.Crust[onto], g.Area[onto])
		}
	}
	return nil
}

// check returns an error if any plate holds crust that cannot be.
func (w *World) check() error {
	for i, p := range w.Plates {
		for cell, in := range p.Member {
			if !in {
				continue
			}
			if err := p.Crust[cell].Check(); err != nil {
				return fmt.Errorf("plates: step %d: plate %d, cell %d: %w", w.Steps, i, cell, err)
			}
		}
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

	// local has, for each plate, the cell of the plate that lies over each
	// cell of the world, or -1 where the plate does not reach.
	local [][]int32
}

// Merge puts the world's crust together from the plates, as they lie now.
//
// Where plates overlap, the least dense is uppermost. The ocean floor and
// the age of the crust are the uppermost plate's, and the lower plate's
// ocean floor is taken to have gone down into the mantle; continental rock
// from every plate there is stacked. Where the plates have parted and none
// covers a cell, the cell has no crust.
func (w *World) Merge() *Merged {
	g := w.Grid
	n := len(g.Pos)
	m := &Merged{
		Columns: make([]crust.Column, n),
		Top:     make([]int32, n),
		Count:   make([]int32, n),
		local:   make([][]int32, len(w.Plates)),
	}
	for i := range m.local {
		m.local[i] = make([]int32, n)
	}
	// For each plate, where its middle is in the world, and the rotation
	// from the world back to its grid.
	centers := make([]sphere.Vec3, len(w.Plates))
	back := make([]sphere.Mat3, len(w.Plates))
	for i, p := range w.Plates {
		centers[i] = p.Rot.Apply(p.center)
		back[i] = p.Rot.Transpose()
	}
	// Each cell of the world is worked out on its own, so the cells are
	// shared among the processors. The result does not depend on how.
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			for cell := worker * n / workers; cell < (worker+1)*n/workers; cell++ {
				w.mergeCell(m, centers, back, cell)
			}
		})
	}
	wg.Wait()
	for cell, count := range m.Count {
		switch count {
		case 0:
			m.Gap += g.Area[cell]
		case 1:
		default:
			m.Overlap += g.Area[cell]
		}
	}
	m.Gap /= 4 * math.Pi
	m.Overlap /= 4 * math.Pi
	return m
}

// mergeCell puts together the crust of one cell of the world.
func (w *World) mergeCell(m *Merged, centers []sphere.Vec3, back []sphere.Mat3, cell int) {
	g := w.Grid
	pos := g.Pos[cell]
	top, lightest := int32(-1), math.Inf(1)
	var column crust.Column
	for i, p := range w.Plates {
		m.local[i][cell] = -1
		if pos.Dot(centers[i]) < p.reach {
			continue
		}
		local := g.Nearest(back[i].Apply(pos))
		if !p.Member[local] {
			continue
		}
		m.local[i][cell] = local
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
	m.Columns[cell] = column
}
