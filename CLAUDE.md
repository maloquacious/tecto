# CLAUDE.md

Guidance for Claude Code in this repository.
Process rules (bug-first priority, checks, verification, version bumps, commits, CI) live in WORKFLOW.md and apply here:

@WORKFLOW.md

Read [PORT_PLAN.md](PORT_PLAN.md) before writing code.
It holds the scope, the interface to Hāmwold, the model, the planned departures from upstream, the verification checks, and the milestones.

## Project

`tecto` simulates plate tectonics on a sphere and returns the resulting elevation.
It is a Go package with a small command-line program around it.
The `maloquacious` organization holds shared code, and this is expected to be some: Hāmwold will import the package.
Its one customer is the world map generator in Hāmwold (`github.com/mdhender/hamwold`, `internal/worldgen`; a local copy is usually at `~/Software/mdhender/hamwold`).

- Go module: `github.com/maloquacious/tecto` (Go 1.26). The module root is the repository root.
- License: AGPL-3.0-or-later. New Go files start with this header:

  ```go
  // Copyright (c) 2026 Michael D Henderson.
  // SPDX-License-Identifier: AGPL-3.0-or-later
  ```

- Milestones 1, 2, and 3 of the plan are in place: the grid, the starting crust, sea level, sampling to the hex map, and plates that are segmented, move, rift, subduct, accrete, and are redrawn every 100 Myr.
  Every cell has crust after every step, and `Generate` reports an account of the continental rock in its summary.
  Milestone 4, the decision point, has been measured twice. The second attempt added three things that make ranges: continents that meet lock together, volcanic arcs rise above subducting ocean floor, and land erodes.
  The decision, made on 2026-11-10, was to go on: the root package's exported names are now taken as stable, and the alpha label is gone. No version is tagged yet; see WORKFLOW.md for when to tag.
  Milestone 5, tuning, is done: the constants are settled, and each is explained where it is defined. What was measured is under "Tuning" in PORT_PLAN.md.
  Milestone 6, Hāmwold integration, is next.

## What it is for

- `tecto` supplies the shape of the world at about 250 km and up: where the continents are and where the high ground runs.
  Hāmwold supplies everything finer from its own noise.
- The output replaces one call in Hāmwold, the `g.fracture(...)` that builds `height` in `worldgen.Generate`.
  Hāmwold floods and ranks that field itself, so it uses only the ordering of the heights.
- The point of the exercise is mountain ranges with a direction to them.
  Milestone 4 in the plan is where it is decided whether the plates deliver that, and the project may stop there.

Keep to that. Do not add climate, a hydrosphere, a biosphere, orbits, interactive display, intermediate results, or saving and resuming a run.

## The upstream source

The algorithms come from the lithosphere model of [tectonics.js](https://github.com/davidson16807/tectonics.js), read at commit `e43927ce88e766b63e35e896f36bfb882a4c8de4`.

- **The JavaScript describes the algorithms. It is not a reference to match.**
  Do not test against it, port its random number generator, or copy its mesh ordering, float rounding, caches, or integer widths.
- Read it to learn what a step does, then write the step in Go and define its correctness with a Go test.
- Where the plan lists a departure from upstream, follow the plan.
  Where upstream looks wrong and the plan is silent, say so and add the departure to the plan with the change.
- Keep any checkout of it under `tmp/`. Do not vendor it into the repository.
- tectonics.js is licensed under CC BY 4.0.
  A package that follows its model credits it in the package comment, and constants taken from the research it cites carry the citation where they are used.

## Layout

Planned, and created as each part is first needed:

- The root package, `tecto`, is the public surface: the parameters, `Generate`, and the result. Its exported names are a promise to Hāmwold; keep them few.
- `cmd/tecto` parses flags, calls the package, and writes files. It holds no logic of its own.
- `internal/cerrs` is constant errors: sentinel errors declared as constants.
- `internal/sphere` is the icosahedral grid, its lookups, and the field operations.
- `internal/crust` is rock columns, thickness, density, isostasy, and sea level.
- `internal/plates` is segmentation, velocity, motion, merging, rifting, subduction, accretion, arcs, collision, and the cycle.
- `internal/erosion` is erosion: which rock moves where. `internal/plates` applies it to the plates.
- `internal/hexmap` rotates the sphere and samples it onto Hāmwold's hex raster.
- `tmp/` holds build output and scratch files. It is git-ignored.

Update this section when the layout changes.

## The model's rules

- A cell holds a column of rock: the mass of each rock type per square metre, and the crust's age.
  Elevation is never stored; it is derived from the column.
- Constants that depend on the size of a cell are written as lengths and areas in kilometres, not as counts of cells, so the model behaves alike at any detail.
- The caller gives the total simulated time. The model picks its own timestep.
- A run is a pure function of its parameters. The package reads and writes no files; only the command does.
  Randomness comes from `math/rand/v2`, with a separate stream for each use derived from the seed.
  Never iterate a Go map where the order can reach the output.
- A non-finite value, a cell with no crust, or negative mass is an error that stops the run. Do not clamp it away.
- The hex layout must match Hāmwold's: flat-top hexes in odd-q offset coordinates, indexed `row*columns + column`, row 0 at the north edge, wrapping east to west.
  Check `internal/worldgen/grid.go` in Hāmwold rather than trusting memory, and change the two together.

## Go

- Follow the `modern-go-guidelines:use-modern-go` skill when writing Go.
- Use the standard library first. Add a dependency only when it pays for itself.
  The command line is the standard `flag` package. Move to `github.com/peterbourgon/ff/v4`, which Hāmwold uses, only when the standard package becomes an obstacle, and ask first.
- Calculate in `float64`. Use `float32` only for stored fields, and only if memory calls for it.
- Cell and plate ids are `int32`.
- `main` only sets up signals and exits.
  The work happens in `run`, which tests call directly with the flags a user would type.
- The package does not print. The command sends progress and the run summary to stderr, and data only to the files named by flags.
