// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"slices"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

// The continental rock of a plate's edge that goes down is added to the
// plate above it as felsic rock: this share as rock that cooled at depth,
// and the rest as rock that came out at the surface. The shares are the
// ones tectonics.js uses.
const accretedPlutonic = 0.85

// sunk is a cell of a plate that has gone down under another plate.
type sunk struct {
	plate int
	// cell is the cell of the plate, and under the cell of the world it
	// lay beneath.
	cell, under int32
}

// margins changes the plates where they meet and where they have parted,
// given the world's crust as Merge has just put it together.
func (w *World) margins(m *Merged) {
	w.subduct(m)
	w.rift(m)
}

// subduct takes away the crust that has gone down under another plate.
//
// Subduction: crust of a plate that lies under another plate and is denser
// than the mantle goes down, if it is joined to the plate's edge by other
// such crust. Its cells are taken from the plate. Accretion: the
// continental rock those cells carried is scraped off onto the plate above.
func (w *World) subduct(m *Merged) {
	g := w.Grid
	// Find all that sinks before taking any of it, so that every plate is
	// judged by the same world.
	var gone []sunk
	for i := range w.Plates {
		gone = w.sinking(m, i, gone)
	}
	changed := make([]bool, len(w.Plates))
	for _, s := range gone {
		p := w.Plates[s.plate]
		rock := p.Crust[s.cell].Continental() * g.Area[s.cell]
		p.Member[s.cell] = false
		p.Crust[s.cell] = crust.Column{}
		changed[s.plate] = true
		w.budget.Subducted += rock * metres2

		// The plate on top where the cell went down is not this one, and
		// its crust there is on top, so it is not itself going down. A
		// cell that lay over no cell of the world may have nothing above
		// it, and then its rock is lost.
		top := m.Top[s.under]
		if rock == 0 || top < 0 || int(top) == s.plate {
			continue
		}
		onto := m.local[top][s.under]
		c := &w.Plates[top].Crust[onto]
		// The rock is spread over a cell of another size.
		c.FelsicPlutonic += rock / g.Area[onto] * accretedPlutonic
		c.FelsicVolcanic += rock / g.Area[onto] * (1 - accretedPlutonic)
		w.budget.Accreted += rock * metres2
	}
	w.outline(changed)
}

// rift makes new ocean floor where the plates have parted: where no plate
// covers the world, the plates beside the gap each take the part of it
// nearest them, however wide it is.
//
// Where two plates' shares of a gap meet, a new cell of one may also be the
// nearest to a cell of the world that went to the other, so the two can
// overlap there by a cell. Both are new floor, and it does no harm.
func (w *World) rift(m *Merged) {
	if m.Gap == 0 {
		return
	}
	g := w.Grid
	// Share out the gaps: each plate grows into the cells nearest it.
	beside := slices.Clone(m.Top)
	grow(g, beside, len(beside))
	changed := make([]bool, len(w.Plates))
	back := make([]sphere.Mat3, len(w.Plates))
	for i, p := range w.Plates {
		back[i] = p.Rot.Transpose()
	}
	for cell, count := range m.Count {
		i := beside[cell]
		if count != 0 || i < 0 {
			continue
		}
		p := w.Plates[i]
		changed[i] = true
		local := g.Nearest(back[i].Apply(g.Pos[cell]))
		p.rift(local)
		// A cell of the plate may be the nearest to no cell of the world,
		// though it lies in the gap. Take those too, or the plate is left
		// with holes that open as gaps when it has moved on.
		for _, v := range g.Neighbors[local] {
			if p.Member[v] {
				continue
			}
			over := g.Nearest(p.Rot.Apply(g.Pos[v]))
			if m.Count[over] == 0 && beside[over] == i {
				p.rift(v)
			}
		}
	}
	w.outline(changed)
}

// outline works out again the outlines of the plates that have changed.
func (w *World) outline(changed []bool) {
	for i, p := range w.Plates {
		if changed[i] {
			p.outline(w.Grid)
		}
	}
}

// rift adds a cell to the plate, as new ocean floor.
func (p *Plate) rift(cell int32) {
	p.Member[cell] = true
	p.Crust[cell] = crust.NewOcean()
}

// sinking appends the cells of a plate that go down into the mantle.
func (w *World) sinking(m *Merged, plate int, gone []sunk) []sunk {
	g := w.Grid
	p := w.Plates[plate]
	if len(p.edge) == 0 {
		return gone
	}
	// A cell of the plate is seen if it lies over some cell of the world,
	// and on top if it is the uppermost crust at any of them. under is one
	// of the cells of the world it lies over.
	n := len(p.Member)
	seen, onTop := make([]bool, n), make([]bool, n)
	under := make([]int32, n)
	for cell, local := range m.local[plate] {
		if local < 0 {
			continue
		}
		seen[local] = true
		under[local] = int32(cell)
		if m.Top[cell] == int32(plate) {
			onTop[local] = true
		}
	}
	sinks := func(cell int32) bool {
		return p.Member[cell] && !onTop[cell] &&
			p.Crust[cell].Density() > crust.MantleDensity
	}

	// Start from the cells of the edge that are seen to be under another
	// plate, and follow the crust that sinks inward from them. A cell that
	// is not seen cannot start it, since nothing shows it is under
	// anything, but it goes down with its neighbors.
	taken := make([]bool, n)
	first := len(gone)
	take := func(cell int32) {
		if taken[cell] || !sinks(cell) {
			return
		}
		taken[cell] = true
		where := under[cell]
		if !seen[cell] {
			where = g.Nearest(p.Rot.Apply(g.Pos[cell]))
		}
		gone = append(gone, sunk{plate, cell, where})
	}
	for _, e := range p.edge {
		if seen[e.cell] {
			take(e.cell)
		}
	}
	for at := first; at < len(gone); at++ {
		for _, v := range g.Neighbors[gone[at].cell] {
			take(v)
		}
	}
	return gone
}
