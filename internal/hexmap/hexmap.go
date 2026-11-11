// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package hexmap carries a field on the sphere onto Hāmwold's hex map.
//
// The map is a rectangle of flat-top hexes in odd-q offset coordinates: odd
// columns sit half a hex lower than even ones. A hex is known by its index,
// row*columns + column, and row 0 is the north edge. The map wraps east to
// west. This must stay the same as the grid in Hāmwold's internal/worldgen.
package hexmap

import (
	"math"

	"github.com/maloquacious/tecto/internal/sphere"
)

// Latitude returns the latitude of a hex in radians, from +π/2 at the north
// edge to -π/2 at the south edge. It is linear in the row, and an odd
// column sits half a row lower. It is the same as Hāmwold's grid.latitude.
func Latitude(col, row, rows int) float64 {
	y := float64(row) + 0.5 + 0.5*float64(col&1)
	return (0.5 - y/(float64(rows)+0.5)) * math.Pi
}

// Longitude returns the longitude of a hex in radians, measured east from
// the meridian of the middle column, columns/2. It is linear in the column,
// and the columns go once around the world.
func Longitude(col, columns int) float64 {
	return 2 * math.Pi * float64(col-columns/2) / float64(columns)
}

// Frame is where the map lies on the sphere: three directions of length
// one, each at right angles to the others.
type Frame struct {
	// Center is the point on the sphere at the middle of the map: on the
	// equator, in the middle column.
	Center sphere.Vec3
	// East is the point on the map's equator a quarter of the way around
	// to the east.
	East sphere.Vec3
	// North is the map's north pole.
	North sphere.Vec3
}

// FrameAt returns the frame that puts center, a point on the unit sphere,
// at the middle of the map. Of all such frames it is the one whose north
// pole is nearest to the sphere's own, the z axis.
func FrameAt(center sphere.Vec3) Frame {
	up := sphere.Vec3{Z: 1}
	if math.Abs(center.Z) > 0.999 {
		// The center is at a pole, and no north is nearer than another.
		up = sphere.Vec3{X: 1}
	}
	north := up.Sub(center.Scale(up.Dot(center))).Normalize()
	return Frame{Center: center, East: north.Cross(center), North: north}
}

// Turned returns the frame turned about its center by an angle in radians.
// The center stays at the middle of the map, and the north pole moves toward
// what was the east.
func (f Frame) Turned(angle float64) Frame {
	north := f.North.Scale(math.Cos(angle)).Add(f.East.Scale(math.Sin(angle))).Normalize()
	return Frame{Center: f.Center, East: north.Cross(f.Center), North: north}
}

// Point returns the point on the sphere at a latitude and longitude on the
// map, in radians.
func (f Frame) Point(lat, lon float64) sphere.Vec3 {
	p := sphere.LatLon(lat, lon)
	return f.Center.Scale(p.X).Add(f.East.Scale(p.Y)).Add(f.North.Scale(p.Z))
}

// Sample returns the value of field, which has one value for each cell of
// g, at every hex of a map of the given size. A hex takes its value from
// the three cells of the face its center is in, weighted by how near it is
// to each.
func Sample(g *sphere.Grid, field []float64, f Frame, columns, rows int) []float32 {
	out := make([]float32, columns*rows)
	for row := range rows {
		for col := range columns {
			p := f.Point(Latitude(col, row, rows), Longitude(col, columns))
			face, w := g.Locate(p)
			cells := g.Faces[face]
			v := w[0]*field[cells[0]] + w[1]*field[cells[1]] + w[2]*field[cells[2]]
			out[row*columns+col] = float32(v)
		}
	}
	return out
}
