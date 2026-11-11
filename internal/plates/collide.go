// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"math"

	"github.com/maloquacious/tecto/internal/sphere"
)

// collisionDrag is how much harder it is to move two continents past each
// other where they have met than to move a plate over the mantle, area for
// area. Where crust that floats lies on crust that floats, neither can go
// down, and the plates can only go on converging by crumpling both. At this
// value a few cells of such crust slow two plates' approach to a fraction of
// what it was, so continents that meet lock together along a narrow belt
// instead of sliding far over each other.
//
// The value was chosen from runs at 300, 1,000, 3,000, and 10,000, measured
// through Hāmwold. At 300 and 1,000 the continents still slide: the belts
// of doubled crust are 650 to 1,000 km wide and less long for their width,
// and the land shrinks as it stacks. At 10,000 the ranges are no narrower
// than at 3,000, and there are fewer of them by 500 million years.
//
// This is not in tectonics.js, which lets continents override each other
// without limit.
const collisionDrag = 3000.0

// collisions returns, for each pair of plates that have met as continents,
// the resistance to their moving past each other: the sum over the cells of
// the world where both have crust that floats of area × (I − r·rᵀ). The
// second result says which plates are in any such pair.
func (w *World) collisions(m *Merged) (between map[[2]int]sphere.Mat3, held []bool) {
	g := w.Grid
	held = make([]bool, len(w.Plates))
	var floats []int
	for cell, count := range m.Count {
		if count < 2 {
			continue
		}
		floats = floats[:0]
		for i, p := range w.Plates {
			if local := m.local[i][cell]; local >= 0 && p.Member[local] && p.Crust[local].Floats() {
				floats = append(floats, i)
			}
		}
		if len(floats) < 2 {
			continue
		}
		r, a := g.Pos[cell], g.Area[cell]
		resist := sphere.Mat3{
			sphere.Vec3{X: 1}.Sub(r.Scale(r.X)).Scale(a),
			sphere.Vec3{Y: 1}.Sub(r.Scale(r.Y)).Scale(a),
			sphere.Vec3{Z: 1}.Sub(r.Scale(r.Z)).Scale(a),
		}
		if between == nil {
			between = make(map[[2]int]sphere.Mat3)
		}
		for x, i := range floats {
			for _, j := range floats[x+1:] {
				sum := between[[2]int{i, j}]
				for k := range sum {
					sum[k] = sum[k].Add(resist[k])
				}
				between[[2]int{i, j}] = sum
				held[i], held[j] = true, true
			}
		}
	}
	return between, held
}

// brake works out again the rates at which the plates turn, for the plates
// that have met others as continents. rates has each plate's rate on its
// own grid, as AngularVelocity gives it.
//
// Each plate is driven as before and resisted by the mantle under it. A
// plate that has met another is resisted as well, at each cell where they
// have met, in proportion to how fast the two move past each other there.
// The resistance on one is the drive on the other, so the rates of all such
// plates are found together.
func (w *World) brake(m *Merged, rates []sphere.Vec3) {
	between, held := w.collisions(m)
	if len(between) == 0 {
		return
	}
	// The unknowns are the rates of the held plates, in the world's frame:
	// three numbers for each.
	at := make([]int, len(w.Plates))
	n := 0
	for i := range w.Plates {
		at[i] = -1
		if held[i] {
			at[i] = n
			n += 3
		}
	}
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, n+1)
	}
	add := func(row, col int, m sphere.Mat3, scale float64) {
		for k := range 3 {
			a[row+k][col] += scale * m[k].X
			a[row+k][col+1] += scale * m[k].Y
			a[row+k][col+2] += scale * m[k].Z
		}
	}
	for i, p := range w.Plates {
		if !held[i] {
			continue
		}
		// The mantle's resistance, and the drive that the plate's rate
		// alone balances against it.
		drag := p.Rot.Mul(p.drag).Mul(p.Rot.Transpose())
		add(at[i], at[i], drag, p.Area)
		drive := drag.Apply(p.Rot.Apply(rates[i])).Scale(p.Area)
		a[at[i]][n], a[at[i]+1][n], a[at[i]+2][n] = drive.X, drive.Y, drive.Z
	}
	// The pairs in a fixed order, so that the sums round the same way on
	// every run.
	for i := range w.Plates {
		for j := i + 1; j < len(w.Plates); j++ {
			resist, ok := between[[2]int{i, j}]
			if !ok {
				continue
			}
			add(at[i], at[i], resist, collisionDrag)
			add(at[j], at[j], resist, collisionDrag)
			add(at[i], at[j], resist, -collisionDrag)
			add(at[j], at[i], resist, -collisionDrag)
		}
	}
	x := solve(a)
	for i, p := range w.Plates {
		if held[i] {
			world := sphere.Vec3{X: x[at[i]], Y: x[at[i]+1], Z: x[at[i]+2]}
			rates[i] = p.Rot.Transpose().Apply(world)
		}
	}
}

// solve returns x for which a·x = b, where each row of a ends with its
// entry of b. It changes a. If there is no one answer the result is not
// finite.
func solve(a [][]float64) []float64 {
	n := len(a)
	for col := range n {
		// Bring up the row with the largest entry in this column.
		best := col
		for row := col + 1; row < n; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[best][col]) {
				best = row
			}
		}
		a[col], a[best] = a[best], a[col]
		for row := col + 1; row < n; row++ {
			f := a[row][col] / a[col][col]
			for k := col; k <= n; k++ {
				a[row][k] -= f * a[col][k]
			}
		}
	}
	x := make([]float64, n)
	for row := n - 1; row >= 0; row-- {
		sum := a[row][n]
		for k := row + 1; k < n; k++ {
			sum -= a[row][k] * x[k]
		}
		x[row] = sum / a[row][row]
	}
	return x
}
