// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sphere is the grid the model runs on: an icosahedron whose faces
// are split into four, over and over, with every new corner pushed out onto
// the unit sphere. A cell is a corner of the resulting mesh and the ground
// nearest to it.
//
// The mesh is the same kind as the one tectonics.js
// (https://github.com/davidson16807/tectonics.js, CC BY 4.0) runs on. The
// lookups here use the way the mesh was built, not a separate index.
package sphere

import (
	"fmt"
	"math"
)

// RadiusKm is the radius of the world, in kilometres. Lengths on the unit
// sphere are multiplied by it to get distances on the ground.
const RadiusKm = 6371.0

// MinDetail and MaxDetail bound the number of times the icosahedron is
// split. Detail d has 10·4^d + 2 cells.
const (
	MinDetail = 1
	MaxDetail = 9
)

// Grid is a subdivided icosahedron on the unit sphere. It is not changed
// after it is built.
type Grid struct {
	// Detail is the number of times the icosahedron was split.
	Detail int
	// Pos is the position of each cell: a point on the unit sphere.
	Pos []Vec3
	// Faces are the triangles of the mesh. A face's three cells run
	// counterclockwise seen from outside. The four faces a face was split
	// into are at four times its index and the three after.
	Faces [][3]int32
	// Neighbors holds, for each cell, the cells it shares an edge with, in
	// counterclockwise order seen from outside, starting from the lowest.
	// Twelve cells have five neighbors and the rest have six.
	Neighbors [][]int32
	// Area is the area of each cell on the unit sphere, in steradians. A
	// cell has a third of each face it is a corner of, so the areas sum to
	// 4π.
	Area []float64

	// base is the icosahedron: the corners of its faces and the middle of
	// each face.
	base     [20][3]Vec3
	baseMids [20]Vec3
}

// NewGrid builds the grid for a detail.
func NewGrid(detail int) (*Grid, error) {
	if detail < MinDetail || detail > MaxDetail {
		return nil, fmt.Errorf("sphere: detail %d: want %d to %d", detail, MinDetail, MaxDetail)
	}
	g := &Grid{Detail: detail}

	// The icosahedron: a cell at each pole and two rings of five between
	// them, the southern ring turned 36° from the northern.
	ringLat := math.Atan(0.5)
	g.Pos = make([]Vec3, 12, cellCount(detail))
	g.Pos[0] = Vec3{0, 0, 1}
	g.Pos[11] = Vec3{0, 0, -1}
	for i := range 5 {
		lon := float64(i) * 2 * math.Pi / 5
		g.Pos[1+i] = LatLon(ringLat, lon)
		g.Pos[6+i] = LatLon(-ringLat, lon+math.Pi/5)
	}
	faces := make([][3]int32, 0, 20)
	for i := range int32(5) {
		j := (i + 1) % 5
		faces = append(faces,
			[3]int32{0, 1 + i, 1 + j},
			[3]int32{1 + i, 6 + i, 1 + j},
			[3]int32{6 + i, 6 + j, 1 + j},
			[3]int32{11, 6 + j, 6 + i},
		)
	}
	for f, face := range faces {
		a, b, c := g.Pos[face[0]], g.Pos[face[1]], g.Pos[face[2]]
		g.base[f] = [3]Vec3{a, b, c}
		g.baseMids[f] = a.Add(b).Add(c).Normalize()
	}

	// Split every face into four, detail times. An edge is shared by two
	// faces, so the cell in its middle is remembered by the edge's ends.
	// The map is only ever looked up, never ranged over.
	for range detail {
		mids := make(map[[2]int32]int32, len(faces)*3/2)
		mid := func(a, b int32) int32 {
			key := [2]int32{min(a, b), max(a, b)}
			if m, ok := mids[key]; ok {
				return m
			}
			m := int32(len(g.Pos))
			g.Pos = append(g.Pos, midpoint(g.Pos[a], g.Pos[b]))
			mids[key] = m
			return m
		}
		next := make([][3]int32, 0, len(faces)*4)
		for _, face := range faces {
			a, b, c := face[0], face[1], face[2]
			ab, bc, ca := mid(a, b), mid(b, c), mid(c, a)
			next = append(next,
				[3]int32{a, ab, ca},
				[3]int32{ab, b, bc},
				[3]int32{ca, bc, c},
				[3]int32{ab, bc, ca},
			)
		}
		faces = next
	}
	g.Faces = faces

	g.buildNeighbors()
	g.buildAreas()
	return g, nil
}

// cellCount returns the number of cells at a detail.
func cellCount(detail int) int { return 10<<(2*detail) + 2 }

// midpoint returns the point on the sphere halfway between a and b. It is
// symmetric in its arguments to the last bit, so the lookups can rebuild a
// cell's position from either end of its edge.
func midpoint(a, b Vec3) Vec3 { return a.Add(b).Normalize() }

// buildNeighbors fills in Neighbors from Faces.
func (g *Grid) buildNeighbors() {
	n := len(g.Pos)
	// A face (a, b, c) says that, going counterclockwise around a, c comes
	// straight after b. after[a] collects those pairs.
	type pair struct{ from, to int32 }
	after := make([][]pair, n)
	backing := make([]pair, 0, len(g.Faces)*3)
	count := make([]int, n)
	for _, face := range g.Faces {
		for _, v := range face {
			count[v]++
		}
	}
	for v := range n {
		after[v] = backing[len(backing) : len(backing) : len(backing)+count[v]]
		backing = backing[:len(backing)+count[v]]
	}
	for _, face := range g.Faces {
		for k := range 3 {
			v := face[k]
			after[v] = append(after[v], pair{face[(k+1)%3], face[(k+2)%3]})
		}
	}

	g.Neighbors = make([][]int32, n)
	flat := make([]int32, 0, len(g.Faces)*3)
	for v := range n {
		pairs := after[v]
		start := pairs[0].from
		for _, p := range pairs[1:] {
			start = min(start, p.from)
		}
		first := len(flat)
		at := start
		for range pairs {
			flat = append(flat, at)
			for _, p := range pairs {
				if p.from == at {
					at = p.to
					break
				}
			}
		}
		g.Neighbors[v] = flat[first:len(flat):len(flat)]
	}
}

// buildAreas fills in Area from Faces.
func (g *Grid) buildAreas() {
	g.Area = make([]float64, len(g.Pos))
	for _, face := range g.Faces {
		third := TriangleArea(g.Pos[face[0]], g.Pos[face[1]], g.Pos[face[2]]) / 3
		for _, v := range face {
			g.Area[v] += third
		}
	}
}

// TriangleArea returns the area on the unit sphere of the triangle with
// corners a, b, and c, joined by great circles.
func TriangleArea(a, b, c Vec3) float64 {
	// Van Oosterom and Strackee (1983), "The Solid Angle of a Plane
	// Triangle", for corners of length one.
	return 2 * math.Atan2(math.Abs(Det(a, b, c)), 1+a.Dot(b)+b.Dot(c)+c.Dot(a))
}

// Locate returns the face that holds the point p, which must be on the unit
// sphere, and the weight of each of the face's three cells at p. The weights
// are not negative and sum to one; a field's value at p is the weighted sum
// of its values at the three cells. The weights change continuously as p
// crosses from one face to the next.
func (g *Grid) Locate(p Vec3) (face int32, weights [3]float64) {
	// The icosahedron's faces are the regions nearest to each face's
	// middle, so the face holding p is the one whose middle is nearest.
	top, best := 0, math.Inf(-1)
	for f, m := range g.baseMids {
		if d := p.Dot(m); d > best {
			top, best = f, d
		}
	}
	// The middle of a great-circle edge is on that edge, so a face's four
	// parts cover it exactly, and p can be followed down through the
	// splits. A part's index is its parent's times four, plus which part.
	a, b, c := g.base[top][0], g.base[top][1], g.base[top][2]
	face = int32(top)
	for range g.Detail {
		ab, bc, ca := midpoint(a, b), midpoint(b, c), midpoint(c, a)
		switch {
		case ab.Cross(ca).Dot(p) >= 0:
			face, b, c = face*4, ab, ca
		case bc.Cross(ab).Dot(p) >= 0:
			face, a, c = face*4+1, ab, bc
		case ca.Cross(bc).Dot(p) >= 0:
			face, a, b = face*4+2, ca, bc
		default:
			face, a, b, c = face*4+3, ab, bc, ca
		}
	}
	// The weights are p's barycentric coordinates in the flat triangle,
	// with p carried onto it along the line from the sphere's center. On an
	// edge the far corner's weight is zero and the other two depend only on
	// the edge, which is what makes the weights continuous.
	w := [3]float64{max(Det(p, b, c), 0), max(Det(a, p, c), 0), max(Det(a, b, p), 0)}
	sum := w[0] + w[1] + w[2]
	if !(sum > 0) {
		return face, [3]float64{1, 0, 0}
	}
	return face, [3]float64{w[0] / sum, w[1] / sum, w[2] / sum}
}

// Nearest returns the cell nearest to the point p, which must be on the
// unit sphere.
func (g *Grid) Nearest(p Vec3) int32 {
	face, _ := g.Locate(p)
	// Start from the nearest corner of the face that holds p, then walk
	// downhill in distance until no neighbor is nearer.
	cell, best := int32(-1), math.Inf(-1)
	for _, v := range g.Faces[face] {
		if d := p.Dot(g.Pos[v]); d > best {
			cell, best = v, d
		}
	}
	for {
		moved := false
		for _, v := range g.Neighbors[cell] {
			if d := p.Dot(g.Pos[v]); d > best {
				cell, best, moved = v, d, true
			}
		}
		if !moved {
			return cell
		}
	}
}

// Components labels the connected groups of cells for which member is true.
// Labels start at 1, in the order the groups are first met going up the
// cells, and 0 marks a cell that is not a member. The number of groups is
// count.
func (g *Grid) Components(member func(cell int32) bool) (label []int32, count int) {
	label = make([]int32, len(g.Pos))
	var stack []int32
	for start := range int32(len(g.Pos)) {
		if label[start] != 0 || !member(start) {
			continue
		}
		count++
		id := int32(count)
		label[start] = id
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			cell := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, v := range g.Neighbors[cell] {
				if label[v] == 0 && member(v) {
					label[v] = id
					stack = append(stack, v)
				}
			}
		}
	}
	return label, count
}
