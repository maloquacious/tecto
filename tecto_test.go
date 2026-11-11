// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tecto

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/tecto/internal/sphere"
)

// small is a world that takes a few milliseconds to build.
func small(seed uint64) Params {
	return Params{Seed: seed, Detail: 4, Columns: 64, Rows: 32}
}

func TestGenerateRejectsBadParams(t *testing.T) {
	bad := map[string]Params{
		"detail too low":    {Detail: -1},
		"detail too high":   {Detail: 99},
		"odd columns":       {Columns: 65},
		"too many columns":  {Columns: 1 << 20},
		"negative rows":     {Rows: -3},
		"negative time":     {Myr: -1},
		"time not finite":   {Myr: math.Inf(1)},
		"time not a number": {Myr: math.NaN()},
		// Until the plates move.
		"any time at all": {Myr: 100},
	}
	for name, p := range bad {
		if _, err := Generate(p); !errors.Is(err, ErrInvalidParams) {
			t.Errorf("%s: error %v, want ErrInvalidParams", name, err)
		}
	}
}

func TestGenerateDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a full-size world")
	}
	r, err := Generate(Params{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.Columns != DefaultColumns || r.Rows != DefaultRows || len(r.Heights) != DefaultColumns*DefaultRows {
		t.Errorf("%d by %d with %d heights", r.Columns, r.Rows, len(r.Heights))
	}
	if want := 10<<(2*DefaultDetail) + 2; r.Summary.Cells != want {
		t.Errorf("%d cells, want %d", r.Summary.Cells, want)
	}
}

func TestGenerateIsRepeatable(t *testing.T) {
	a, err := Generate(small(42))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(small(42))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Heights, b.Heights) || a.Summary != b.Summary {
		t.Error("the same parameters gave different worlds")
	}
	c, err := Generate(small(43))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(a.Heights, c.Heights) {
		t.Error("different seeds gave the same world")
	}
}

func TestGenerateRanges(t *testing.T) {
	for seed := range uint64(20) {
		p := small(seed)
		r, err := Generate(p)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if len(r.Heights) != p.Columns*p.Rows {
			t.Fatalf("seed %d: %d heights, want %d", seed, len(r.Heights), p.Columns*p.Rows)
		}
		lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
		for i, h := range r.Heights {
			if math.IsNaN(float64(h)) || math.IsInf(float64(h), 0) {
				t.Fatalf("seed %d: hex %d has height %v", seed, i, h)
			}
			lo, hi = min(lo, h), max(hi, h)
		}
		s := r.Summary
		// A hex's height lies between those of the cells around it.
		if float64(lo) < s.Min-0.01 || float64(hi) > s.Max+0.01 {
			t.Errorf("seed %d: hexes run from %v to %v, cells from %v to %v", seed, lo, hi, s.Min, s.Max)
		}
		// The starting crust is 40% continental, and the sea comes a
		// little way up the continental slope.
		if s.LandFraction < 0.33 || s.LandFraction > 0.43 {
			t.Errorf("seed %d: land fraction %.3f, want 0.33 to 0.43", seed, s.LandFraction)
		}
		if s.Landmasses < 1 {
			t.Errorf("seed %d: no land", seed)
		}
		if !slices.IsSorted([]float64{s.Min, s.P5, s.P25, s.P50, s.P75, s.P95, s.Max}) {
			t.Errorf("seed %d: percentiles out of order: %+v", seed, s)
		}
		for name, c := range map[string]struct{ got, lo, hi float64 }{
			"lowest":  {s.Min, -4500, -3300},
			"median":  {s.P50, -3500, -2700},
			"P75":     {s.P75, 1400, 2300},
			"P95":     {s.P95, 2500, 3500},
			"highest": {s.Max, 3500, 7000},
		} {
			if c.got < c.lo || c.got > c.hi {
				t.Errorf("seed %d: %s height %.0f m, want %.0f to %.0f", seed, name, c.got, c.lo, c.hi)
			}
		}
	}
}

func TestLargestLandmass(t *testing.T) {
	g, err := sphere.NewGrid(4)
	if err != nil {
		t.Fatal(err)
	}
	// A large island and a small one, well apart.
	big, little := sphere.LatLon(0.9, -2.0), sphere.LatLon(-0.5, 1.0)
	height := make([]float64, len(g.Pos))
	for cell, p := range g.Pos {
		switch {
		case p.Dot(big) > math.Cos(0.5), p.Dot(little) > math.Cos(0.3):
			height[cell] = 100
		default:
			height[cell] = -100
		}
	}
	center, count := largestLandmass(g, height)
	if count != 2 {
		t.Errorf("%d landmasses, want 2", count)
	}
	if d := math.Acos(center.Dot(big)); d > 0.02 {
		t.Errorf("center is %.3f radians from the middle of the large island", d)
	}

	// No land at all: the center is still a point on the sphere.
	for i := range height {
		height[i] = -100
	}
	center, count = largestLandmass(g, height)
	if count != 0 || math.Abs(center.Len()-1) > 1e-12 {
		t.Errorf("with no land: center %v, %d landmasses", center, count)
	}
}

func TestLargestLandmassIsInTheMiddle(t *testing.T) {
	// More of the map's middle is land than of its far side.
	var middle, far, n int
	for seed := range uint64(10) {
		p := small(seed)
		r, err := Generate(p)
		if err != nil {
			t.Fatal(err)
		}
		for row := p.Rows / 4; row < 3*p.Rows/4; row++ {
			for d := -p.Columns / 8; d < p.Columns/8; d++ {
				n++
				if r.Heights[row*p.Columns+p.Columns/2+d] > 0 {
					middle++
				}
				if r.Heights[row*p.Columns+(p.Columns+d)%p.Columns] > 0 {
					far++
				}
			}
		}
	}
	if middle <= far {
		t.Errorf("%d of %d hexes are land in the middle of the map, %d on the far side", middle, n, far)
	}
}
