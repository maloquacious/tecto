// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Mat3 is a three by three matrix, held as its rows.
type Mat3 [3]Vec3

// Identity returns the matrix that changes nothing.
func Identity() Mat3 {
	return Mat3{{X: 1}, {Y: 1}, {Z: 1}}
}

// Rotation returns the rotation whose axis is the direction of v and whose
// angle, in radians, is the length of v. It turns counterclockwise seen
// from the tip of v, so a point p moves, for a small v, by v × p.
func Rotation(v Vec3) Mat3 {
	angle := v.Len()
	if angle == 0 {
		return Identity()
	}
	// Rodrigues' formula.
	k := v.Scale(1 / angle)
	sin, cos := math.Sincos(angle)
	t := 1 - cos
	return Mat3{
		{cos + t*k.X*k.X, t*k.X*k.Y - sin*k.Z, t*k.X*k.Z + sin*k.Y},
		{t*k.Y*k.X + sin*k.Z, cos + t*k.Y*k.Y, t*k.Y*k.Z - sin*k.X},
		{t*k.Z*k.X - sin*k.Y, t*k.Z*k.Y + sin*k.X, cos + t*k.Z*k.Z},
	}
}

// Apply returns the matrix times the vector.
func (m Mat3) Apply(v Vec3) Vec3 {
	return Vec3{m[0].Dot(v), m[1].Dot(v), m[2].Dot(v)}
}

// Transpose returns the matrix with its rows and columns exchanged. For a
// rotation, that is the rotation that undoes it.
func (m Mat3) Transpose() Mat3 {
	return Mat3{
		{m[0].X, m[1].X, m[2].X},
		{m[0].Y, m[1].Y, m[2].Y},
		{m[0].Z, m[1].Z, m[2].Z},
	}
}

// Mul returns the product m·n: the matrix that does n and then m.
func (m Mat3) Mul(n Mat3) Mat3 {
	t := n.Transpose()
	return Mat3{
		{m[0].Dot(t[0]), m[0].Dot(t[1]), m[0].Dot(t[2])},
		{m[1].Dot(t[0]), m[1].Dot(t[1]), m[1].Dot(t[2])},
		{m[2].Dot(t[0]), m[2].Dot(t[1]), m[2].Dot(t[2])},
	}
}

// Orthonormal returns the rotation nearest to hand for a matrix that has
// drifted a little from being one: its rows made of length one and at right
// angles to each other.
func (m Mat3) Orthonormal() Mat3 {
	x := m[0].Normalize()
	y := m[1].Sub(x.Scale(x.Dot(m[1]))).Normalize()
	return Mat3{x, y, x.Cross(y)}
}

// Solve returns the vector x for which m·x = b. If m has no inverse the
// result is not finite.
func (m Mat3) Solve(b Vec3) Vec3 {
	// Cramer's rule, on the columns of m.
	t := m.Transpose()
	det := Det(t[0], t[1], t[2])
	return Vec3{
		Det(b, t[1], t[2]) / det,
		Det(t[0], b, t[2]) / det,
		Det(t[0], t[1], b) / det,
	}
}
