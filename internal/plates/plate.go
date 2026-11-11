// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package plates

import (
	"math"

	"github.com/maloquacious/tecto/internal/crust"
	"github.com/maloquacious/tecto/internal/sphere"
)

// The terminal speed of a sinking slab, from Schellart 2010, "Evolution of
// subduction zone curvature and its dependence on the trench velocity and
// the slab to upper mantle viscosity ratio", and its supplement: a slab
// that weighs F more than the mantle it displaces sinks at
//
//	v = S·F·(W·L·T)^(2/3) / (18·c·μ)
//
// The size of the slab and the constants are the ones tectonics.js uses.
const (
	slabWidth       = 300e3   // W, metres
	slabLength      = 600e3   // L, metres
	slabThickness   = 100e3   // T, metres
	slabShape       = 0.725   // S
	slabDip         = 4.025   // c
	mantleViscosity = 1.57e20 // μ, pascal seconds

	secondsPerMyr = 1e6 * 365.25 * 24 * 3600
)

// pullPerBuoyancy is the speed, in radians per million years, at which
// crust at a plate's edge would draw the plate along if it were alone, for
// each newton per cubic metre by which the mantle fails to hold it up.
var pullPerBuoyancy = math.Pow(slabWidth*slabLength*slabThickness, 2.0/3) *
	slabShape / (18 * slabDip * mantleViscosity) /
	(sphere.RadiusKm * 1000) * secondsPerMyr

// Plate is a rigid piece of the crust. It keeps its crust on a grid of its
// own, which turns with it: a cell of the plate stays the same cell however
// far the plate travels.
type Plate struct {
	// Member says which cells of the plate's grid the plate covers.
	Member []bool
	// Crust is the rock under each cell the plate covers.
	Crust []crust.Column
	// Rot takes a point on the plate's grid to where it is in the world.
	Rot sphere.Mat3
	// Area is the plate's area on the unit sphere, in steradians.
	Area float64

	// What follows is worked out from Member by outline.

	// center is the middle of the plate on its own grid, and reach the
	// cosine of the angle within which all of it lies.
	center sphere.Vec3
	reach  float64
	// edge is the cells of the plate that touch a cell outside it.
	edge []edgeCell
	// drag relates the turning force on the plate to the rate it turns at:
	// the mean over the plate of I − r·rᵀ.
	drag sphere.Mat3
}

// edgeCell is a cell at the edge of a plate, and the direction along the
// ground that leads straight out of the plate from it.
type edgeCell struct {
	cell int32
	out  sphere.Vec3
}

// newPlate returns the plate that covers the given cells, where it lies in
// the world now.
func newPlate(g *sphere.Grid, member []bool, columns []crust.Column) *Plate {
	p := &Plate{
		Member: member,
		Crust:  make([]crust.Column, len(columns)),
		Rot:    sphere.Identity(),
	}
	for cell, in := range member {
		if in {
			p.Crust[cell] = columns[cell]
		}
	}
	p.outline(g)
	return p
}

// outline works out everything that depends on which cells the plate
// covers. It is called again whenever they change.
func (p *Plate) outline(g *sphere.Grid) {
	var sum sphere.Vec3
	var drag sphere.Mat3
	mask := make([]float64, len(p.Member))
	p.Area = 0
	for cell, in := range p.Member {
		if !in {
			continue
		}
		mask[cell] = 1
		r, a := g.Pos[cell], g.Area[cell]
		p.Area += a
		sum = sum.Add(r.Scale(a))
		drag[0] = drag[0].Add(sphere.Vec3{X: 1}.Sub(r.Scale(r.X)).Scale(a))
		drag[1] = drag[1].Add(sphere.Vec3{Y: 1}.Sub(r.Scale(r.Y)).Scale(a))
		drag[2] = drag[2].Add(sphere.Vec3{Z: 1}.Sub(r.Scale(r.Z)).Scale(a))
	}
	p.edge = p.edge[:0]
	if p.Area == 0 {
		// A plate that has gone wholly into the mantle covers nothing, and
		// no point is within its reach.
		p.center, p.reach, p.drag = sphere.Vec3{Z: 1}, 2, sphere.Mat3{}
		return
	}
	for i := range drag {
		drag[i] = drag[i].Scale(1 / p.Area)
	}
	p.drag = drag

	// The cap that holds the plate, with room for a point to be nearest to
	// a cell without being inside the cap of the cells. A plate spread so
	// evenly that it has no middle gets the whole sphere.
	p.center, p.reach = sphere.Vec3{Z: 1}, -1
	if sum.Len() > 1e-9*p.Area {
		p.center = sum.Normalize()
		widest := 1.0
		for cell, in := range p.Member {
			if in {
				widest = min(widest, g.Pos[cell].Dot(p.center))
			}
		}
		angle := math.Acos(max(widest, -1)) + 2*g.Spacing()
		if angle < math.Pi {
			p.reach = math.Cos(angle)
		}
	}

	// The mask rises going into the plate, so the way out is down its
	// slope.
	for cell, in := range p.Member {
		if !in {
			continue
		}
		atEdge := false
		for _, v := range g.Neighbors[cell] {
			if !p.Member[v] {
				atEdge = true
				break
			}
		}
		if !atEdge {
			continue
		}
		slope := g.GradientAt(mask, int32(cell))
		if steep := slope.Len(); steep > 0 {
			p.edge = append(p.edge, edgeCell{int32(cell), slope.Scale(-1 / steep)})
		}
	}
}

// AngularVelocity returns the rate at which the plate turns, on its own
// grid: a vector along the axis it turns about, counterclockwise seen from
// the vector's tip, whose length is the rate in radians per million years.
// A point r of the plate moves at the vector crossed with r.
//
// Crust at the plate's edge that is denser than the mantle sinks, and draws
// the plate after it: out across that edge. Each such cell, alone, would
// move the plate at the terminal speed of a sinking slab. The plate is
// rigid and can only turn, so the cells' pulls are combined as turning
// forces about the world's center, r × v, averaged over the cells that
// pull. The mantle under the whole plate resists in proportion to how fast
// each part of it moves, and the plate turns at the rate at which that
// resistance balances the pull.
func (p *Plate) AngularVelocity(g *sphere.Grid) sphere.Vec3 {
	var torque sphere.Vec3
	var pulling float64
	for _, e := range p.edge {
		sinking := -p.Crust[e.cell].Buoyancy()
		if sinking <= 0 {
			continue
		}
		r, a := g.Pos[e.cell], g.Area[e.cell]
		v := e.out.Scale(sinking * pullPerBuoyancy)
		torque = torque.Add(r.Cross(v).Scale(a))
		pulling += a
	}
	if pulling == 0 {
		return sphere.Vec3{}
	}
	return p.drag.Solve(torque.Scale(1 / pulling))
}

// speed returns the speed of the fastest part of the plate when it turns at
// a rate, in radians per million years.
func (p *Plate) speed(g *sphere.Grid, rate sphere.Vec3) float64 {
	if rate == (sphere.Vec3{}) {
		return 0
	}
	var fastest float64
	for cell, in := range p.Member {
		if in {
			fastest = max(fastest, rate.Cross(g.Pos[cell]).Len())
		}
	}
	return fastest
}

// turn moves the plate as it turns at a rate for a time, in millions of
// years, and ages its crust by that time.
func (p *Plate) turn(rate sphere.Vec3, myr float64) {
	p.Rot = p.Rot.Mul(sphere.Rotation(rate.Scale(myr))).Orthonormal()
	for cell, in := range p.Member {
		if in {
			p.Crust[cell].Age += myr
		}
	}
}
