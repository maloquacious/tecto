// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package crust

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/maloquacious/tecto/internal/sphere"
)

// Cuts is the number of times RandomRanks cuts the sphere in two.
const Cuts = 1000

// CutWidthKm is the distance from a cut over which the step from one side
// to the other is spread. It is about the size of the smallest feature the
// model is meant to supply. tectonics.js spreads the step over about 40 km,
// which is narrower than a cell at any detail it runs at, so its field is
// whole numbers with ties.
const CutWidthKm = 250.0

// RandomRanks returns a random smooth field over the cells at pos. The
// field's values mean nothing but their order: a cell with a higher value
// is to be higher ground.
//
// The field is the sum of Cuts steps. Each step cuts the sphere in half
// along a random great circle, raising one side by one and lowering the
// other, with the step spread over CutWidthKm. The method is Hugo Elias's
// for terrain on a sphere, as tectonics.js uses it.
func RandomRanks(pos []sphere.Vec3, rng *rand.Rand) []float64 {
	normals := make([]sphere.Vec3, Cuts)
	for i := range normals {
		z := 2*rng.Float64() - 1
		r := math.Sqrt(1 - z*z)
		sin, cos := math.Sincos(2 * math.Pi * rng.Float64())
		normals[i] = sphere.Vec3{X: r * cos, Y: r * sin, Z: z}
	}
	// A cell's distance from the plane of a cut, over the width.
	scale := sphere.RadiusKm / CutWidthKm
	ranks := make([]float64, len(pos))
	for i, p := range pos {
		sum := 0.0
		for _, n := range normals {
			sum += math.Tanh(scale * p.Dot(n))
		}
		ranks[i] = sum
	}
	return ranks
}

// The share of the crust that is ocean floor, and the spread of elevations
// within the ocean floor and within the continents, in metres relative to
// sea level. The spreads are Earth's, as tectonics.js gives them.
//
// The share is not Earth's, which is six tenths. With five tenths, 0.46 of
// the sphere is land, and Hāmwold, which floods its maps to 0.35, drowns a
// quarter of it: the coast is then Hāmwold's flood line and not the edge of
// the model's continents. In return its maps have a third more mountains,
// in ranges as long for their width, on sixteen seeds at 150, 300, and 500
// Myr. With seven tenths the model's land is 0.30, Hāmwold must make up the
// rest out of ocean floor, and its maps have a third fewer mountains than
// with six.
const (
	OceanShare         = 0.5
	oceanMeanElevation = -4019.0
	oceanDeviation     = 1113.0
	landMeanElevation  = 797.0
	landDeviation      = 1169.0
)

// targetElevation draws one elevation from Earth's two-humped spread.
func targetElevation(rng *rand.Rand) float64 {
	if rng.Float64() < OceanShare {
		return oceanMeanElevation + oceanDeviation*rng.NormFloat64()
	}
	return landMeanElevation + landDeviation*rng.NormFloat64()
}

// TargetElevations turns ranks into elevations, in metres relative to sea
// level, that are spread as Earth's are. It draws one elevation for each
// cell and hands them out in rank order: the lowest to the cell with the
// lowest rank. Cells of equal rank take theirs in cell order.
func TargetElevations(ranks []float64, rng *rand.Rand) []float64 {
	order := make([]int32, len(ranks))
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortFunc(order, func(a, b int32) int {
		return cmp.Or(cmp.Compare(ranks[a], ranks[b]), cmp.Compare(a, b))
	})
	samples := make([]float64, len(ranks))
	for i := range samples {
		samples[i] = targetElevation(rng)
	}
	slices.Sort(samples)
	out := make([]float64, len(ranks))
	for i, cell := range order {
		out[cell] = samples[i]
	}
	return out
}

// lerp returns the value at x of the line through the points (xs[i],
// ys[i]), which must be in rising order of x. Beyond either end it returns
// the end's value.
func lerp(xs, ys []float64, x float64) float64 {
	if x <= xs[0] {
		return ys[0]
	}
	for i := 1; i < len(xs); i++ {
		if x <= xs[i] {
			t := (x - xs[i-1]) / (xs[i] - xs[i-1])
			return ys[i-1] + t*(ys[i]-ys[i-1])
		}
	}
	return ys[len(ys)-1]
}

// The typical crust at a target elevation: how thick each rock is, in
// metres, and how old the crust is, in millions of years. Between the
// elevations given, the values run in straight lines.
var (
	// Ocean crust is 7,100 m thick, give or take 800 (White, McKenzie and
	// O'Nions 1992, "Oceanic crustal thickness from seismic measurements
	// and rare earth element inversions"). It gives way to continental
	// crust across the continental slope.
	maficAt        = []float64{-1500, -950}
	maficThickness = []float64{7100, 0}
	// Continental crust, thickest under the highest ground. The shield
	// thickness is from Zandt and Ammon 1995, "Continental crust
	// composition constrained by measurements of crustal Poisson's ratio".
	felsicAt          = []float64{-1500, -950, 840, 8848}
	felsicThickness   = []float64{0, 28300, 36900, 70000}
	sedimentAt        = []float64{-3200, -1500}
	sedimentThickness = []float64{0, 5}
	// The ocean floor is youngest at its ridges and oldest in its deeps;
	// the continents are old throughout.
	ageAt = []float64{-11000, -5000, -4000, -2000, -1500, -900, 840}
	age   = []float64{250, 100, 0, 0, 100, 1000, 1000}
)

// The share of continental rock that cooled underground. The rest erupted.
const felsicPlutonicShare = 0.85

// ColumnAt returns the typical crust for a target elevation, in metres
// relative to sea level.
//
// The target only chooses the rock. The elevation the model goes on to use
// is the one the column floats at (see Column.Elevation), which is near the
// target but not equal to it.
//
// tectonics.js gives these columns as masses reckoned at a density of 2,700
// for continental rock, and then finds their thickness at 2,600. Here the
// thicknesses are the cited ones and the masses follow from the model's own
// densities.
func ColumnAt(elevation float64) Column {
	felsic := lerp(felsicAt, felsicThickness, elevation) * FelsicDensity
	c := Column{
		Sediment:       lerp(sedimentAt, sedimentThickness, elevation) * SedimentDensity,
		FelsicPlutonic: felsic * felsicPlutonicShare,
		FelsicVolcanic: felsic * (1 - felsicPlutonicShare),
		Age:            lerp(ageAt, age, elevation),
	}
	// The thickness given is that of new crust. Old crust of the same mass
	// is thinner.
	c.MaficVolcanic = lerp(maficAt, maficThickness, elevation) * MaficDensityYoung
	return c
}

// Initial returns a random starting crust for the cells at pos: continents
// and ocean floor in equal shares, laid out by RandomRanks. The two
// sources of random numbers are used for the layout and for the elevations.
func Initial(pos []sphere.Vec3, layout, elevations *rand.Rand) []Column {
	targets := TargetElevations(RandomRanks(pos, layout), elevations)
	columns := make([]Column, len(targets))
	for i, e := range targets {
		columns[i] = ColumnAt(e)
	}
	return columns
}
