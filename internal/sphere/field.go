// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Spacing returns the typical distance between neighboring cells, in
// radians: the distance between the centers of hexagons of the mean cell
// area.
func (g *Grid) Spacing() float64 {
	mean := 4 * math.Pi / float64(len(g.Pos))
	return math.Sqrt(2 * mean / math.Sqrt(3))
}

// Hops returns the number of steps from cell to neighbor that cover a
// distance on the ground, in kilometres. It is never less than one.
func (g *Grid) Hops(km float64) int {
	return max(1, int(math.Round(km/(g.Spacing()*RadiusKm))))
}

// Gradient returns the gradient of a field that has one value for each
// cell: at each cell, the vector along the ground that points the way the
// field rises fastest, whose length is the rise per radian.
func (g *Grid) Gradient(field []float64) []Vec3 {
	out := make([]Vec3, len(g.Pos))
	for i := range g.Pos {
		out[i] = g.GradientAt(field, int32(i))
	}
	return out
}

// GradientAt returns the gradient of a field at one cell, as Gradient does
// for all of them.
func (g *Grid) GradientAt(field []float64, cell int32) Vec3 {
	// Fit the neighbors' values, by least squares, with a slope in the
	// plane that touches the sphere at the cell, which e1 and e2 span, and
	// a bowl around the cell. The bowl takes up the part of the field's
	// change that comes from the ground curving away from the plane, which
	// would otherwise be read as slope wherever the neighbors are not
	// evenly placed. A field that is linear in space is fitted exactly.
	p := g.Pos[cell]
	e1 := tangent(p)
	e2 := p.Cross(e1)
	var m Mat3
	var rhs Vec3
	for _, j := range g.Neighbors[cell] {
		d := g.Pos[j].Sub(p)
		row := Vec3{d.Dot(e1), d.Dot(e2), d.Dot(d)}
		df := field[j] - field[cell]
		m[0] = m[0].Add(row.Scale(row.X))
		m[1] = m[1].Add(row.Scale(row.Y))
		m[2] = m[2].Add(row.Scale(row.Z))
		rhs = rhs.Add(row.Scale(df))
	}
	fit := m.Solve(rhs)
	return e1.Scale(fit.X).Add(e2.Scale(fit.Y))
}

// tangent returns a vector of length one at right angles to p, which must
// be of length one.
func tangent(p Vec3) Vec3 {
	// Cross p with the axis it is furthest from, so the product is never
	// near zero.
	axis := Vec3{X: 1}
	if math.Abs(p.X) > 0.6 {
		axis = Vec3{Y: 1}
	}
	return p.Cross(axis).Normalize()
}

// EdgeWeights returns, for each cell and each of its neighbors in the order
// of Neighbors, how strongly a difference between the two drives a flow
// between them. A weight is the same seen from either end, which is what
// makes Diffuse keep the field's mean. They are the cotangent weights of
// the flat triangles of the mesh.
func (g *Grid) EdgeWeights() [][]float64 {
	weights := make([][]float64, len(g.Pos))
	flat := make([]float64, len(g.Faces)*3)
	for i, n := range g.Neighbors {
		weights[i], flat = flat[:len(n):len(n)], flat[len(n):]
	}
	add := func(a, b int32, w float64) {
		for k, v := range g.Neighbors[a] {
			if v == b {
				weights[a][k] += w
				return
			}
		}
	}
	for _, face := range g.Faces {
		for k := range 3 {
			// The corner at c faces the edge from a to b.
			a, b, c := face[k], face[(k+1)%3], face[(k+2)%3]
			u, v := g.Pos[a].Sub(g.Pos[c]), g.Pos[b].Sub(g.Pos[c])
			w := 0.5 * u.Dot(v) / u.Cross(v).Len()
			add(a, b, w)
			add(b, a, w)
		}
	}
	return weights
}

// Diffuse returns the field smoothed as heat spreads: as if each value had
// been spread over the ground around its cell in a bell curve whose
// standard deviation along any one direction is sigma radians. The mean of
// the field, each cell counting by its area, is kept, and its variance is
// not raised.
func (g *Grid) Diffuse(field []float64, sigma float64) []float64 {
	out := make([]float64, len(field))
	copy(out, field)
	if !(sigma > 0) {
		return out
	}
	weights := g.EdgeWeights()
	// The largest step that leaves each cell's new value a mix of its own
	// and its neighbors' old ones.
	limit := math.Inf(1)
	for i, ws := range weights {
		var sum float64
		for _, w := range ws {
			sum += w
		}
		limit = min(limit, g.Area[i]/sum)
	}
	// Heat that has spread for a time t has a variance of 2t.
	total := sigma * sigma / 2
	steps := int(math.Ceil(total / (0.9 * limit)))
	dt := total / float64(steps)
	next := make([]float64, len(field))
	for range steps {
		for i, ws := range weights {
			var flow float64
			for k, j := range g.Neighbors[i] {
				flow += ws[k] * (out[j] - out[i])
			}
			next[i] = out[i] + dt*flow/g.Area[i]
		}
		out, next = next, out
	}
	return out
}

// Dilate returns the set grown by a number of steps: every cell that can be
// reached from a cell of the set in that many steps from cell to neighbor.
func (g *Grid) Dilate(set []bool, steps int) []bool {
	return g.spread(set, steps, true)
}

// Erode returns the set shrunk by a number of steps: every cell that
// cannot be reached from a cell outside the set in that many steps.
func (g *Grid) Erode(set []bool, steps int) []bool {
	return g.spread(set, steps, false)
}

// spread grows the cells whose value is grow into the others.
func (g *Grid) spread(set []bool, steps int, grow bool) []bool {
	out := make([]bool, len(set))
	copy(out, set)
	var front, next []int32
	for cell := range int32(len(out)) {
		if out[cell] == grow {
			front = append(front, cell)
		}
	}
	for range steps {
		next = next[:0]
		for _, cell := range front {
			for _, v := range g.Neighbors[cell] {
				if out[v] != grow {
					out[v] = grow
					next = append(next, v)
				}
			}
		}
		front, next = next, front
	}
	return out
}

// Flood returns the cells that can be reached from start by steps from cell
// to neighbor without leaving the cells for which member is true. If start
// is not a member, the result is empty.
func (g *Grid) Flood(start int32, member func(cell int32) bool) []bool {
	out := make([]bool, len(g.Pos))
	if !member(start) {
		return out
	}
	out[start] = true
	stack := []int32{start}
	for len(stack) > 0 {
		cell := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, v := range g.Neighbors[cell] {
			if !out[v] && member(v) {
				out[v] = true
				stack = append(stack, v)
			}
		}
	}
	return out
}
