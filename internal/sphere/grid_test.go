// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func mustGrid(t *testing.T, detail int) *Grid {
	t.Helper()
	g, err := NewGrid(detail)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// randomPoint returns a point drawn evenly from the unit sphere.
func randomPoint(rng *rand.Rand) Vec3 {
	z := 2*rng.Float64() - 1
	r := math.Sqrt(1 - z*z)
	sin, cos := math.Sincos(2 * math.Pi * rng.Float64())
	return Vec3{r * cos, r * sin, z}
}

func TestNewGridRejectsBadDetail(t *testing.T) {
	for _, detail := range []int{MinDetail - 1, MaxDetail + 1} {
		if _, err := NewGrid(detail); err == nil {
			t.Errorf("detail %d: no error", detail)
		}
	}
}

func TestGridTopology(t *testing.T) {
	for detail := 1; detail <= 5; detail++ {
		g := mustGrid(t, detail)
		v := len(g.Pos)
		f := len(g.Faces)
		if want := 10<<(2*detail) + 2; v != want {
			t.Errorf("detail %d: %d cells, want %d", detail, v, want)
		}

		// Each edge is seen from both of its ends.
		ends, five := 0, 0
		for cell, ns := range g.Neighbors {
			ends += len(ns)
			switch len(ns) {
			case 5:
				five++
			case 6:
			default:
				t.Fatalf("detail %d: cell %d has %d neighbors", detail, cell, len(ns))
			}
			for _, n := range ns {
				if n == int32(cell) {
					t.Fatalf("detail %d: cell %d is its own neighbor", detail, cell)
				}
				if !slices.Contains(g.Neighbors[n], int32(cell)) {
					t.Fatalf("detail %d: cell %d has neighbor %d, but not the other way", detail, cell, n)
				}
			}
			if sorted := slices.Sorted(slices.Values(ns)); len(slices.Compact(sorted)) != len(ns) {
				t.Fatalf("detail %d: cell %d lists a neighbor twice: %v", detail, cell, ns)
			}
		}
		e := ends / 2
		if v-e+f != 2 {
			t.Errorf("detail %d: V - E + F = %d - %d + %d = %d, want 2", detail, v, e, f, v-e+f)
		}
		if five != 12 {
			t.Errorf("detail %d: %d cells with five neighbors, want 12", detail, five)
		}
	}
}

func TestGridGeometry(t *testing.T) {
	g := mustGrid(t, 4)
	for cell, p := range g.Pos {
		if d := math.Abs(p.Len() - 1); d > 1e-12 {
			t.Fatalf("cell %d is %g off the unit sphere", cell, d)
		}
	}
	for f, face := range g.Faces {
		if Det(g.Pos[face[0]], g.Pos[face[1]], g.Pos[face[2]]) <= 0 {
			t.Fatalf("face %d does not run counterclockwise", f)
		}
	}
	// Neighbors run counterclockwise: each one to the next turns the same
	// way around the cell.
	for cell, ns := range g.Neighbors {
		for k, n := range ns {
			next := ns[(k+1)%len(ns)]
			if Det(g.Pos[cell], g.Pos[n], g.Pos[next]) <= 0 {
				t.Fatalf("cell %d: neighbors %d then %d do not run counterclockwise", cell, n, next)
			}
		}
		if ns[0] != slices.Min(ns) {
			t.Fatalf("cell %d: neighbors do not start from the lowest: %v", cell, ns)
		}
	}
}

func TestGridAreas(t *testing.T) {
	for detail := 1; detail <= 5; detail++ {
		g := mustGrid(t, detail)
		sum := 0.0
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, a := range g.Area {
			sum += a
			lo, hi = min(lo, a), max(hi, a)
		}
		if d := math.Abs(sum - 4*math.Pi); d > 1e-9 {
			t.Errorf("detail %d: areas sum to 4π %+g", detail, sum-4*math.Pi)
		}
		// The cells are not equal, but none is far from the others.
		if lo <= 0 || hi/lo > 2 {
			t.Errorf("detail %d: cell areas run from %g to %g", detail, lo, hi)
		}
	}
}

func TestTriangleArea(t *testing.T) {
	// An eighth of the sphere.
	got := TriangleArea(Vec3{1, 0, 0}, Vec3{0, 1, 0}, Vec3{0, 0, 1})
	if d := math.Abs(got - math.Pi/2); d > 1e-12 {
		t.Errorf("octant area %g, want π/2", got)
	}
}

func TestLocateAgainstBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for detail := 1; detail <= 5; detail++ {
		g := mustGrid(t, detail)
		for range 2000 {
			p := randomPoint(rng)
			face, w := g.Locate(p)

			// The brute-force search: the face p is deepest inside.
			want, depth := -1, math.Inf(-1)
			for f, fc := range g.Faces {
				a, b, c := g.Pos[fc[0]], g.Pos[fc[1]], g.Pos[fc[2]]
				if d := min(Det(p, b, c), Det(a, p, c), Det(a, b, p)); d > depth {
					want, depth = f, d
				}
			}
			if int(face) != want {
				// On an edge either face will do. Anywhere else is wrong.
				fc := g.Faces[face]
				a, b, c := g.Pos[fc[0]], g.Pos[fc[1]], g.Pos[fc[2]]
				if d := min(Det(p, b, c), Det(a, p, c), Det(a, b, p)); d < -1e-12 {
					t.Fatalf("detail %d: %v located in face %d (%g outside), brute force says %d", detail, p, face, -d, want)
				}
			}

			sum := 0.0
			var q Vec3
			for k, wk := range w {
				if wk < 0 {
					t.Fatalf("detail %d: negative weight %g", detail, wk)
				}
				sum += wk
				q = q.Add(g.Pos[g.Faces[face][k]].Scale(wk))
			}
			if math.Abs(sum-1) > 1e-12 {
				t.Fatalf("detail %d: weights sum to %g", detail, sum)
			}
			// The weighted corners give back p's direction.
			if d := q.Normalize().Sub(p).Len(); d > 1e-9 {
				t.Fatalf("detail %d: weights place %v at %v", detail, p, q.Normalize())
			}
		}
	}
}

func TestLocateAtCells(t *testing.T) {
	g := mustGrid(t, 3)
	for cell, p := range g.Pos {
		face, w := g.Locate(p)
		k := slices.Index(g.Faces[face][:], int32(cell))
		if k < 0 {
			t.Fatalf("cell %d located in face %d, which it is not a corner of", cell, face)
		}
		if math.Abs(w[k]-1) > 1e-9 {
			t.Fatalf("cell %d has weight %g at its own position", cell, w[k])
		}
	}
}

func TestNearestAgainstBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for detail := 1; detail <= 5; detail++ {
		g := mustGrid(t, detail)
		for range 2000 {
			p := randomPoint(rng)
			got := g.Nearest(p)
			best := math.Inf(-1)
			for _, q := range g.Pos {
				best = max(best, p.Dot(q))
			}
			// Two cells can be equally near; either is right.
			if d := best - p.Dot(g.Pos[got]); d > 1e-12 {
				t.Fatalf("detail %d: nearest to %v is cell %d, but another is nearer by %g", detail, p, got, d)
			}
		}
	}
	g := mustGrid(t, 3)
	for cell, p := range g.Pos {
		if got := g.Nearest(p); got != int32(cell) {
			t.Fatalf("nearest to cell %d is cell %d", cell, got)
		}
	}
}

func TestComponents(t *testing.T) {
	g := mustGrid(t, 3)
	// Two caps and the band between them left out.
	member := func(cell int32) bool { return math.Abs(g.Pos[cell].Z) > 0.5 }
	label, count := g.Components(member)
	if count != 2 {
		t.Fatalf("%d components, want 2", count)
	}
	for cell, l := range label {
		z := g.Pos[cell].Z
		switch {
		case z > 0.5 && l != 1, z < -0.5 && l != 2, math.Abs(z) <= 0.5 && l != 0:
			t.Fatalf("cell %d at z=%g has label %d", cell, z, l)
		}
	}
	if _, count := g.Components(func(int32) bool { return true }); count != 1 {
		t.Errorf("the whole sphere is %d components, want 1", count)
	}
}
