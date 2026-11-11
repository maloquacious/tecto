// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package crust holds the rock under each cell and what follows from it:
// thickness, density, elevation, and the level of the sea.
//
// The model follows the lithosphere model of tectonics.js
// (https://github.com/davidson16807/tectonics.js), which is licensed under
// CC BY 4.0. This is a new implementation of its algorithms, not a
// translation of its code.
package crust

import (
	"errors"
	"fmt"
	"math"
)

// Densities, in kilograms per cubic metre. They are the ones tectonics.js
// uses.
const (
	SedimentDensity    = 1500.0
	SedimentaryDensity = 2600.0
	MetamorphicDensity = 2800.0
	FelsicDensity      = 2600.0
	// Mafic rock, the rock of the ocean floor, grows denser as it cools:
	// from MaficDensityYoung when new (Carlson and Raskin 1984, "Density of
	// the ocean crust") to MaficDensityOld at MaficLifetime and after.
	MaficDensityYoung = 2890.0
	MaficDensityOld   = 3300.0
	// MantleDensity is what the crust floats in. tectonics.js fitted it to
	// its isostatic model, not to the mantle.
	MantleDensity = 3075.0
)

// MaficLifetime is the age, in millions of years, at which ocean crust
// reaches its greatest density.
const MaficLifetime = 250.0

// Column is the rock under one square metre of a cell: the mass of each
// kind of rock, in kilograms, and the age of the crust, in millions of
// years. Elevation is not stored. It follows from the column.
type Column struct {
	Sediment       float64
	Sedimentary    float64
	Metamorphic    float64
	FelsicPlutonic float64
	FelsicVolcanic float64
	MaficVolcanic  float64
	MaficPlutonic  float64
	Age            float64
}

// Mass returns the column's mass, in kilograms per square metre.
func (c Column) Mass() float64 {
	return c.Sediment + c.Sedimentary + c.Metamorphic +
		c.FelsicPlutonic + c.FelsicVolcanic +
		c.MaficVolcanic + c.MaficPlutonic
}

// MaficDensity returns the density of the column's mafic rock, which
// depends on the crust's age.
func (c Column) MaficDensity() float64 {
	t := min(max(c.Age/MaficLifetime, 0), 1)
	return MaficDensityYoung + t*(MaficDensityOld-MaficDensityYoung)
}

// Thickness returns the column's thickness, in metres.
func (c Column) Thickness() float64 {
	return c.Sediment/SedimentDensity +
		c.Sedimentary/SedimentaryDensity +
		c.Metamorphic/MetamorphicDensity +
		(c.FelsicPlutonic+c.FelsicVolcanic)/FelsicDensity +
		(c.MaficVolcanic+c.MaficPlutonic)/c.MaficDensity()
}

// Density returns the column's mean density. A column with no rock has
// none, and the result for it is not finite.
func (c Column) Density() float64 { return c.Mass() / c.Thickness() }

// Elevation returns the height of the column's top, in metres, when the
// column floats in the mantle: thickness × (1 − density / mantle density).
// It is measured from the level the mantle would rise to with no crust on
// it, so ocean floor comes out a little above zero and continents several
// kilometres above that. A column denser than the mantle comes out below
// zero.
func (c Column) Elevation() float64 {
	// The same expression with the density multiplied through, so that a
	// column with no rock gives zero and not a division by zero.
	return c.Thickness() - c.Mass()/MantleDensity
}

// Continental returns the mass of the column's continental rock, in
// kilograms per square metre: everything but the mafic rock of the ocean
// floor. The mantle takes ocean floor back and makes it anew, but this rock
// stays at the surface, so the model keeps account of it.
func (c Column) Continental() float64 {
	return c.Sediment + c.Sedimentary + c.Metamorphic +
		c.FelsicPlutonic + c.FelsicVolcanic
}

// Check returns an error if the column cannot be: a mass or age that is
// negative or not finite, or no rock at all.
func (c Column) Check() error {
	for _, v := range [...]float64{
		c.Sediment, c.Sedimentary, c.Metamorphic,
		c.FelsicPlutonic, c.FelsicVolcanic,
		c.MaficVolcanic, c.MaficPlutonic, c.Age,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("value is not finite: %+v", c)
		}
		if v < 0 {
			return fmt.Errorf("value is negative: %+v", c)
		}
	}
	if c.Mass() <= 0 {
		return errors.New("no crust")
	}
	return nil
}

// Check returns an error if any column cannot be, as Column.Check finds.
func Check(columns []Column) error {
	for cell, c := range columns {
		if err := c.Check(); err != nil {
			return fmt.Errorf("crust: cell %d: %w", cell, err)
		}
	}
	return nil
}

// Elevations returns the elevation of each column.
func Elevations(columns []Column) []float64 {
	out := make([]float64, len(columns))
	for i, c := range columns {
		out[i] = c.Elevation()
	}
	return out
}

// OceanMeanDepth is how deep the ocean would be, in metres, if it were
// spread evenly over the whole sphere, land included. It is the value
// tectonics.js uses, and it fixes the ocean's volume.
const OceanMeanDepth = 2000.0

// MeanDepth returns the depth of the sea at a level, averaged over the
// whole sphere: each cell counts by its area, and a cell above the level
// counts as zero.
func MeanDepth(elevation, area []float64, level float64) float64 {
	var volume, total float64
	for i, e := range elevation {
		volume += area[i] * max(level-e, 0)
		total += area[i]
	}
	return volume / total
}

// SeaLevel returns the level at which the sea's mean depth, as MeanDepth
// reckons it, is meanDepth. That is the level at which the ocean holds a
// fixed volume of water. It is found by bisection.
func SeaLevel(elevation, area []float64, meanDepth float64) (float64, error) {
	if len(elevation) == 0 || len(elevation) != len(area) {
		return 0, fmt.Errorf("crust: sea level: %d elevations and %d areas", len(elevation), len(area))
	}
	if !(meanDepth >= 0) || math.IsInf(meanDepth, 0) {
		return 0, fmt.Errorf("crust: sea level: mean depth %v", meanDepth)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, e := range elevation {
		if math.IsNaN(e) || math.IsInf(e, 0) {
			return 0, fmt.Errorf("crust: sea level: elevation %v is not finite", e)
		}
		lo, hi = min(lo, e), max(hi, e)
	}
	// At the lowest elevation the sea is empty. With every cell under
	// water, each metre the level rises adds a metre of mean depth, so
	// meanDepth above the highest elevation is always enough.
	hi += meanDepth
	// The mean depth rises with the level, so halve the interval until it
	// is as narrow as floating point allows.
	for range 200 {
		mid := lo + (hi-lo)/2
		if mid <= lo || mid >= hi {
			break
		}
		if MeanDepth(elevation, area, mid) < meanDepth {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi, nil
}

// Gravity is the pull of the world at its surface, in metres per second
// squared.
const Gravity = 9.8

// Buoyancy returns the force with which the mantle pushes the column up,
// in newtons per cubic metre of crust. It is zero for a column lighter than
// the mantle, which floats, and below zero for a denser one, which would
// sink if the crust around it let it.
func (c Column) Buoyancy() float64 {
	return min(Gravity*(MantleDensity-c.Density()), 0)
}

// OceanCrustThickness is the thickness of new ocean crust, in metres: 7,100,
// give or take 800 (White, McKenzie and O'Nions 1992, "Oceanic crustal
// thickness from seismic measurements and rare earth element inversions").
const OceanCrustThickness = 7100.0

// NewOcean returns a column of new ocean crust, of no age.
func NewOcean() Column {
	return Column{MaficVolcanic: OceanCrustThickness * MaficDensityYoung}
}
