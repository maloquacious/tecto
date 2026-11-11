// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package tecto simulates plate tectonics on a sphere and returns the
// resulting elevation, for the Hāmwold world map generator.
//
// Most of the model follows the lithosphere model of tectonics.js
// (https://github.com/davidson16807/tectonics.js), which is licensed under
// CC BY 4.0. This is a new implementation of its algorithms, not a
// translation of its code.
//
// Two parts of the model are not from tectonics.js: continents that meet
// lock together and do not slide over each other, and volcanic arcs rise
// above ocean floor that goes down into the mantle. They are what make
// mountain ranges here. Erosion, plate velocity, and the handling of plate
// margins follow its rules but depart from them in detail.
package tecto

import (
	"github.com/maloquacious/semver"
)

var (
	version = semver.Version{
		Major: 0,
		Minor: 9,
		Patch: 0,
	}
)

// Version returns the version of the package.
func Version() semver.Version {
	return version
}
