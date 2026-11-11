// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package plates divides the crust into rigid plates, moves them over the
// sphere, and puts the world's crust back together from them.
//
// The model follows the lithosphere model of tectonics.js
// (https://github.com/davidson16807/tectonics.js), which is licensed under
// CC BY 4.0. This is a new implementation of its algorithms, not a
// translation of its code, and it departs from them where PORT_PLAN.md
// says so. The locking of continents that meet and the volcanic arcs are not
// from tectonics.js at all.
package plates

import (
	"cmp"
	"math"
	"slices"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

// The constants of segmentation. tectonics.js gives the lengths as counts
// of cells at its usual detail of 5, where cells are about 240 km apart;
// they are lengths and areas here so that the plates come out alike at any
// detail.
const (
	// MaxPlates is the most plates the crust is divided into.
	MaxPlates = 7
	// MinPlateAreaKm2 is the least area a plate may have, in square
	// kilometres.
	MinPlateAreaKm2 = 10e6
	// GrowthKm is how far, in kilometres, a plate found from the mantle's
	// flow is first grown into ground that no plate has claimed.
	GrowthKm = 1200.0
	// SmoothingKm is the distance, in kilometres, over which the crust's
	// buoyancy is smoothed into the pressure that drives the mantle. It is
	// the spread of the thirty rounds of averaging that tectonics.js does
	// on a grid of detail 4.
	SmoothingKm = 1900.0
	// likeFlow is the cosine of the widest angle between the mantle's turn
	// at two cells of one plate's seed region.
	likeFlow = 0.5
)

// Segment divides the crust into plates. It returns the plate of each
// cell, numbered from zero, and the number of plates, which is at least
// one and at most MaxPlates. Every cell is in one plate, every plate is
// connected, and none is smaller than MinPlateAreaKm2.
//
// Where the crust is dense the mantle is taken to be under pressure, and to
// flow down the pressure's slope. A plate begins as a region over which the
// mantle turns about much the same axis.
func Segment(g *sphere.Grid, columns []crust.Column) (plate []int32, count int) {
	n := int32(len(g.Pos))
	minArea := MinPlateAreaKm2 / (sphere.RadiusKm * sphere.RadiusKm)

	pressure := make([]float64, n)
	for i, c := range columns {
		pressure[i] = -c.Buoyancy()
	}
	pressure = g.Diffuse(pressure, SmoothingKm/sphere.RadiusKm)
	// The axis the mantle turns about under each cell, and how fast.
	turn := g.Gradient(pressure)
	speed := make([]float64, n)
	for i, flow := range turn {
		turn[i] = flow.Cross(g.Pos[i])
		speed[i] = turn[i].Len()
	}

	plate = make([]int32, n)
	for i := range plate {
		plate[i] = -1
	}

	// Seed regions: start where the mantle turns fastest, and take the
	// ground around it that turns the same way. A region too small to be a
	// plate is set aside, and its ground is not tried again.
	tried := make([]bool, n)
	for range 2 * MaxPlates {
		if count == MaxPlates-1 {
			break
		}
		seed := int32(-1)
		for cell := range n {
			if !tried[cell] && speed[cell] > 0 && (seed < 0 || speed[cell] > speed[seed]) {
				seed = cell
			}
		}
		if seed < 0 {
			break
		}
		region := g.Flood(seed, func(cell int32) bool {
			return !tried[cell] && turn[cell].Dot(turn[seed]) > likeFlow*speed[cell]*speed[seed]
		})
		var area float64
		for cell, in := range region {
			if in {
				tried[cell] = true
				area += g.Area[cell]
			}
		}
		if area < minArea {
			continue
		}
		for cell, in := range region {
			if in {
				plate[cell] = int32(count)
			}
		}
		count++
	}

	grow(g, plate, g.Hops(GrowthKm))

	// Ground still unclaimed makes plates of its own, largest first, while
	// there is room for more and the ground is large enough.
	label, groups := g.Components(func(cell int32) bool { return plate[cell] < 0 })
	area := make([]float64, groups+1)
	for cell, l := range label {
		area[l] += g.Area[cell]
	}
	order := make([]int32, groups)
	for i := range order {
		order[i] = int32(i + 1)
	}
	slices.SortFunc(order, func(a, b int32) int {
		return cmp.Or(cmp.Compare(area[b], area[a]), cmp.Compare(a, b))
	})
	for _, l := range order {
		if count == MaxPlates || area[l] < minArea {
			break
		}
		for cell := range plate {
			if label[cell] == l {
				plate[cell] = int32(count)
			}
		}
		count++
	}

	// Whatever is left joins the plates beside it.
	grow(g, plate, math.MaxInt)
	return plate, count
}

// grow extends each plate by a number of steps from cell to neighbor into
// cells that have no plate, all plates at once. A cell that two plates
// reach together goes to the lower numbered.
func grow(g *sphere.Grid, plate []int32, steps int) {
	var front, next []int32
	for cell := range int32(len(plate)) {
		if plate[cell] >= 0 {
			front = append(front, cell)
		}
	}
	type claim struct{ cell, plate int32 }
	var claims []claim
	for ; steps > 0 && len(front) > 0; steps-- {
		claims = claims[:0]
		for _, cell := range front {
			for _, v := range g.Neighbors[cell] {
				if plate[v] < 0 {
					claims = append(claims, claim{v, plate[cell]})
				}
			}
		}
		// The claims of a round are settled together, lowest plate first,
		// so the order the cells were visited in does not matter.
		slices.SortFunc(claims, func(a, b claim) int {
			return cmp.Or(cmp.Compare(a.plate, b.plate), cmp.Compare(a.cell, b.cell))
		})
		next = next[:0]
		for _, c := range claims {
			if plate[c.cell] < 0 {
				plate[c.cell] = c.plate
				next = append(next, c.cell)
			}
		}
		front, next = next, front
	}
}
