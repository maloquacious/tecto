// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tecto

import (
	"math"
	"slices"

	"github.com/maloquacious/tecto/internal/sphere"
)

const (
	// highShare is the share of the land, by area, that counts as high
	// ground: the highest tenth. Hāmwold puts about that share of its land
	// above 2,000 m.
	highShare = 0.10
	// minRangeKm2 is the smallest piece of high ground whose shape is
	// measured, in square kilometres. A smaller one is a few cells, and
	// has no shape to speak of.
	minRangeKm2 = 100e3
)

// highGround marks the cells of the highest tenth of the land, by area.
// Land is the cells above zero.
func highGround(g *sphere.Grid, height []float64) []bool {
	var land []int32
	var area float64
	for cell, h := range height {
		if h > 0 {
			land = append(land, int32(cell))
			area += g.Area[cell]
		}
	}
	// Highest first, and the lower cell first among equals, so the result
	// does not depend on how the sort treats them.
	slices.SortFunc(land, func(a, b int32) int {
		switch {
		case height[a] != height[b]:
			if height[a] > height[b] {
				return -1
			}
			return 1
		default:
			return int(a - b)
		}
	})
	high := make([]bool, len(height))
	var taken float64
	for _, cell := range land {
		if taken >= highShare*area {
			break
		}
		high[cell] = true
		taken += g.Area[cell]
	}
	return high
}

// elongation returns how long the pieces of high ground are for their
// width: 1 for a round piece, and 4 for one four times as long as it is
// wide. It is the mean over the connected pieces of at least minRangeKm2,
// each counting by its area, and zero if there are none.
//
// A piece's length and width are those of the bar of even weight that is
// spread as widely as the piece is, along the piece's longest direction and
// across it. No piece is taken to be narrower than one cell.
func elongation(g *sphere.Grid, high []bool) float64 {
	label, count := g.Components(func(cell int32) bool { return high[cell] })
	cells := make([][]int32, count+1)
	for cell, l := range label {
		if l != 0 {
			cells[l] = append(cells[l], int32(cell))
		}
	}
	const km2 = sphere.RadiusKm * sphere.RadiusKm
	var sum, total float64
	for _, piece := range cells[1:] {
		var area float64
		var middle sphere.Vec3
		for _, cell := range piece {
			area += g.Area[cell]
			middle = middle.Add(g.Pos[cell].Scale(g.Area[cell]))
		}
		if area*km2 < minRangeKm2 || middle.Len() < 1e-9*area {
			continue
		}
		// The spread of the piece along two directions at right angles on
		// the plane that touches the sphere at its middle.
		c := middle.Normalize()
		axis := sphere.Vec3{X: 1}
		if math.Abs(c.X) > 0.6 {
			axis = sphere.Vec3{Y: 1}
		}
		e1 := c.Cross(axis).Normalize()
		e2 := c.Cross(e1)
		var xx, xy, yy float64
		for _, cell := range piece {
			d := g.Pos[cell].Sub(c)
			x, y, a := d.Dot(e1), d.Dot(e2), g.Area[cell]
			xx += a * x * x
			xy += a * x * y
			yy += a * y * y
		}
		xx, xy, yy = xx/area, xy/area, yy/area
		// The greatest and least spread, over all directions. A bar of
		// even weight has a spread of its length squared over 12.
		mid, half := (xx+yy)/2, math.Hypot((xx-yy)/2, xy)
		length := math.Sqrt(12 * (mid + half))
		width := max(math.Sqrt(12*max(mid-half, 0)), g.Spacing())
		sum += area * max(length/width, 1)
		total += area
	}
	if total == 0 {
		return 0
	}
	return sum / total
}

// meanDistanceKm returns how far the chosen cells are, on average and each
// counting by its area, from the nearest of the cells marked, in
// kilometres. The distance is counted in steps from cell to cell. It is
// zero if no cell is marked or none is chosen.
func meanDistanceKm(g *sphere.Grid, marked, chosen []bool) float64 {
	steps := make([]int32, len(marked))
	var front, next []int32
	for cell, m := range marked {
		steps[cell] = -1
		if m {
			steps[cell] = 0
			front = append(front, int32(cell))
		}
	}
	if len(front) == 0 {
		return 0
	}
	for depth := int32(1); len(front) > 0; depth++ {
		next = next[:0]
		for _, cell := range front {
			for _, v := range g.Neighbors[cell] {
				if steps[v] < 0 {
					steps[v] = depth
					next = append(next, v)
				}
			}
		}
		front, next = next, front
	}
	var sum, area float64
	for cell, c := range chosen {
		if c {
			sum += float64(steps[cell]) * g.Area[cell]
			area += g.Area[cell]
		}
	}
	if area == 0 {
		return 0
	}
	return sum / area * g.Spacing() * sphere.RadiusKm
}
