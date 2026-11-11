// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Vec3 is a vector in three dimensions. A point on the sphere is a Vec3 of
// length one.
type Vec3 struct {
	X, Y, Z float64
}

// Add returns a + b.
func (a Vec3) Add(b Vec3) Vec3 { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

// Sub returns a - b.
func (a Vec3) Sub(b Vec3) Vec3 { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }

// Scale returns a with every component multiplied by s.
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }

// Dot returns the dot product of a and b.
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// Cross returns the cross product a × b.
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

// Len returns the length of a.
func (a Vec3) Len() float64 { return math.Sqrt(a.Dot(a)) }

// Normalize returns a scaled to length one. The zero vector has no
// direction, and the result for it is not finite.
func (a Vec3) Normalize() Vec3 { return a.Scale(1 / a.Len()) }

// Det returns the determinant of the matrix whose columns are a, b, and c.
// It is positive when a, b, c run counterclockwise seen from outside the
// sphere.
func Det(a, b, c Vec3) float64 { return a.Dot(b.Cross(c)) }

// LatLon returns the point on the sphere at a latitude and longitude, in
// radians. The z axis is north, and longitude zero is the x axis.
func LatLon(lat, lon float64) Vec3 {
	sinLat, cosLat := math.Sincos(lat)
	sinLon, cosLon := math.Sincos(lon)
	return Vec3{cosLat * cosLon, cosLat * sinLon, sinLat}
}
