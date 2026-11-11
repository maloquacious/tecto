// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"math"
	"math/rand/v2"
	"slices"
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

// startingCrust is the crust a run with this seed begins from.
func startingCrust(g *sphere.Grid, seed uint64) []crust.Column {
	return crust.Initial(g.Pos, rand.New(rand.NewPCG(seed, 1)), rand.New(rand.NewPCG(seed, 2)))
}

// Rock that floats, and rock that sinks.
var (
	continent = crust.ColumnAt(500)
	oldOcean  = func() crust.Column {
		c := crust.NewOcean()
		c.Age = crust.MaficLifetime
		return c
	}()
)

// cap returns the cells within an angle of a point.
func capOf(g *sphere.Grid, center sphere.Vec3, angle float64) []bool {
	member := make([]bool, len(g.Pos))
	for i, p := range g.Pos {
		member[i] = p.Dot(center) > math.Cos(angle)
	}
	return member
}

func uniform(g *sphere.Grid, c crust.Column) []crust.Column {
	columns := make([]crust.Column, len(g.Pos))
	for i := range columns {
		columns[i] = c
	}
	return columns
}

func orthonormalError(m sphere.Mat3) float64 {
	p := m.Mul(m.Transpose())
	id := sphere.Identity()
	var worst float64
	for i := range 3 {
		worst = max(worst, p[i].Sub(id[i]).Len())
	}
	return worst
}

// checkSegments fails unless every cell is in one plate and each plate is
// connected and large enough.
func checkSegments(t *testing.T, g *sphere.Grid, plate []int32, count int) {
	t.Helper()
	if count < 1 || count > MaxPlates {
		t.Fatalf("%d plates, want 1 to %d", count, MaxPlates)
	}
	area := make([]float64, count)
	for cell, id := range plate {
		if id < 0 || int(id) >= count {
			t.Fatalf("cell %d is in plate %d of %d", cell, id, count)
		}
		area[id] += g.Area[cell]
	}
	minArea := MinPlateAreaKm2 / (sphere.RadiusKm * sphere.RadiusKm)
	for id := range int32(count) {
		if area[id] < minArea {
			t.Errorf("plate %d has area %v, less than the least, %v", id, area[id], minArea)
		}
		if _, pieces := g.Components(func(cell int32) bool { return plate[cell] == id }); pieces != 1 {
			t.Errorf("plate %d is in %d pieces", id, pieces)
		}
	}
}

func TestSegment(t *testing.T) {
	for _, detail := range []int{4, 5} {
		g := mustGrid(t, detail)
		for seed := range uint64(8) {
			plate, count := Segment(g, startingCrust(g, seed))
			checkSegments(t, g, plate, count)
			if count < 2 {
				t.Errorf("detail %d, seed %d: the crust stayed in one piece", detail, seed)
			}
			again, _ := Segment(g, startingCrust(g, seed))
			if !slices.Equal(plate, again) {
				t.Errorf("detail %d, seed %d: the same crust gave different plates", detail, seed)
			}
		}
	}
}

// With nothing dense enough to sink, nothing drives the mantle, and the
// crust is one plate.
func TestSegmentOfCrustThatFloats(t *testing.T) {
	g := mustGrid(t, 3)
	plate, count := Segment(g, uniform(g, continent))
	checkSegments(t, g, plate, count)
	if count != 1 {
		t.Errorf("%d plates, want 1", count)
	}
}

// The constants of segmentation are lengths and areas, so the plates do not
// depend much on the size of the cells. The cells of one grid are the first
// cells of the next, so the two sets of plates can be compared cell by cell.
func TestSegmentIsAlikeAtTwoDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("segments worlds at detail 6")
	}
	coarse, fine := mustGrid(t, 5), mustGrid(t, 6)
	for seed := range uint64(4) {
		a, na := Segment(coarse, startingCrust(coarse, seed))
		b, nb := Segment(fine, startingCrust(fine, seed))
		if d := na - nb; d < -1 || d > 1 {
			t.Errorf("seed %d: %d plates at detail 5 and %d at detail 6", seed, na, nb)
		}
		// Pair each fine plate with the coarse plate it shares most cells
		// with, and count the cells that the pairing accounts for.
		var shared [MaxPlates][MaxPlates]int
		for cell, id := range a {
			shared[b[cell]][id]++
		}
		agree := 0
		for _, row := range shared {
			agree += slices.Max(row[:])
		}
		if share := float64(agree) / float64(len(a)); share < 0.8 {
			t.Errorf("seed %d: the plates at the two details agree on %.2f of the cells, want 0.8", seed, share)
		}
	}
}

func TestGrowSettlesTiesByPlate(t *testing.T) {
	g := mustGrid(t, 2)
	plate := make([]int32, len(g.Pos))
	for i := range plate {
		plate[i] = -1
	}
	plate[0], plate[11] = 1, 0
	grow(g, plate, 1)
	for cell, id := range plate {
		want := int32(-1)
		switch {
		case cell == 0 || slices.Contains(g.Neighbors[0], int32(cell)):
			want = 1
		case cell == 11 || slices.Contains(g.Neighbors[11], int32(cell)):
			want = 0
		}
		if id != want {
			t.Fatalf("after one step cell %d is in plate %d, want %d", cell, id, want)
		}
	}
	grow(g, plate, math.MaxInt)
	if slices.Contains(plate, -1) {
		t.Error("cells were left without a plate")
	}
}

// A plate with sinking crust along one side moves toward that side.
func TestPlateMovesTowardItsDenseEdge(t *testing.T) {
	g := mustGrid(t, 4)
	center := sphere.Vec3{X: 1}
	for _, toward := range []sphere.Vec3{{Y: 1}, {Y: -1}, {Z: 1}, sphere.Vec3{Y: 1, Z: -1}.Normalize()} {
		columns := uniform(g, continent)
		for i, p := range g.Pos {
			if p.Dot(toward) > 0.2 {
				columns[i] = oldOcean
			}
		}
		p := newPlate(g, capOf(g, center, 0.5), columns)
		rate := p.AngularVelocity(g)
		v := rate.Cross(center)
		if along := v.Dot(toward); !(along > 0.9*v.Len()) || v.Len() == 0 {
			t.Errorf("dense edge toward %v: the plate's middle moves at %v", toward, v)
		}
		// The plate goes no faster than a slab sinks.
		limit := -oldOcean.Buoyancy() * pullPerBuoyancy
		if speed := v.Len(); speed > limit || speed < 0.2*limit {
			t.Errorf("dense edge toward %v: speed %v, want between a fifth of %v and all of it", toward, speed, limit)
		}
	}
}

func TestPullsAllAroundAPlateCancel(t *testing.T) {
	g := mustGrid(t, 4)
	p := newPlate(g, capOf(g, sphere.Vec3{Z: 1}, 0.5), uniform(g, oldOcean))
	limit := -oldOcean.Buoyancy() * pullPerBuoyancy
	if rate := p.AngularVelocity(g).Len(); rate > 0.02*limit {
		t.Errorf("a plate pulled evenly all around turns at %v; a slab sinks at %v", rate, limit)
	}
}

func TestPlateThatFloatsDoesNotMove(t *testing.T) {
	g := mustGrid(t, 3)
	p := newPlate(g, capOf(g, sphere.Vec3{Z: 1}, 0.5), uniform(g, continent))
	if rate := p.AngularVelocity(g); rate != (sphere.Vec3{}) {
		t.Errorf("rate %v, want none", rate)
	}
	// A plate that is the whole world has no edge to be pulled by.
	everywhere := make([]bool, len(g.Pos))
	for i := range everywhere {
		everywhere[i] = true
	}
	all := newPlate(g, everywhere, uniform(g, oldOcean))
	if rate := all.AngularVelocity(g); rate != (sphere.Vec3{}) {
		t.Errorf("the whole world turns at %v", rate)
	}
}

// The slab speed the model works from is a few centimetres a year.
func TestPullIsAtPlateSpeeds(t *testing.T) {
	kmPerMyr := -oldOcean.Buoyancy() * pullPerBuoyancy * sphere.RadiusKm
	if kmPerMyr < 100 || kmPerMyr > 500 {
		t.Errorf("the oldest ocean floor sinks at %.0f km per million years, want 100 to 500", kmPerMyr)
	}
}

// A plate turned by a known angle keeps its area and moves its middle by
// that angle.
func TestTurnedPlateKeepsItsAreaAndMovesItsMiddle(t *testing.T) {
	g := mustGrid(t, 5)
	start := sphere.Vec3{X: 1}
	for _, angle := range []float64{0.3, 1.1, 2.5} {
		p := newPlate(g, capOf(g, start, 0.4), uniform(g, continent))
		// Turn about z in many small steps, as a run does.
		for range 100 {
			p.turn(sphere.Vec3{Z: angle / 100}, 1)
		}
		if e := orthonormalError(p.Rot); e > 1e-14 {
			t.Errorf("angle %v: the rotation is off by %v", angle, e)
		}
		w := &World{Grid: g, Plates: []*Plate{p}}
		m := w.Merge()
		var area float64
		var sum sphere.Vec3
		for cell, top := range m.Top {
			if top == 0 {
				area += g.Area[cell]
				sum = sum.Add(g.Pos[cell].Scale(g.Area[cell]))
			}
		}
		if math.Abs(area-p.Area) > 0.02*p.Area {
			t.Errorf("angle %v: area %v after turning, %v before", angle, area, p.Area)
		}
		// A cell is given to the plate whole or not at all, so the edge
		// lands a little differently wherever the plate comes to rest.
		want := sphere.Rotation(sphere.Vec3{Z: angle}).Apply(p.center)
		if off := math.Acos(min(1, p.center.Dot(start))); off > 0.01 {
			t.Fatalf("the plate's middle began %v radians from where it was put", off)
		}
		if off := math.Acos(min(1, sum.Normalize().Dot(want))); off > 0.5*g.Spacing() {
			t.Errorf("angle %v: the middle is %v radians from where it should be", angle, off)
		}
		if got, want := m.Gap, 1-area/(4*math.Pi); math.Abs(got-want) > 1e-12 {
			t.Errorf("angle %v: gap %v, want %v", angle, got, want)
		}
		inside := g.Nearest(start)
		if got := p.Crust[inside].Age; math.Abs(got-continent.Age-100) > 1e-9 {
			t.Errorf("angle %v: the crust's age went from %v to %v in 100 Myr", angle, continent.Age, got)
		}
	}
}

func TestMergeOfUnmovedPlatesIsTheCrust(t *testing.T) {
	g := mustGrid(t, 4)
	columns := startingCrust(g, 7)
	w := New(g, columns)
	m := w.Merge()
	if !slices.Equal(m.Columns, columns) {
		t.Error("merging plates that have not moved changed the crust")
	}
	if m.Gap != 0 || m.Overlap != 0 {
		t.Errorf("gap %v and overlap %v, want none", m.Gap, m.Overlap)
	}
	owner, _ := Segment(g, columns)
	if !slices.Equal(m.Top, owner) {
		t.Error("the plate on top is not the plate the cell was given to")
	}
}

func TestMergeWherePlatesOverlapAndPart(t *testing.T) {
	g := mustGrid(t, 4)
	// A continent and a piece of old ocean floor with sediment on it,
	// lying across each other.
	floor := oldOcean
	floor.Sediment = 1000
	land := newPlate(g, capOf(g, sphere.Vec3{X: 1}, 0.5), uniform(g, continent))
	sea := newPlate(g, capOf(g, sphere.LatLon(0, 0.6), 0.5), uniform(g, floor))
	// The order of the plates must not matter.
	for _, plates := range [][]*Plate{{land, sea}, {sea, land}} {
		w := &World{Grid: g, Plates: plates}
		m := w.Merge()
		var gap, overlap float64
		for cell, c := range m.Columns {
			onLand, onSea := land.Member[cell], sea.Member[cell]
			want := c
			switch {
			case onLand && onSea:
				overlap += g.Area[cell]
				want = continent
				want.Sediment += floor.Sediment
				if plates[m.Top[cell]] != land {
					t.Fatalf("cell %d: the ocean floor is on top of the continent", cell)
				}
			case onLand:
				want = continent
			case onSea:
				want = floor
			default:
				gap += g.Area[cell]
				want = crust.NewOcean()
				if m.Top[cell] != -1 {
					t.Fatalf("cell %d: plate %d is on top of a gap", cell, m.Top[cell])
				}
			}
			if c != want {
				t.Fatalf("cell %d: crust %+v, want %+v", cell, c, want)
			}
		}
		if overlap == 0 || gap == 0 {
			t.Fatal("the test has no overlap or no gap")
		}
		if math.Abs(m.Gap-gap/(4*math.Pi)) > 1e-12 || math.Abs(m.Overlap-overlap/(4*math.Pi)) > 1e-12 {
			t.Errorf("gap %v and overlap %v, want %v and %v", m.Gap, m.Overlap, gap/(4*math.Pi), overlap/(4*math.Pi))
		}
		if err := crust.Check(m.Columns); err != nil {
			t.Error(err)
		}
	}
}

// The plates drift for 150 million years.
func TestPlatesDrift(t *testing.T) {
	g := mustGrid(t, 4)
	for seed := range uint64(5) {
		columns := startingCrust(g, seed)
		w := New(g, columns)
		areas := make([]float64, len(w.Plates))
		for i, p := range w.Plates {
			areas[i] = p.Area
		}
		const myr = 150
		if err := w.Advance(myr); err != nil {
			t.Fatal(err)
		}
		if w.Steps < myr/MaxStepMyr {
			t.Errorf("seed %d: %d steps, so some were longer than %v Myr", seed, w.Steps, MaxStepMyr)
		}
		// No plate can have moved a cell in a step, even had every step
		// been at the highest speed of the run.
		if w.Fastest <= 0 {
			t.Errorf("seed %d: nothing moved", seed)
		}
		if most := myr * w.Fastest / g.Spacing() / stepCells; float64(w.Steps) > most+myr/MaxStepMyr+1 {
			t.Errorf("seed %d: %d steps, more than the %0.f the fastest plate calls for", seed, w.Steps, most)
		}
		moved := 0
		for i, p := range w.Plates {
			if e := orthonormalError(p.Rot); e > 1e-14 {
				t.Errorf("seed %d, plate %d: the rotation is off by %v", seed, i, e)
			}
			if p.Rot != sphere.Identity() {
				moved++
			}
			var area float64
			for cell, in := range p.Member {
				if !in {
					continue
				}
				area += g.Area[cell]
				if got, want := p.Crust[cell].Age, columns[cell].Age+myr; math.Abs(got-want) > 1e-9 {
					t.Fatalf("seed %d, plate %d, cell %d: age %v, want %v", seed, i, cell, got, want)
				}
			}
			if area != areas[i] {
				t.Errorf("seed %d, plate %d: area went from %v to %v", seed, i, areas[i], area)
			}
		}
		if moved < 2 {
			t.Errorf("seed %d: %d of %d plates moved", seed, moved, len(w.Plates))
		}
		m := w.Merge()
		if err := crust.Check(m.Columns); err != nil {
			t.Errorf("seed %d: %v", seed, err)
		}
		if m.Gap <= 0 || m.Overlap <= 0 || m.Gap+m.Overlap >= 1 {
			t.Errorf("seed %d: gap %v and overlap %v", seed, m.Gap, m.Overlap)
		}

		again := New(g, columns)
		if err := again.Advance(myr); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(again.Merge().Columns, m.Columns) {
			t.Errorf("seed %d: the same crust drifted to two different worlds", seed)
		}
	}
}

func TestAdvanceTakesNoStepsForNoTime(t *testing.T) {
	g := mustGrid(t, 3)
	w := New(g, startingCrust(g, 1))
	if err := w.Advance(0); err != nil {
		t.Fatal(err)
	}
	if w.Steps != 0 {
		t.Errorf("%d steps", w.Steps)
	}
}
