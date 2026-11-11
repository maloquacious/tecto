// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package hexmap

import (
	"math"
	"testing"

	"github.com/maloquacious/tecto/internal/sphere"
)

// hamwoldLatitude is grid.latitude from Hāmwold's internal/worldgen/grid.go,
// copied as it is written there, so the test fails if the two drift apart.
func hamwoldLatitude(rows, col, row int) float64 {
	y := float64(row) + 0.5 + 0.5*float64(col&1)
	return (0.5 - y/(float64(rows)+0.5)) * math.Pi
}

var testFrames = map[string]Frame{
	"the sphere's own": FrameAt(sphere.Vec3{X: 1}),
	"tilted":           FrameAt(sphere.LatLon(0.7, 2.1)),
	"center at a pole": FrameAt(sphere.Vec3{Z: -1}),
}

func TestFrames(t *testing.T) {
	for name, f := range testFrames {
		for _, v := range []float64{
			f.Center.Len() - 1, f.East.Len() - 1, f.North.Len() - 1,
			f.Center.Dot(f.East), f.East.Dot(f.North), f.North.Dot(f.Center),
			// Right-handed: east is to the right with north up, seen
			// from outside.
			sphere.Det(f.Center, f.East, f.North) - 1,
		} {
			if math.Abs(v) > 1e-12 {
				t.Errorf("%s: frame is not orthonormal and right-handed: %+v", name, f)
				break
			}
		}
	}
	// The middle column's equator is the center.
	c := sphere.LatLon(0.7, 2.1)
	f := FrameAt(c)
	if d := f.Point(0, Longitude(960, 1920)).Sub(c).Len(); d > 1e-12 {
		t.Errorf("the middle of the map is %g from the frame's center", d)
	}
	// North stays as near the sphere's north as it can: in the plane of
	// the center and the z axis, on the z side.
	if f.North.Z <= 0 || math.Abs(sphere.Det(c, sphere.Vec3{Z: 1}, f.North)) > 1e-12 {
		t.Errorf("north %v is not toward the z axis from %v", f.North, c)
	}
}

func TestLongitude(t *testing.T) {
	const columns = 64
	if got := Longitude(columns/2, columns); got != 0 {
		t.Errorf("middle column is at longitude %v", got)
	}
	step := 2 * math.Pi / columns
	for col := 1; col < columns; col++ {
		if d := Longitude(col, columns) - Longitude(col-1, columns); math.Abs(d-step) > 1e-12 {
			t.Fatalf("column %d is %v east of the one before, want %v", col, d, step)
		}
	}
	// One more column would be the first again.
	around := Longitude(columns-1, columns) + step - Longitude(0, columns)
	if math.Abs(around-2*math.Pi) > 1e-12 {
		t.Errorf("the columns go %v around, want 2π", around)
	}
}

func TestSampleLatitude(t *testing.T) {
	const columns, rows = 96, 40
	g, err := sphere.NewGrid(5)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range testFrames {
		// A field equal to latitude on the map.
		field := make([]float64, len(g.Pos))
		for cell, p := range g.Pos {
			field[cell] = math.Asin(min(max(p.Dot(f.North), -1), 1))
		}
		got := Sample(g, field, f, columns, rows)
		if len(got) != columns*rows {
			t.Fatalf("%s: %d hexes, want %d", name, len(got), columns*rows)
		}
		for row := range rows {
			for col := range columns {
				want := hamwoldLatitude(rows, col, row)
				// Latitude is close to linear across a face, except in
				// the faces around a pole, where it comes to a point.
				// Half a row is 0.039 radians here, so either tolerance
				// would catch a hex in the wrong row or with the wrong
				// stagger.
				tolerance := 0.002
				if math.Abs(want) > 1.45 {
					tolerance = 0.02
				}
				if d := float64(got[row*columns+col]) - want; math.Abs(d) > tolerance {
					t.Fatalf("%s: hex (%d, %d) sampled latitude %v, want %v", name, col, row, got[row*columns+col], want)
				}
			}
		}
	}
}

func TestSampleIsContinuousAcrossTheWrap(t *testing.T) {
	const columns, rows = 96, 40
	g, err := sphere.NewGrid(4)
	if err != nil {
		t.Fatal(err)
	}
	f := testFrames["tilted"]
	// A smooth field that changes with longitude: it changes by no more
	// than the distance between two points, in radians.
	toward := sphere.LatLon(-0.3, 0.4)
	field := make([]float64, len(g.Pos))
	for cell, p := range g.Pos {
		field[cell] = p.Dot(toward)
	}
	got := Sample(g, field, f, columns, rows)
	// The slack is for the straight-line interpolation across a face.
	const slack = 0.005
	for row := range rows {
		for col := range columns {
			east := (col + 1) % columns
			a := f.Point(Latitude(col, row, rows), Longitude(col, columns))
			b := f.Point(Latitude(east, row, rows), Longitude(east, columns))
			apart := a.Sub(b).Len()
			d := math.Abs(float64(got[row*columns+col] - got[row*columns+east]))
			if d > apart+slack {
				t.Fatalf("hexes (%d, %d) and (%d, %d) are %v apart and differ by %v", col, row, east, row, apart, d)
			}
		}
		// The last column's eastern neighbor is the first, and they are
		// as near to each other as any other pair in the row.
		a := f.Point(Latitude(columns-1, row, rows), Longitude(columns-1, columns))
		b := f.Point(Latitude(0, row, rows), Longitude(0, columns))
		c := f.Point(Latitude(1, row, rows), Longitude(1, columns))
		if wrap, inside := a.Sub(b).Len(), b.Sub(c).Len(); math.Abs(wrap-inside) > 1e-9 {
			t.Fatalf("row %d: the wrap is %v wide, the other gaps %v", row, wrap, inside)
		}
	}
}
