// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestSpacingIsTheDistanceBetweenNeighbors(t *testing.T) {
	g := mustGrid(t, 4)
	var sum float64
	var n int
	for i, ns := range g.Neighbors {
		for _, j := range ns {
			sum += math.Acos(g.Pos[i].Dot(g.Pos[j]))
			n++
		}
	}
	mean := sum / float64(n)
	if got := g.Spacing(); math.Abs(got-mean) > 0.03*mean {
		t.Errorf("spacing %v, mean distance between neighbors %v", got, mean)
	}
	if got, want := g.Hops(5*g.Spacing()*RadiusKm), 5; got != want {
		t.Errorf("hops across five spacings: %d, want %d", got, want)
	}
	if got := g.Hops(1); got != 1 {
		t.Errorf("hops across a kilometre: %d, want 1", got)
	}
}

// The gradient of a field linear in x is the part of the x axis that lies
// along the ground.
func TestGradientOfALinearField(t *testing.T) {
	g := mustGrid(t, 4)
	for _, axis := range []Vec3{{X: 1}, {Y: 1}, {Z: 1}, Vec3{1, -2, 0.5}.Normalize()} {
		field := make([]float64, len(g.Pos))
		for i, p := range g.Pos {
			field[i] = p.Dot(axis)
		}
		for i, got := range g.Gradient(field) {
			p := g.Pos[i]
			want := axis.Sub(p.Scale(axis.Dot(p)))
			if d := got.Sub(want).Len(); d > 1e-9 {
				t.Fatalf("axis %v, cell %d: gradient %v, want %v (off by %v)", axis, i, got, want, d)
			}
			if along := got.Dot(p); math.Abs(along) > 1e-12 {
				t.Fatalf("cell %d: gradient leaves the ground by %v", i, along)
			}
		}
	}
}

func TestGradientOfAFlatFieldIsZero(t *testing.T) {
	g := mustGrid(t, 3)
	field := make([]float64, len(g.Pos))
	for i := range field {
		field[i] = 7
	}
	for i, v := range g.Gradient(field) {
		if v.Len() > 1e-12 {
			t.Fatalf("cell %d: gradient %v", i, v)
		}
	}
}

// meanAndVariance weighs each cell by its area.
func meanAndVariance(g *Grid, field []float64) (mean, variance float64) {
	var total float64
	for i, v := range field {
		mean += g.Area[i] * v
		total += g.Area[i]
	}
	mean /= total
	for i, v := range field {
		variance += g.Area[i] * (v - mean) * (v - mean)
	}
	return mean, variance / total
}

func TestDiffuseKeepsTheMeanAndLowersTheVariance(t *testing.T) {
	g := mustGrid(t, 4)
	rng := rand.New(rand.NewPCG(1, 2))
	field := make([]float64, len(g.Pos))
	for i := range field {
		field[i] = rng.NormFloat64()
	}
	mean, variance := meanAndVariance(g, field)
	for _, sigma := range []float64{0.01, 0.05, 0.3} {
		got := g.Diffuse(field, sigma)
		m, v := meanAndVariance(g, got)
		if math.Abs(m-mean) > 1e-12 {
			t.Errorf("sigma %v: mean went from %v to %v", sigma, mean, m)
		}
		if !(v < variance) {
			t.Errorf("sigma %v: variance went from %v to %v", sigma, variance, v)
		}
		variance = min(variance, v)
	}
	if got := g.Diffuse(field, 0); got[5] != field[5] {
		t.Error("no smoothing changed the field")
	}
}

// A spike spreads into a bell curve of the width asked for.
func TestDiffuseSpreadsBySigma(t *testing.T) {
	g := mustGrid(t, 5)
	const sigma = 0.15
	field := make([]float64, len(g.Pos))
	const start = 100
	field[start] = 1
	got := g.Diffuse(field, sigma)
	// On a plane the mean squared distance from the start would be
	// 2·sigma². The sphere's curve takes a little off.
	var sum, weight float64
	for i, v := range got {
		if v < 0 {
			t.Fatalf("cell %d went below zero: %v", i, v)
		}
		d := math.Acos(min(1, g.Pos[i].Dot(g.Pos[start])))
		sum += g.Area[i] * v * d * d
		weight += g.Area[i] * v
	}
	if rms, want := math.Sqrt(sum/weight), sigma*math.Sqrt2; math.Abs(rms-want) > 0.05*want {
		t.Errorf("spread %v, want about %v", rms, want)
	}
}

func randomSet(g *Grid, seed uint64) []bool {
	// A few caps, so the set has an inside and an edge.
	rng := rand.New(rand.NewPCG(seed, 3))
	set := make([]bool, len(g.Pos))
	for range 6 {
		c := LatLon(math.Asin(2*rng.Float64()-1), 2*math.Pi*rng.Float64())
		r := math.Cos(0.1 + 0.4*rng.Float64())
		for i, p := range g.Pos {
			if p.Dot(c) > r {
				set[i] = true
			}
		}
	}
	return set
}

func TestDilateThenErodeContainsTheSet(t *testing.T) {
	g := mustGrid(t, 4)
	for seed := range uint64(5) {
		set := randomSet(g, seed)
		for _, steps := range []int{1, 3} {
			grown := g.Dilate(set, steps)
			closed := g.Erode(grown, steps)
			shrunk := g.Erode(set, steps)
			for i := range set {
				if set[i] && !grown[i] {
					t.Fatalf("seed %d: dilation lost cell %d", seed, i)
				}
				if set[i] && !closed[i] {
					t.Fatalf("seed %d: dilation then erosion lost cell %d", seed, i)
				}
				if shrunk[i] && !set[i] {
					t.Fatalf("seed %d: erosion gained cell %d", seed, i)
				}
			}
		}
	}
}

func TestDilateByOneAddsExactlyTheNeighbors(t *testing.T) {
	g := mustGrid(t, 3)
	set := make([]bool, len(g.Pos))
	set[40] = true
	grown := g.Dilate(set, 1)
	want := map[int32]bool{40: true}
	for _, v := range g.Neighbors[40] {
		want[v] = true
	}
	for i, in := range grown {
		if in != want[int32(i)] {
			t.Errorf("cell %d: in the grown set is %v", i, in)
		}
	}
	if back := g.Erode(grown, 1); !back[40] {
		t.Error("erosion removed the middle of a cell and its neighbors")
	}
}

func TestFlood(t *testing.T) {
	g := mustGrid(t, 3)
	north := func(cell int32) bool { return g.Pos[cell].Z > 0.3 }
	got := g.Flood(0, north)
	for i := range got {
		if got[i] != north(int32(i)) {
			t.Fatalf("cell %d: flooded is %v", i, got[i])
		}
	}
	for i, in := range g.Flood(11, north) {
		if in {
			t.Fatalf("a flood from outside the set reached cell %d", i)
		}
	}
}

func TestRotation(t *testing.T) {
	// A quarter turn about z takes x to y.
	r := Rotation(Vec3{Z: math.Pi / 2})
	if got := r.Apply(Vec3{X: 1}); got.Sub(Vec3{Y: 1}).Len() > 1e-15 {
		t.Errorf("a quarter turn about z took x to %v", got)
	}
	if Rotation(Vec3{}) != Identity() {
		t.Error("no rotation is not the identity")
	}
	// A small rotation moves p by v × p.
	v := Vec3{1e-6, -2e-6, 3e-6}
	p := Vec3{0.6, 0, 0.8}
	moved := Rotation(v).Apply(p).Sub(p)
	if d := moved.Sub(v.Cross(p)).Len(); d > 1e-11 {
		t.Errorf("a small rotation moved p by %v, want %v", moved, v.Cross(p))
	}
}

func orthonormalError(m Mat3) float64 {
	p := m.Mul(m.Transpose())
	id := Identity()
	var worst float64
	for i := range 3 {
		worst = max(worst, p[i].Sub(id[i]).Len())
	}
	return worst
}

func TestRotationsStayOrthonormal(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	m := Identity()
	for range 10000 {
		v := Vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.Scale(0.05)
		m = m.Mul(Rotation(v)).Orthonormal()
	}
	if e := orthonormalError(m); e > 1e-14 {
		t.Errorf("after 10,000 turns the matrix is off by %v", e)
	}
	if d := Det(m[0], m[1], m[2]); math.Abs(d-1) > 1e-14 {
		t.Errorf("determinant %v, want 1", d)
	}
}

func TestMatrixProductAndSolve(t *testing.T) {
	a := Rotation(Vec3{0.3, -0.2, 0.9})
	b := Rotation(Vec3{-1.1, 0.4, 0.2})
	p := Vec3{0.1, 0.7, -0.4}
	if d := a.Mul(b).Apply(p).Sub(a.Apply(b.Apply(p))).Len(); d > 1e-15 {
		t.Errorf("the product is off by %v", d)
	}
	if d := a.Transpose().Apply(a.Apply(p)).Sub(p).Len(); d > 1e-15 {
		t.Errorf("the transpose does not undo a rotation: off by %v", d)
	}
	m := Mat3{{2, 1, 0}, {1, 3, 1}, {0, 1, 4}}
	x := m.Solve(p)
	if d := m.Apply(x).Sub(p).Len(); d > 1e-15 {
		t.Errorf("solve is off by %v", d)
	}
}
