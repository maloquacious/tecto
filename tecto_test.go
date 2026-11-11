// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tecto

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/hexmap"
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

// The plates leave the world much as Earth is: the same share of it land,
// the ocean floor and the continents at the same levels, and the highest
// ground higher, where crust has been stacked.
func TestGenerateRangesWithPlates(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the plates for 300 million years on twenty seeds")
	}
	for seed := range uint64(20) {
		r, err := Generate(Params{Seed: seed, Myr: 300, Detail: 5, Columns: 64, Rows: 32})
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		s := r.Summary
		if !slices.IsSorted([]float64{s.Min, s.P5, s.P25, s.P50, s.P75, s.P95, s.Max}) {
			t.Errorf("seed %d: percentiles out of order: %+v", seed, s)
		}
		for _, c := range []struct {
			name        string
			got, lo, hi float64
		}{
			{"land fraction", s.LandFraction, 0.33, 0.41},
			{"lowest height", s.Min, -4200, -3300},
			{"median height", s.P50, -3300, -2600},
			{"P75 height", s.P75, 1600, 2400},
			{"P95 height", s.P95, 2800, 4100},
			{"highest height", s.Max, 5000, 20000},
			{"overlap", s.OverlapFraction, 0, 0.15},
			// Arcs add rock, a percent or so for each hundred million
			// years, and nothing takes much away.
			{"continental rock, as a share of the start", s.Continental.End / s.Continental.Start, 1, 1.06},
		} {
			if !(c.got >= c.lo && c.got <= c.hi) {
				t.Errorf("seed %d: %s is %.4g, want %.4g to %.4g", seed, c.name, c.got, c.lo, c.hi)
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

func TestMapFrameKeepsLandFromThePoles(t *testing.T) {
	g, err := sphere.NewGrid(4)
	if err != nil {
		t.Fatal(err)
	}
	limit := math.Sin(polarLatitude * math.Pi / 180)
	// A continent in the middle of the map, and an island where the map's
	// north pole would be if nothing were done about it.
	center, pole := sphere.Vec3{X: 1}, sphere.Vec3{Z: 1}
	height := make([]float64, len(g.Pos))
	for cell, p := range g.Pos {
		height[cell] = -1000
		if p.Dot(center) > math.Cos(0.5) || p.Dot(pole) > math.Cos(0.15) {
			height[cell] = 500
		}
	}
	plain := hexmap.FrameAt(center)
	if polarLand(g, height, plain.North, limit) == 0 {
		t.Fatal("the island is not at the pole of the plain frame")
	}
	f := mapFrame(g, height, center)
	if f.Center != center {
		t.Errorf("the middle of the map is %v, want %v", f.Center, center)
	}
	if polar := polarLand(g, height, f.North, limit); polar != 0 {
		t.Errorf("%v steradians of land are left at the poles, and there is room to leave none", polar)
	}

	// With no land at the poles the map is not turned.
	for cell, p := range g.Pos {
		if p.Dot(pole) > math.Cos(0.15) {
			height[cell] = -1000
		}
	}
	if f := mapFrame(g, height, center); f != plain {
		t.Errorf("the map was turned to %+v with no land at its poles, want %+v", f, plain)
	}
}

// However the crust lies, turning the map leaves no more land at the poles
// than not turning it.
func TestGenerateKeepsLandFromThePoles(t *testing.T) {
	limit := math.Sin(polarLatitude * math.Pi / 180)
	g, err := sphere.NewGrid(4)
	if err != nil {
		t.Fatal(err)
	}
	var turned, plain float64
	for seed := range uint64(20) {
		columns := crust.Initial(g.Pos, stream(seed, streamLayout), stream(seed, streamElevations))
		height := crust.Elevations(columns)
		sea, err := crust.SeaLevel(height, g.Area, crust.OceanMeanDepth)
		if err != nil {
			t.Fatal(err)
		}
		for i := range height {
			height[i] -= sea
		}
		center, _ := largestLandmass(g, height)
		a := polarLand(g, height, mapFrame(g, height, center).North, limit)
		b := polarLand(g, height, hexmap.FrameAt(center).North, limit)
		if a > b {
			t.Errorf("seed %d: %v steradians of land at the poles, and %v with the map not turned", seed, a, b)
		}
		turned += a
		plain += b
	}
	if turned > plain/2 {
		t.Errorf("over twenty seeds %v steradians of land are at the poles, and %v with the map not turned: want under half", turned, plain)
	}
}

func TestGenerateAtZeroDoesNotRunThePlates(t *testing.T) {
	r, err := Generate(small(3))
	if err != nil {
		t.Fatal(err)
	}
	if s := r.Summary; s.Plates != 0 || s.OverlapFraction != 0 || s.Continental != (Budget{}) ||
		s.HighGroundToMarginKm != 0 || s.LandToMarginKm != 0 || s.HighGroundElongation < 1 {
		t.Errorf("a run of no time reports plates: %+v", s)
	}
}

// A run with time in it is repeatable, differs from the starting crust,
// leaves no cell without crust, and accounts for its continental rock.
func TestGenerateWithPlates(t *testing.T) {
	for seed := range uint64(5) {
		start, err := Generate(small(seed))
		if err != nil {
			t.Fatal(err)
		}
		p := small(seed)
		p.Myr = 150
		r, err := Generate(p)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		again, err := Generate(p)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(r.Heights, again.Heights) || r.Summary != again.Summary {
			t.Errorf("seed %d: the same parameters gave different worlds", seed)
		}
		if slices.Equal(r.Heights, start.Heights) {
			t.Errorf("seed %d: 150 million years changed nothing", seed)
		}
		for i, h := range r.Heights {
			if math.IsNaN(float64(h)) || math.IsInf(float64(h), 0) {
				t.Fatalf("seed %d: hex %d has height %v", seed, i, h)
			}
		}
		s := r.Summary
		if s.Plates < 2 || s.Plates > 7 {
			t.Errorf("seed %d: %d plates, want 2 to 7", seed, s.Plates)
		}
		if !(s.OverlapFraction > 0 && s.OverlapFraction < 0.5) {
			t.Errorf("seed %d: overlaps over %v of the sphere", seed, s.OverlapFraction)
		}
		b := s.Continental
		if !(b.Start > 0 && b.Subducted >= b.Accreted && b.Accreted >= 0) {
			t.Errorf("seed %d: continental rock: %+v", seed, b)
		}
		if sum := b.Start - b.Subducted + b.Accreted + b.Erupted + b.Resampled; math.Abs(sum-b.End) > 1e-9*b.Start {
			t.Errorf("seed %d: continental rock ends at %v, but the account comes to %v", seed, b.End, sum)
		}
		if math.Abs(b.End-b.Start) > 0.05*b.Start {
			t.Errorf("seed %d: continental rock went from %v to %v", seed, b.Start, b.End)
		}
		if s.LandFraction <= 0 || s.LandFraction >= 1 || s.Landmasses < 1 {
			t.Errorf("seed %d: land fraction %v in %d landmasses", seed, s.LandFraction, s.Landmasses)
		}
		if !slices.IsSorted([]float64{s.Min, s.P5, s.P25, s.P50, s.P75, s.P95, s.Max}) {
			t.Errorf("seed %d: percentiles out of order: %+v", seed, s)
		}
		if !(s.HighGroundElongation >= 1 && s.HighGroundToMarginKm >= 0 && s.LandToMarginKm > 0) {
			t.Errorf("seed %d: high ground %v times as long as wide, %v km from a margin, and land %v km",
				seed, s.HighGroundElongation, s.HighGroundToMarginKm, s.LandToMarginKm)
		}

		// A very short run moves nothing far.
		p.Myr = 0.5
		brief, err := Generate(p)
		if err != nil {
			t.Fatal(err)
		}
		if b := brief.Summary; b.Plates < 2 || b.Plates > 7 || b.OverlapFraction > 0.05 {
			t.Errorf("seed %d: after half a million years: %+v", seed, b)
		}
	}
}
