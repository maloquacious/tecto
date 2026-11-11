// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package erosion

import (
	"math"
	"testing"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

func mustGrid(t *testing.T, detail int) *sphere.Grid {
	t.Helper()
	g, err := sphere.NewGrid(detail)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// flat returns a world of level land.
func flat(g *sphere.Grid) []crust.Column {
	columns := make([]crust.Column, len(g.Pos))
	for i := range columns {
		columns[i] = crust.ColumnAt(500)
	}
	return columns
}

// rock returns the continental rock of each kind in the world, in kilograms
// for a sphere of radius one.
func rock(g *sphere.Grid, columns []crust.Column) [5]float64 {
	var sum [5]float64
	for cell, c := range columns {
		for k, v := range [...]float64{c.Sediment, c.Sedimentary, c.Metamorphic, c.FelsicPlutonic, c.FelsicVolcanic} {
			sum[k] += v * g.Area[cell]
		}
	}
	return sum
}

func TestFlatLandDoesNotChange(t *testing.T) {
	g := mustGrid(t, 3)
	columns := flat(g)
	sea := columns[0].Elevation() - 100
	if flows := Flows(g, columns, sea, 5); len(flows) != 0 {
		t.Errorf("%d flows on level land, want none", len(flows))
	}
}

func TestPeakGetsLowerAndItsNeighborsHigher(t *testing.T) {
	g := mustGrid(t, 3)
	columns := flat(g)
	const peak = 100
	columns[peak].FelsicPlutonic *= 2
	columns[peak].Sediment = 5e5
	before := crust.Elevations(columns)
	had := rock(g, columns)
	sea := before[0] - 100

	flows := Flows(g, columns, sea, 5)
	if len(flows) != len(g.Neighbors[peak]) {
		t.Fatalf("%d flows, want one to each of the peak's %d neighbors", len(flows), len(g.Neighbors[peak]))
	}
	Apply(g, columns, flows)
	after := crust.Elevations(columns)
	if !(after[peak] < before[peak]) {
		t.Errorf("the peak went from %v m to %v m", before[peak], after[peak])
	}
	beside := make(map[int32]bool)
	for _, v := range g.Neighbors[peak] {
		beside[v] = true
		if !(after[v] > before[v]) {
			t.Errorf("cell %d, beside the peak, went from %v m to %v m", v, before[v], after[v])
		}
		if !(after[v] < after[peak]) {
			t.Errorf("cell %d, beside the peak, is now at %v m and the peak at %v m", v, after[v], after[peak])
		}
	}
	for cell := range columns {
		if cell != peak && !beside[int32(cell)] && after[cell] != before[cell] {
			t.Errorf("cell %d, away from the peak, has changed", cell)
		}
	}
	// Each kind of rock is kept, and each moves.
	has := rock(g, columns)
	for k := range had {
		if math.Abs(has[k]-had[k]) > 1e-12*had[k] {
			t.Errorf("rock %d went from %v to %v", k, had[k], has[k])
		}
	}
	if !(columns[peak].Sediment < 5e5) {
		t.Errorf("the peak has kept its sediment")
	}
	if err := crust.Check(columns); err != nil {
		t.Error(err)
	}
}

func TestErosionSpeedsWithTheDropAndTheTime(t *testing.T) {
	g := mustGrid(t, 3)
	share := func(extra, myr float64) float64 {
		columns := flat(g)
		columns[100].FelsicPlutonic += extra
		sea := columns[0].Elevation() - 100
		var sum float64
		for _, f := range Flows(g, columns, sea, myr) {
			sum += f.Share * columns[100].Continental()
		}
		return sum
	}
	one := share(1e6, 1)
	if !(one > 0) {
		t.Fatal("nothing moved")
	}
	if got := share(2e6, 1); math.Abs(got-2*one) > 1e-9*one {
		t.Errorf("twice the drop moved %v kg, want twice %v", got, one)
	}
	if got := share(1e6, 3); math.Abs(got-3*one) > 1e-9*one {
		t.Errorf("three times as long moved %v kg, want three times %v", got, one)
	}
}

func TestOnlyLandWearsDownAndOnlyToTheSea(t *testing.T) {
	g := mustGrid(t, 3)
	// Ocean floor everywhere, with one seamount and one island.
	columns := make([]crust.Column, len(g.Pos))
	for i := range columns {
		columns[i] = crust.NewOcean()
	}
	const seamount, island = 100, 300
	columns[seamount].FelsicPlutonic = 1e7
	columns[island] = crust.ColumnAt(500)
	height := crust.Elevations(columns)
	sea := (height[seamount] + height[island]) / 2
	if !(height[seamount] < sea && sea < height[island] && height[0] < height[seamount]) {
		t.Fatalf("the test's heights are %v, %v, and %v", height[0], height[seamount], height[island])
	}

	flows := Flows(g, columns, sea, 1)
	var moved float64
	for _, f := range flows {
		if f.From != island {
			t.Fatalf("rock moves from cell %d, which is under the sea", f.From)
		}
		moved += f.Share * columns[island].Continental()
	}
	// The drop is to the sea, not to the floor under it.
	var weight float64
	for _, w := range g.EdgeWeights()[island] {
		weight += w
	}
	want := Rate * weight * (height[island] - sea) * crust.FelsicDensity /
		(g.Area[island] * sphere.RadiusKm * sphere.RadiusKm)
	if math.Abs(moved-want) > 1e-9*want {
		t.Errorf("%v kg left each square metre of the island, want %v", moved, want)
	}
}

func TestNoCellGivesMoreThanItsLimit(t *testing.T) {
	g := mustGrid(t, 3)
	columns := flat(g)
	columns[100].FelsicPlutonic *= 3
	sea := columns[0].Elevation() - 100
	var sum float64
	for _, f := range Flows(g, columns, sea, 1e6) {
		if f.From == 100 {
			sum += f.Share
		}
	}
	if math.Abs(sum-maxShare) > 1e-12 {
		t.Errorf("the peak gives %v of its rock, want %v", sum, maxShare)
	}
}

func TestRockDoesNotMoveOntoBareCells(t *testing.T) {
	g := mustGrid(t, 3)
	columns := flat(g)
	columns[100].FelsicPlutonic *= 2
	bare := g.Neighbors[100][0]
	columns[bare] = crust.Column{}
	sea := columns[0].Elevation() - 100
	for _, f := range Flows(g, columns, sea, 1) {
		if f.To == bare {
			t.Fatalf("rock moves onto cell %d, which has no crust", bare)
		}
	}
}
