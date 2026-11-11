// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package crust

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/maloquacious/tecto/internal/sphere"
)

func near(got, want, tolerance float64) bool { return math.Abs(got-want) <= tolerance }

func TestHandWorkedColumns(t *testing.T) {
	tests := []struct {
		name      string
		column    Column
		thickness float64
		elevation float64
	}{
		{
			// 7,100 m of new ocean floor: 7100 × 2890 = 20,519,000 kg.
			// It floats 7100 × (1 − 2890/3075) = 427.154 m up.
			name:      "new ocean floor",
			column:    Column{MaficVolcanic: 20_519_000},
			thickness: 7100,
			elevation: 427.154,
		},
		{
			// The same rock at 125 Myr is halfway to its greatest density:
			// 3095. It is 20,519,000 / 3095 = 6629.725 m thick, and denser
			// than the mantle, so it sits 6629.725 − 6672.846 = −43.121 m.
			name:      "old ocean floor",
			column:    Column{MaficVolcanic: 20_519_000, Age: 125},
			thickness: 6629.725,
			elevation: -43.121,
		},
		{
			// Past its lifetime the density stays at 3300.
			name:      "ancient ocean floor",
			column:    Column{MaficVolcanic: 10_000_000, MaficPlutonic: 10_519_000, Age: 900},
			thickness: 6217.879,
			elevation: -454.967,
		},
		{
			// 36,900 m of continental rock: 36900 × 2600 = 95,940,000 kg,
			// floating 36900 × (1 − 2600/3075) = 5700 m up.
			name:      "continent",
			column:    Column{FelsicPlutonic: 81_549_000, FelsicVolcanic: 14_391_000, Age: 1000},
			thickness: 36900,
			elevation: 5700,
		},
		{
			// A metre of each of the other rocks on the ocean floor adds
			// 3 m, and (1500 + 2600 + 2800) / 3075 = 2.244 m of sinking.
			name: "every rock",
			column: Column{
				Sediment: 1500, Sedimentary: 2600, Metamorphic: 2800,
				MaficVolcanic: 20_519_000,
			},
			thickness: 7103,
			elevation: 427.910,
		},
	}
	for _, tc := range tests {
		if got := tc.column.Thickness(); !near(got, tc.thickness, 0.001) {
			t.Errorf("%s: thickness %.3f, want %.3f", tc.name, got, tc.thickness)
		}
		if got := tc.column.Elevation(); !near(got, tc.elevation, 0.001) {
			t.Errorf("%s: elevation %.3f, want %.3f", tc.name, got, tc.elevation)
		}
		// Elevation is thickness × (1 − density / mantle density).
		want := tc.column.Thickness() * (1 - tc.column.Density()/MantleDensity)
		if got := tc.column.Elevation(); !near(got, want, 1e-6) {
			t.Errorf("%s: elevation %v, but the isostatic formula gives %v", tc.name, got, want)
		}
	}
}

func TestUniformCrustIsFlat(t *testing.T) {
	columns := make([]Column, 100)
	for i := range columns {
		columns[i] = ColumnAt(300)
	}
	elevation := Elevations(columns)
	if lo, hi := slices.Min(elevation), slices.Max(elevation); lo != hi {
		t.Errorf("uniform crust runs from %v to %v", lo, hi)
	}
}

func TestColumnAt(t *testing.T) {
	// The deep ocean floor: ocean crust and nothing else.
	c := ColumnAt(-3200)
	if want := (Column{MaficVolcanic: 7100 * 2890}); c != want {
		t.Errorf("at -3200 m: %+v, want %+v", c, want)
	}
	// A continent at 840 m: 36,900 m of continental rock, 85% of it
	// plutonic, under 5 m of sediment, a thousand million years old.
	c = ColumnAt(840)
	if !near(c.FelsicPlutonic, 0.85*36900*2600, 1) || !near(c.FelsicVolcanic, 0.15*36900*2600, 1) ||
		c.Sediment != 5*1500 || c.MaficVolcanic != 0 || c.Age != 1000 {
		t.Errorf("at 840 m: %+v", c)
	}
	if !near(c.Thickness(), 36905, 1e-6) {
		t.Errorf("at 840 m: thickness %v, want 36905", c.Thickness())
	}
	// Halfway down the continental slope, half of each.
	c = ColumnAt(-1225)
	if !near(c.MaficVolcanic, 3550*2890, 1) || !near(c.FelsicPlutonic+c.FelsicVolcanic, 14150*2600, 1) {
		t.Errorf("at -1225 m: %+v", c)
	}

	// Every target has crust, and higher targets float higher.
	last := math.Inf(-1)
	var columns []Column
	for e := -12000.0; e <= 10000; e += 50 {
		c := ColumnAt(e)
		columns = append(columns, c)
		got := c.Elevation()
		// From the foot of the continental slope up, a higher target must
		// not float lower. The ocean floor is left out: there the depth
		// goes by the crust's age, and the old floor beside the continents
		// lies deeper than the young floor at the ridges.
		if e > -1500 && got < last {
			t.Errorf("target %v floats at %v, below the %v of the target before", e, got, last)
		}
		last = got
	}
	if err := Check(columns); err != nil {
		t.Error(err)
	}
}

func TestCheck(t *testing.T) {
	good := ColumnAt(0)
	bad := map[string]Column{
		"no crust":      {},
		"negative mass": {FelsicPlutonic: 1e7, Sediment: -1},
		"not a number":  {FelsicPlutonic: math.NaN()},
		"infinite":      {FelsicPlutonic: 1e7, Age: math.Inf(1)},
	}
	if err := Check([]Column{good, good}); err != nil {
		t.Errorf("good columns: %v", err)
	}
	for name, c := range bad {
		if err := Check([]Column{good, c}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestSeaLevelHoldsTheVolume(t *testing.T) {
	g, err := sphere.NewGrid(3)
	if err != nil {
		t.Fatal(err)
	}
	columns := Initial(g.Pos, rand.New(rand.NewPCG(1, 1)), rand.New(rand.NewPCG(1, 2)))
	elevation := Elevations(columns)
	for _, depth := range []float64{0, 1, 500, OceanMeanDepth, 20000} {
		level, err := SeaLevel(elevation, g.Area, depth)
		if err != nil {
			t.Fatal(err)
		}
		if got := MeanDepth(elevation, g.Area, level); !near(got, depth, 1e-6) {
			t.Errorf("asked for a mean depth of %v, level %v gives %v", depth, level, got)
		}
	}
	// With no water the level is at the lowest ground.
	level, _ := SeaLevel(elevation, g.Area, 0)
	if lowest := slices.Min(elevation); !near(level, lowest, 1e-6) {
		t.Errorf("empty sea at %v, lowest ground at %v", level, lowest)
	}
}

func TestSeaLevelByHand(t *testing.T) {
	// Two cells of equal area, at 0 and 100 m. A mean depth of 25 m is
	// 50 m over the low cell. A mean depth of 150 m covers both: the
	// level is at 200, with 200 and 100 m of water.
	elevation, area := []float64{0, 100}, []float64{1, 1}
	for depth, want := range map[float64]float64{25: 50, 150: 200} {
		got, err := SeaLevel(elevation, area, depth)
		if err != nil || !near(got, want, 1e-9) {
			t.Errorf("mean depth %v: level %v, %v; want %v", depth, got, err, want)
		}
	}
	// The low cell three times the size: 30 m of mean depth is 40 m on it.
	if got, _ := SeaLevel(elevation, []float64{3, 1}, 30); !near(got, 40, 1e-9) {
		t.Errorf("unequal areas: level %v, want 40", got)
	}
	for _, e := range [][]float64{nil, {math.NaN(), 1}} {
		if _, err := SeaLevel(e, area[:len(e)], 10); err == nil {
			t.Errorf("elevations %v: no error", e)
		}
	}
}

func TestRandomRanks(t *testing.T) {
	g, err := sphere.NewGrid(3)
	if err != nil {
		t.Fatal(err)
	}
	a := RandomRanks(g.Pos, rand.New(rand.NewPCG(7, 1)))
	b := RandomRanks(g.Pos, rand.New(rand.NewPCG(7, 1)))
	if !slices.Equal(a, b) {
		t.Error("the same source gave different ranks")
	}
	c := RandomRanks(g.Pos, rand.New(rand.NewPCG(8, 1)))
	if slices.Equal(a, c) {
		t.Error("different sources gave the same ranks")
	}
	// The field is smooth: neighbors differ by far less than the field's
	// whole range. It is not flat.
	lo, hi := slices.Min(a), slices.Max(a)
	if hi-lo < 10 {
		t.Fatalf("ranks run only from %v to %v", lo, hi)
	}
	for cell, ns := range g.Neighbors {
		for _, n := range ns {
			if d := math.Abs(a[cell] - a[n]); d > (hi-lo)/2 {
				t.Fatalf("cells %d and %d are neighbors and differ by %v of a range of %v", cell, n, d, hi-lo)
			}
		}
	}
}

func TestTargetElevations(t *testing.T) {
	ranks := []float64{3, 1, 2, 1, 5}
	got := TargetElevations(ranks, rand.New(rand.NewPCG(1, 1)))
	// Elevations follow rank order, and equal ranks follow cell order.
	for _, pair := range [][2]int{{1, 3}, {3, 2}, {2, 0}, {0, 4}} {
		if got[pair[0]] > got[pair[1]] {
			t.Errorf("cell %d (rank %v) is above cell %d (rank %v): %v", pair[0], ranks[pair[0]], pair[1], ranks[pair[1]], got)
		}
	}

	// Over many cells, the share below the gap between the two humps is
	// the ocean's share.
	rng := rand.New(rand.NewPCG(2, 2))
	big := make([]float64, 20000)
	for i := range big {
		big[i] = rng.Float64()
	}
	elevation := TargetElevations(big, rand.New(rand.NewPCG(3, 3)))
	below := 0
	for _, e := range elevation {
		if e < -1500 {
			below++
		}
	}
	if share := float64(below) / float64(len(big)); !near(share, 0.6, 0.03) {
		t.Errorf("%.3f of the cells are ocean floor, want about 0.6", share)
	}
}

func TestBuoyancy(t *testing.T) {
	young := NewOcean()
	if got := young.Buoyancy(); got != 0 {
		t.Errorf("new ocean crust: buoyancy %v, want 0, since it floats", got)
	}
	if got, want := young.Thickness(), OceanCrustThickness; math.Abs(got-want) > 1e-9 {
		t.Errorf("new ocean crust is %v m thick, want %v", got, want)
	}
	old := young
	old.Age = MaficLifetime
	if got, want := old.Buoyancy(), Gravity*(MantleDensity-MaficDensityOld); math.Abs(got-want) > 1e-9 {
		t.Errorf("old ocean crust: buoyancy %v, want %v", got, want)
	}
	if got := ColumnAt(500).Buoyancy(); got != 0 {
		t.Errorf("a continent: buoyancy %v, want 0", got)
	}
}

func TestContinentalLeavesOutTheOceanFloor(t *testing.T) {
	c := Column{Sediment: 1, Sedimentary: 2, Metamorphic: 4, FelsicPlutonic: 8, FelsicVolcanic: 16, MaficVolcanic: 32, MaficPlutonic: 64, Age: 100}
	if got := c.Continental(); got != 31 {
		t.Errorf("continental rock %v, want 31", got)
	}
	if got := NewOcean().Continental(); got != 0 {
		t.Errorf("new ocean floor holds %v of continental rock", got)
	}
	if err := c.Check(); err != nil {
		t.Error(err)
	}
	if err := (Column{}).Check(); err == nil {
		t.Error("a column with no rock passes the check")
	}
}
