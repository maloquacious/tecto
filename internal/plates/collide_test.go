// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

func TestSolve(t *testing.T) {
	// Three equations whose rows must be swapped to be solved in order.
	x := solve([][]float64{
		{0, 2, 1, 5},
		{1, 1, 3, 12},
		{4, 0, 0, 8},
	})
	want := []float64{2, 1, 3}
	for i := range want {
		if math.Abs(x[i]-want[i]) > 1e-12 {
			t.Fatalf("got %v, want %v", x, want)
		}
	}
}

// meeting returns two plates that overlap by a narrow belt, the second
// turned on its own grid, and the middle of the belt.
func meeting(g *sphere.Grid, first, second crust.Column) (*World, sphere.Vec3) {
	turned := sphere.Rotation(sphere.Vec3{X: 0.3, Y: 0.2, Z: 0.5})
	east, west := sphere.Vec3{X: 1}, sphere.LatLon(0, 1.9)
	a := newPlate(g, capOf(g, east, 1.0), uniform(g, first))
	b := newPlate(g, capOf(g, turned.Transpose().Apply(west), 1.0), uniform(g, second))
	b.Rot = turned
	return &World{Grid: g, Plates: []*Plate{a, b}}, sphere.LatLon(0, 0.95)
}

func TestContinentsThatMeetStopConverging(t *testing.T) {
	g := mustGrid(t, 4)
	w, belt := meeting(g, continent, continent)
	m := w.Merge()
	if m.Overlap == 0 || m.Overlap > 0.05 {
		t.Fatalf("the test's plates overlap over %v of the sphere", m.Overlap)
	}
	// The first plate is driven east into the second, which is not driven.
	drive := sphere.Vec3{Z: 0.01}
	rates := []sphere.Vec3{drive, {}}
	w.brake(m, rates)

	world := []sphere.Vec3{w.Plates[0].Rot.Apply(rates[0]), w.Plates[1].Rot.Apply(rates[1])}
	closing := world[0].Sub(world[1]).Cross(belt).Len()
	if was := drive.Cross(belt).Len(); !(closing < 0.05*was) {
		t.Errorf("the plates close at %v, and did at %v before they met", closing, was)
	}
	// The second is carried along by the first, and the two go on at less
	// than the first went alone.
	speed := world[1].Cross(belt).Len()
	if was := drive.Cross(belt).Len(); !(speed > 0.2*was && speed < 0.8*was) {
		t.Errorf("the plate that was met moves at %v, where the other came on at %v", speed, was)
	}
	if world[1].Cross(belt).Dot(drive.Cross(belt)) <= 0 {
		t.Errorf("the plate that was met does not move the way it was pushed")
	}
}

func TestOceanFloorUnderAContinentDoesNotHoldIt(t *testing.T) {
	g := mustGrid(t, 4)
	for _, floor := range []crust.Column{oldOcean, crust.NewOcean()} {
		w, _ := meeting(g, continent, floor)
		m := w.Merge()
		if m.Overlap == 0 {
			t.Fatal("the test has no overlap")
		}
		rates := []sphere.Vec3{{Z: 0.01}, {X: 0.002}}
		want := slices.Clone(rates)
		w.brake(m, rates)
		if !slices.Equal(rates, want) {
			t.Errorf("floor of age %v: the rates went from %v to %v", floor.Age, want, rates)
		}
	}
}

func TestPlatesApartAreNotHeld(t *testing.T) {
	g := mustGrid(t, 4)
	a := newPlate(g, capOf(g, sphere.Vec3{X: 1}, 0.5), uniform(g, continent))
	b := newPlate(g, capOf(g, sphere.Vec3{X: -1}, 0.5), uniform(g, continent))
	w := &World{Grid: g, Plates: []*Plate{a, b}}
	rates := []sphere.Vec3{{Z: 0.01}, {X: 0.002}}
	want := slices.Clone(rates)
	w.brake(w.Merge(), rates)
	if !slices.Equal(rates, want) {
		t.Errorf("the rates went from %v to %v", want, rates)
	}
}

func TestErosionOnPlatesKeepsTheRock(t *testing.T) {
	g := mustGrid(t, 4)
	// Two continents that have met: the belt where they overlap stands
	// high, and wears down onto both.
	w, _ := meeting(g, continent, continent)
	w.budget.Start = w.continental()
	m := w.Merge()
	before := crust.Elevations(m.Columns)
	if err := w.erode(m, 5); err != nil {
		t.Fatal(err)
	}
	closes(t, w)
	if err := w.check(); err != nil {
		t.Fatal(err)
	}
	after := crust.Elevations(w.Merge().Columns)
	lower, higher := 0, 0
	for cell, count := range m.Count {
		switch {
		case count == 2 && after[cell] < before[cell]:
			lower++
		case count == 1 && after[cell] > before[cell]:
			higher++
		}
	}
	if lower == 0 || higher == 0 {
		t.Errorf("%d cells of the belt are lower and %d cells beside it higher, want some of each", lower, higher)
	}
}
