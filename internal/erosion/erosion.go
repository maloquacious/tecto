// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package erosion wears down the land: rock moves from a cell to each lower
// cell beside it, in proportion to the drop.
//
// The rule is the erosion of tectonics.js
// (https://github.com/davidson16807/tectonics.js), which is licensed under
// CC BY 4.0. This is a new implementation, not a translation of its code,
// and it departs from it as PORT_PLAN.md records.
package erosion

import (
	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

// Rate is how fast the land wears down, in square kilometres per million
// years: the surface moves as heat spreads, with this as its diffusivity,
// before the crust rises to make up for the rock it has lost.
//
// A column that loses rock floats higher in the mantle, and gets back about
// five sixths of the height it lost. So a range wears down about six times
// more slowly than this figure alone would have it: one 300 km across lasts
// some hundreds of millions of years.
//
// The value was chosen from runs at 100, 200, 400, 800, and 1,600, measured
// through Hāmwold. The faster the land wears, the fewer mountains are left
// and the less they run in ranges: at 1,600 there are a third fewer than at
// 400. At 200 there are a fifth more than at 400, in ranges as long for
// their width or longer. At 100 there are more again, but the ranges grow
// wider, the highest ground higher, and the land smaller as rock stays
// piled where it was stacked.
const Rate = 200.0

// maxShare is the most of a cell's continental rock that may leave it in
// one step. It is there so that a long step over a steep drop cannot take
// more rock than the cell has, and is set well short of all of it so that
// a cell is not emptied onto its neighbors and left lower than they are.
const maxShare = 0.25

// Flow is rock on the move from one cell to a cell beside it.
type Flow struct {
	From, To int32
	// Share is the share of From's continental rock that moves.
	Share float64
}

// Flows returns the rock that moves in a time, in millions of years, given
// the crust of the world and the level of the sea in it.
//
// Only land wears down: a cell loses rock if it stands above the sea, to
// each cell beside it that is lower, in proportion to the drop to that cell
// or to the sea if the cell is under it. Rock that reaches the sea stays
// where it lands, and builds the shore out. The rock of the ocean floor does
// not move.
func Flows(g *sphere.Grid, columns []crust.Column, sea, myr float64) []Flow {
	height := crust.Elevations(columns)
	weights := g.EdgeWeights()
	var flows []Flow
	for cell, h := range height {
		rock := columns[cell].Continental()
		if h <= sea || rock <= 0 {
			continue
		}
		first := len(flows)
		var total float64
		for k, v := range g.Neighbors[cell] {
			// A cell with no crust has no plate to take the rock.
			drop := h - max(height[v], sea)
			if drop <= 0 || columns[v].Mass() <= 0 {
				continue
			}
			// The volume that crosses to the neighbor, in square
			// kilometres of ground by metres of height, then as a mass for
			// each square metre of the cell.
			volume := Rate * myr * weights[cell][k] * drop
			mass := volume * crust.FelsicDensity / (g.Area[cell] * sphere.RadiusKm * sphere.RadiusKm)
			share := mass / rock
			flows = append(flows, Flow{int32(cell), v, share})
			total += share
		}
		if total > maxShare {
			for i := first; i < len(flows); i++ {
				flows[i].Share *= maxShare / total
			}
		}
	}
	return flows
}

// Apply moves the rock of the flows, for crust that is all on one grid.
func Apply(g *sphere.Grid, columns []crust.Column, flows []Flow) {
	// Every flow is a share of what the cell held before any of them.
	before := make(map[int32]crust.Column)
	for _, f := range flows {
		if _, ok := before[f.From]; !ok {
			before[f.From] = columns[f.From]
		}
	}
	for _, f := range flows {
		Move(before[f.From], f.Share, &columns[f.From], g.Area[f.From], &columns[f.To], g.Area[f.To])
	}
}

// Move takes a share of the continental rock of the column held from the
// column from, and adds it to the column to. The two columns stand on cells
// of the areas given, and the rock is spread over the cell it lands on, so
// that no mass is made or lost.
func Move(held crust.Column, share float64, from *crust.Column, fromArea float64, to *crust.Column, toArea float64) {
	spread := fromArea / toArea
	move := func(held float64, from, to *float64) {
		if held <= 0 {
			return
		}
		mass := min(held*share, *from)
		*from -= mass
		*to += mass * spread
	}
	move(held.Sediment, &from.Sediment, &to.Sediment)
	move(held.Sedimentary, &from.Sedimentary, &to.Sedimentary)
	move(held.Metamorphic, &from.Metamorphic, &to.Metamorphic)
	move(held.FelsicPlutonic, &from.FelsicPlutonic, &to.FelsicPlutonic)
	move(held.FelsicVolcanic, &from.FelsicVolcanic, &to.FelsicVolcanic)
}
