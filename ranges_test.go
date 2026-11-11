// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tecto

import (
	"math"
	"testing"

	"github.com/maloquacious/tecto/internal/sphere"
)

func rangesGrid(t *testing.T) *sphere.Grid {
	t.Helper()
	g, err := sphere.NewGrid(5)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// latLon returns a cell's latitude and longitude, in radians.
func latLon(p sphere.Vec3) (lat, lon float64) {
	return math.Asin(p.Z), math.Atan2(p.Y, p.X)
}

func TestHighGroundIsTheHighestTenthOfTheLand(t *testing.T) {
	g := rangesGrid(t)
	// Land is the northern hemisphere, and rises to the pole.
	height := make([]float64, len(g.Pos))
	var land float64
	for cell, p := range g.Pos {
		height[cell] = p.Z
		if p.Z > 0 {
			land += g.Area[cell]
		}
	}
	high := highGround(g, height)
	var area, lowest float64 = 0, 1
	for cell, h := range high {
		if h {
			area += g.Area[cell]
			lowest = min(lowest, height[cell])
		}
	}
	if math.Abs(area/land-highShare) > 0.01 {
		t.Errorf("high ground is %v of the land, want %v", area/land, highShare)
	}
	for cell, h := range high {
		if !h && height[cell] > lowest {
			t.Fatalf("cell %d, at %v, is left out, and the high ground goes down to %v", cell, height[cell], lowest)
		}
	}
	if got := highGround(g, make([]float64, len(g.Pos))); elongation(g, got) != 0 {
		t.Error("a world with no land has high ground")
	}
}

func TestElongation(t *testing.T) {
	g := rangesGrid(t)
	shape := func(in func(lat, lon float64) bool) []bool {
		high := make([]bool, len(g.Pos))
		for cell, p := range g.Pos {
			high[cell] = in(latLon(p))
		}
		return high
	}
	for _, tc := range []struct {
		name string
		in   func(lat, lon float64) bool
		want float64
	}{
		{"a round piece", func(lat, lon float64) bool { return math.Hypot(lat, lon) < 0.3 }, 1},
		{"a bar along the equator", func(lat, lon float64) bool { return math.Abs(lat) < 0.1 && math.Abs(lon) < 0.4 }, 4},
		{"a bar up a meridian", func(lat, lon float64) bool { return math.Abs(lat) < 0.45 && math.Abs(lon) < 0.15 }, 3},
		// Two pieces count by their areas: the bar has four times the
		// area of the square.
		{"a bar and a square", func(lat, lon float64) bool {
			return math.Abs(lat) < 0.1 && (math.Abs(lon) < 0.4 || math.Abs(lon-2) < 0.1)
		}, (4*4 + 1) / 5.0},
		{"nothing", func(lat, lon float64) bool { return false }, 0},
		// One cell is too small to have a shape.
		{"a speck", func(lat, lon float64) bool { return math.Hypot(lat, lon) < 0.01 }, 0},
	} {
		got := elongation(g, shape(tc.in))
		if math.Abs(got-tc.want) > 0.12*max(tc.want, 1) {
			t.Errorf("%s: %v times as long as wide, want %v", tc.name, got, tc.want)
		}
	}
}

func TestMeanDistance(t *testing.T) {
	g := rangesGrid(t)
	north := sphere.Vec3{Z: 1}
	marked := make([]bool, len(g.Pos))
	near := make([]bool, len(g.Pos))
	far := make([]bool, len(g.Pos))
	for cell, p := range g.Pos {
		angle := math.Acos(p.Dot(north))
		marked[cell] = angle < 0.2
		near[cell] = angle < 0.3
		far[cell] = angle > 1.0 && angle < 1.1
	}
	if got := meanDistanceKm(g, marked, marked); got != 0 {
		t.Errorf("the marked cells are %v km from themselves", got)
	}
	// Steps from cell to cell do not run straight, so the distance is
	// over the true one by up to about a sixth.
	want := (1.05 - 0.2) * sphere.RadiusKm
	if got := meanDistanceKm(g, marked, far); got < want || got > 1.2*want {
		t.Errorf("a ring is %v km from the cap, want about %v", got, want)
	}
	if a, b := meanDistanceKm(g, marked, near), meanDistanceKm(g, marked, far); a >= b {
		t.Errorf("the near cells are %v km away and the far ones %v", a, b)
	}
	if got := meanDistanceKm(g, make([]bool, len(g.Pos)), far); got != 0 {
		t.Errorf("%v km from nothing", got)
	}
	if got := meanDistanceKm(g, marked, make([]bool, len(g.Pos))); got != 0 {
		t.Errorf("no cells are %v km away", got)
	}
}
