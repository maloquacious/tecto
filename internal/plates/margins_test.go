// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

// closes reports whether the plates hold the continental rock the account
// says they do.
func closes(t *testing.T, w *World) {
	t.Helper()
	b := w.budget
	want := b.Start - b.Subducted + b.Accreted + b.Resampled
	if got := w.continental(); math.Abs(got-want) > 1e-9*b.Start {
		t.Errorf("at %v Myr the plates hold %v kg of continental rock, but the account comes to %v (%+v)", w.Time, got, want, b)
	}
	if b.Subducted < 0 || b.Accreted < 0 || b.Accreted > b.Subducted*(1+1e-12) {
		t.Errorf("at %v Myr: %v kg subducted and %v kg accreted", w.Time, b.Subducted, b.Accreted)
	}
}

func TestRiftFillsTheGapFromBothSides(t *testing.T) {
	g := mustGrid(t, 4)
	east, west := sphere.Vec3{X: 1}, sphere.Vec3{X: -1}
	// Two plates that between them leave a wide band of the world bare,
	// one of them turned on its own grid so that its cells do not line up
	// with the world's.
	turned := sphere.Rotation(sphere.Vec3{X: 0.3, Y: 0.2, Z: 0.5})
	a := newPlate(g, capOf(g, east, 1.0), uniform(g, continent))
	b := newPlate(g, capOf(g, turned.Transpose().Apply(west), 0.8), uniform(g, oldOcean))
	b.Rot = turned
	w := &World{Grid: g, Plates: []*Plate{a, b}}
	before := w.Merge()
	if before.Gap < 0.2 {
		t.Fatalf("the test has gaps over only %v of the sphere", before.Gap)
	}
	areas := []float64{a.Area, b.Area}
	w.margins(before)

	after := w.Merge()
	// The plates may overlap by a cell where their shares of the gap meet.
	if after.Gap != 0 || after.Overlap > 0.005 {
		t.Fatalf("gaps over %v of the sphere and overlaps over %v, want none", after.Gap, after.Overlap)
	}
	if err := crust.Check(after.Columns); err != nil {
		t.Fatal(err)
	}
	for cell, c := range after.Columns {
		if before.Count[cell] != 0 {
			if c != before.Columns[cell] || after.Top[cell] != before.Top[cell] || after.Count[cell] != 1 {
				t.Fatalf("cell %d was covered, and has changed", cell)
			}
			continue
		}
		if c != crust.NewOcean() {
			t.Fatalf("cell %d was bare, and now holds %+v", cell, c)
		}
		// The new floor belongs to the nearer plate. Allow a cell or so
		// either way, since the gap is shared out cell by cell.
		toA := math.Acos(g.Pos[cell].Dot(east)) - 1.0
		toB := math.Acos(g.Pos[cell].Dot(west)) - 0.8
		nearer := int32(0)
		if toB < toA {
			nearer = 1
		}
		if after.Top[cell] != nearer && math.Abs(toA-toB) > 3*g.Spacing() {
			t.Fatalf("cell %d is %v from one plate and %v from the other, and went to plate %d", cell, toA, toB, after.Top[cell])
		}
	}
	// Each plate has grown, and between them they cover the sphere about
	// once: no holes are left in them to open later.
	if a.Area <= areas[0] || b.Area <= areas[1] {
		t.Errorf("areas went from %v to %v and %v", areas, a.Area, b.Area)
	}
	if total := a.Area + b.Area; math.Abs(total-4*math.Pi) > 0.02*4*math.Pi {
		t.Errorf("the plates cover %v of the sphere", total/(4*math.Pi))
	}
	if w.budget != (Budget{}) {
		t.Errorf("rifting changed the account of continental rock: %+v", w.budget)
	}
}

func TestSunkenEdgeIsTakenAndItsRockAccreted(t *testing.T) {
	g := mustGrid(t, 4)
	floor := oldOcean
	floor.Sediment = 1000
	landCap := capOf(g, sphere.Vec3{X: 1}, 0.5)
	seaCap := capOf(g, sphere.LatLon(0, 0.6), 0.5)
	for _, order := range []int{0, 1} {
		land := newPlate(g, slices.Clone(landCap), uniform(g, continent))
		sea := newPlate(g, slices.Clone(seaCap), uniform(g, floor))
		w := &World{Grid: g, Plates: []*Plate{land, sea}}
		if order == 1 {
			w.Plates = []*Plate{sea, land}
		}
		w.budget.Start = w.continental()
		m := w.Merge()
		if m.Overlap == 0 {
			t.Fatal("the test has no overlap")
		}
		w.subduct(m)
		closes(t, w)

		var taken float64
		for cell := range g.Pos {
			both := landCap[cell] && seaCap[cell]
			if sea.Member[cell] != (seaCap[cell] && !both) {
				t.Fatalf("cell %d: the ocean floor is there: %v", cell, sea.Member[cell])
			}
			if land.Member[cell] != landCap[cell] {
				t.Fatalf("cell %d: the continent has changed shape", cell)
			}
			want := continent
			if both {
				taken += floor.Sediment * g.Area[cell] * metres2
				want.FelsicPlutonic += 0.85 * floor.Sediment
				want.FelsicVolcanic += 0.15 * floor.Sediment
			}
			got := land.Crust[cell]
			if landCap[cell] && (math.Abs(got.FelsicPlutonic-want.FelsicPlutonic) > 1e-6 ||
				math.Abs(got.FelsicVolcanic-want.FelsicVolcanic) > 1e-6 ||
				got.Sediment != want.Sediment || got.MaficPlutonic != want.MaficPlutonic) {
				t.Fatalf("cell %d: the continent holds %+v, want %+v", cell, got, want)
			}
		}
		b := w.budget
		if math.Abs(b.Subducted-taken) > 1e-9*taken || math.Abs(b.Accreted-taken) > 1e-9*taken {
			t.Errorf("%v kg subducted and %v kg accreted, want %v of each", b.Subducted, b.Accreted, taken)
		}
		after := w.Merge()
		if after.Overlap != 0 || after.Gap != m.Gap {
			t.Errorf("overlap %v and gap %v after, want none and %v", after.Overlap, after.Gap, m.Gap)
		}
	}
}

func TestCrustThatFloatsIsNotSubducted(t *testing.T) {
	g := mustGrid(t, 4)
	for name, under := range map[string]crust.Column{
		"new ocean floor": crust.NewOcean(),
		"a continent":     crust.ColumnAt(200),
	} {
		top := newPlate(g, capOf(g, sphere.Vec3{X: 1}, 0.5), uniform(g, continent))
		low := newPlate(g, capOf(g, sphere.LatLon(0, 0.6), 0.5), uniform(g, under))
		w := &World{Grid: g, Plates: []*Plate{top, low}}
		areas := []float64{top.Area, low.Area}
		w.subduct(w.Merge())
		if top.Area != areas[0] || low.Area != areas[1] {
			t.Errorf("%s under a continent: areas went from %v to %v and %v", name, areas, top.Area, low.Area)
		}
		if w.budget != (Budget{}) {
			t.Errorf("%s under a continent: %+v", name, w.budget)
		}
	}
}

// Crust that sinks goes down only where it is joined to the plate's edge
// by crust that sinks.
func TestSunkenCrustBehindCrustThatFloatsStays(t *testing.T) {
	g := mustGrid(t, 4)
	center := sphere.LatLon(0, 0.6)
	columns := uniform(g, oldOcean)
	seaCap := capOf(g, center, 0.5)
	inner := capOf(g, center, 0.3)
	for cell := range columns {
		if seaCap[cell] && !inner[cell] {
			columns[cell] = crust.NewOcean()
		}
	}
	sea := newPlate(g, seaCap, columns)
	lid := newPlate(g, capOf(g, center, 0.8), uniform(g, continent))
	w := &World{Grid: g, Plates: []*Plate{lid, sea}}
	area := sea.Area
	w.subduct(w.Merge())
	if sea.Area != area {
		t.Errorf("the plate's area went from %v to %v", area, sea.Area)
	}
}

func TestPlateWithNoCrustIsLeftOut(t *testing.T) {
	g := mustGrid(t, 3)
	columns := uniform(g, continent)
	whole := newPlate(g, capOf(g, sphere.Vec3{X: 1}, 4), columns)
	empty := newPlate(g, make([]bool, len(g.Pos)), columns)
	w := &World{Grid: g, Plates: []*Plate{empty, whole}}
	if rate := empty.AngularVelocity(g); rate != (sphere.Vec3{}) {
		t.Errorf("a plate with no crust turns at %v", rate)
	}
	if err := w.Advance(10); err != nil {
		t.Fatal(err)
	}
	m := w.Merge()
	if m.Gap != 0 || m.Overlap != 0 || w.Count() != 1 {
		t.Errorf("gap %v, overlap %v, %d plates", m.Gap, m.Overlap, w.Count())
	}
	if slices.Contains(m.Top, 0) {
		t.Error("the plate with no crust is on top somewhere")
	}
}

func TestAdvanceFailsOnCrustThatCannotBe(t *testing.T) {
	g := mustGrid(t, 3)
	for name, spoil := range map[string]func(*crust.Column){
		"a mass that is not finite":   func(c *crust.Column) { c.FelsicPlutonic = math.Inf(1) },
		"a mass that is not a number": func(c *crust.Column) { c.FelsicPlutonic = math.NaN() },
		"a negative mass":             func(c *crust.Column) { c.Sediment = -1 },
		"no crust":                    func(c *crust.Column) { *c = crust.Column{} },
	} {
		w := New(g, startingCrust(g, 1))
		p := w.Plates[0]
		spoil(&p.Crust[slices.Index(p.Member, true)])
		err := w.Advance(1)
		if err == nil || !strings.Contains(err.Error(), "plates: step 0") {
			t.Errorf("%s: error %v", name, err)
		}
	}
}

func TestPlatesAreDrawnAfreshEachCycle(t *testing.T) {
	g := mustGrid(t, 3)
	columns := startingCrust(g, 2)
	w := New(g, columns)
	// In pieces, to show that the cycle's end does not depend on how the
	// time is given.
	for range 3 {
		if err := w.Advance(CycleMyr / 3); err != nil {
			t.Fatal(err)
		}
	}
	if w.Cycles != 0 || w.Time != CycleMyr {
		t.Fatalf("%d cycles ended by %v Myr", w.Cycles, w.Time)
	}
	steps := w.Steps
	if err := w.Advance(1); err != nil {
		t.Fatal(err)
	}
	if w.Cycles != 1 || w.Steps == steps {
		t.Fatalf("%d cycles ended by %v Myr, in %d more steps", w.Cycles, w.Time, w.Steps-steps)
	}
	closes(t, w)
	// The new plates were cut from one crust, so a million years on they
	// have hardly begun to overlap.
	if m := w.Merge(); m.Gap != 0 || m.Overlap > 0.05 {
		t.Errorf("gap %v and overlap %v a million years into a cycle", m.Gap, m.Overlap)
	}
}

// The check that milestone 3 is done: over 500 million years no cell is
// left without crust, and the account of continental rock closes.
func TestMarginsOver500Myr(t *testing.T) {
	detail, seeds := 4, uint64(4)
	if testing.Short() {
		detail, seeds = 3, 2
	}
	g := mustGrid(t, detail)
	for seed := range seeds {
		columns := startingCrust(g, seed)
		w := New(g, columns)
		var m *Merged
		for w.Time < 500 {
			if err := w.Advance(25); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			closes(t, w)
			m = w.Merge()
			if m.Gap != 0 {
				t.Fatalf("seed %d: gaps over %v of the sphere at %v Myr", seed, m.Gap, w.Time)
			}
			if err := crust.Check(m.Columns); err != nil {
				t.Fatalf("seed %d, at %v Myr: %v", seed, w.Time, err)
			}
			for i, p := range w.Plates {
				if e := orthonormalError(p.Rot); e > 1e-13 {
					t.Fatalf("seed %d, plate %d: the rotation is off by %v", seed, i, e)
				}
			}
		}
		if w.Time != 500 || w.Cycles != 3 {
			t.Errorf("seed %d: %v Myr and %d cycles", seed, w.Time, w.Cycles)
		}
		b := w.Budget(m)
		if sum := b.Start - b.Subducted + b.Accreted + b.Resampled; math.Abs(sum-b.End) > 1e-9*b.Start {
			t.Errorf("seed %d: the world ends with %v kg of continental rock, but the account comes to %v", seed, b.End, sum)
		}
		if b.Start != continental(g, columns, nil) || b.End != continental(g, m.Columns, nil) {
			t.Errorf("seed %d: the account's ends are not the crust's: %+v", seed, b)
		}
		// Reading the plates onto the world does not keep the total, but
		// it does not lose or make much.
		if math.Abs(b.Resampled) > 0.05*b.Start {
			t.Errorf("seed %d: %v of the continental rock came from resampling", seed, b.Resampled/b.Start)
		}
		if b.Subducted == 0 {
			t.Errorf("seed %d: nothing was subducted in 500 Myr", seed)
		}
		// New floor has been made: some of the crust is younger than the
		// run, and none of the starting crust has been left to age alone.
		young := 0
		for _, c := range m.Columns {
			if c.Age < 500 {
				young++
			}
		}
		if young == 0 {
			t.Errorf("seed %d: no crust is younger than the run", seed)
		}

		again := New(g, columns)
		for again.Time < 500 {
			if err := again.Advance(25); err != nil {
				t.Fatal(err)
			}
		}
		if !slices.Equal(again.Merge().Columns, m.Columns) {
			t.Errorf("seed %d: the same crust ran to two different worlds", seed)
		}
	}
}
