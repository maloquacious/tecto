# tecto plan

## Purpose

`tecto` simulates plate tectonics on a sphere and returns the resulting elevation.
It is a Go package with a small command-line program around it.
Its one customer is the world map generator in [hamwold](https://github.com/mdhender/hamwold) (`internal/worldgen`), which is expected to import the package.
Shared code of this kind lives in the `maloquacious` organization.

The algorithms come from the lithosphere model of [tectonics.js](https://github.com/davidson16807/tectonics.js), read at commit `e43927ce88e766b63e35e896f36bfb882a4c8de4` (August 22, 2020).
The JavaScript is a description of the algorithms, not a reference to match.
Nothing here is tested against it, and its limits and shortcuts are not preserved.

### In scope

- An icosahedral grid on a sphere, and the field operations the model needs on it.
- A seeded initial crust.
- Plates: segmentation, rigid motion, merging, rifting, subduction, and accretion.
- Erosion.
- Isostatic elevation and a sea level.
- Sampling the sphere onto hamwold's hex map.
- A command that runs the package and writes the result to a file, for inspection.

### Out of scope

- Matching tectonics.js output for any seed.
- Climate, hydrosphere, biosphere, stars, and orbits. Hamwold has its own temperature and moisture.
- Rendering beyond a debug image, interactive display, and intermediate results.
- Saving and resuming a run. A run is cheap enough to repeat from its seed.

## What this is for, and what it is not

Hamwold's default map is 1,920 by 800 hexes of 25 km: 1,536,000 hexes.
The model's cells are far larger than a hex.

| Detail | Cells   | Area per cell | Hexes per cell | Cell spacing |
| ------ | ------- | ------------- | -------------- | ------------ |
| 5      | 10,242  | ~49,800 km²   | ~92            | ~240 km      |
| 6      | 40,962  | ~12,450 km²   | ~23            | ~120 km      |
| 7      | 163,842 | ~3,100 km²    | ~6             | ~60 km       |

So `tecto` supplies the shape of the world at 250 km and up: where the continents are, and where the high ground runs.
Hamwold keeps supplying everything finer, from its own noise.

The reason to do this at all is one open item in hamwold's `docs/terrain.md`: mountains come out as massifs, not ranges, because the height field has no direction to it.
A plate model gives the high ground a direction, because crust thickens along the margins where plates converge.

At time zero the model offers nothing hamwold lacks.
Its initial terrain is random cuts ranked onto a height curve, which is what hamwold's `fracture` and `ranks` already do.
All the value is in running the plates, and that is not yet known to be enough: see the decision point under Milestones.

## The interface to hamwold

`tecto` replaces one call in hamwold: the `g.fracture(...)` that builds `height` in `worldgen.Generate`.
Everything after it stays: hamwold floods the field to its own land fraction, ranks the land onto its own elevation curve, and derives landforms and climate.
Hamwold therefore uses only the ordering of `tecto`'s heights, not its metres or its sea level.

The package returns one height per hex, in hamwold's layout:

- Flat-top hexes in odd-q offset coordinates, indexed `row*columns + column`, row 0 at the north edge.
- Latitude is linear in the row, as in hamwold's `grid.latitude`: an odd column sits half a row lower.
- Longitude is linear in the column, and the map wraps east to west.
- A height is in metres relative to the model's sea level, as `float32`.

Each hex takes its height by interpolating between the three cells of the grid triangle that contains it, not from the nearest cell.
Nearest-cell sampling would give hamwold terraces five hexes wide with cliffs between them.

The sphere is rotated before sampling so the largest landmass is centred on the equator at the middle column.
This does the job of hamwold's `centeringSlide`, and a rotated sphere has no seam.

The package's entry point takes the parameters (seed, simulated time, detail, columns, rows) and returns the heights with a summary of the run.
It reads and writes nothing, so hamwold's `worldgen.Generate` stays a pure function of its parameters.

The command writes the same heights to a file: a small header (magic, version, columns, rows, seed, detail, simulated time) followed by the heights, little-endian.
It writes a debug PNG of the raster on request.

## The model

Each cell holds a column of rock: the mass of each rock type per square metre, and the crust's age.
Elevation is never stored. It is derived from the column.

| Step | What it does | Read from |
| ---- | ------------ | --------- |
| Grid | Subdivided icosahedron: positions, neighbours, cell areas, nearest-cell and containing-triangle lookup | `precompiled/rasters/Grid.js`, `VoronoiSphere.js` |
| Field operations | Gradient, diffusion, binary dilation and erosion, flood fill | `precompiled/rasters/` |
| Initial heights | The sum of many random smoothed half-space cuts of the sphere, as a rank | `SphericalGeometry.get_random_surface_field` |
| Initial crust | Ranks mapped to a two-humped height distribution (ocean floor and continent), then to rock columns by height | `CrustGenerator.js` |
| Thickness and density | Thickness from each pool's mass and density; ocean crust grows denser with age over 250 Myr | `Crust.get_thickness`, `get_density` |
| Elevation | Isostasy: `thickness × (1 − density / mantle density)` | `FluidMechanics.get_isostatic_displacements` |
| Sea level | The level at which the ocean holds a fixed volume, by bisection | `Hydrology.solve_sealevel` |
| Plate segmentation | Smooth the crust's buoyancy into a pressure field, take its gradient as mantle flow, and flood-fill regions of like flow into at most seven plates | `Lithosphere.resetPlates`, `Tectonophysics.guess_plate_map`, `VectorImageAnalysis.js` |
| Plate velocity | Dense crust at a plate's edge pulls the plate toward that edge; the pulls are averaged into one rotation for the rigid plate | `Tectonophysics.guess_plate_velocity`, `get_plate_rotation_matrix3x3` |
| Plate motion | Each plate keeps its crust in its own frame and a rotation to the world frame | `Plate.js` |
| Merge | Build the world's crust from the plates. Where plates overlap, the least dense is on top, and continental rock from all of them stacks | `merge_plates_to_master`, `Crust.overlap` |
| Rifting | Where plates have parted, the gap fills with new ocean crust, joined to the plate beside it | `update_rifting` |
| Subduction | Where a plate lies under another and is denser than the mantle, its edge is removed. Its continental rock is added to the plate above | `update_subducted` |
| Erosion | Rock moves from a cell to each lower neighbour, in proportion to the drop | `Crust.model_erosion` |
| Supercontinent cycle | Every 150 Myr the plates are discarded and segmented afresh from the current crust | `SupercontinentCycle.js` |

Mountains come from two of these steps: continental rock stacking where plates overlap, and accretion above a subducting edge.
Both act along plate margins, which is what makes ranges.

The rock reactions (weathering, lithification, metamorphosis) are left out to begin with.
They move mass between rock types of similar density, so they change elevation little, and upstream has weathering switched off.
They are added only if a measured run shows the lack.

## Departures from upstream

These are decided now, because nothing obliges the Go to repeat them.

- **Integer widths.** Cell and plate ids are `int32`. Upstream's `Uint16` ids cap the grid at detail 6.
- **Lengths, not cell counts.** Upstream dilates plate edges by 5 cells and drops plates under 200 cells, at a default detail of 5. Those become about 1,200 km and 10 million km², so the model behaves alike at any detail. Smoothing for the pressure field becomes a length too, replacing upstream's fixed detail-4 side grid.
- **The timestep is internal.** Upstream rifts and subducts one ring of cells per step, whatever the step's length. `tecto` picks its own step so the fastest plate moves less than one cell, and the caller gives only the total time.
- **Random numbers.** `math/rand/v2` PCG, with a separate stream for each use, derived from the seed. Hamwold can pass a seed drawn from its own domains.
- **Arithmetic.** `float64` for calculation; `float32` only for stored fields if memory calls for it.
- **Plate velocity.** Upstream calls its own velocity model mostly wrong, and negates a rotation without knowing why. Implement it as written first, since it is known to produce moving plates, then fix the sign by derivation and test.
- **A plain copy bug.** Upstream's `RockColumn` sets `mafic_plutonic` from `mafic_volcanic`, so new ocean crust is doubled. Not repeated.
- **Smooth cuts.** Upstream spreads each of the initial terrain's cuts over about 40 km, less than a cell, so its field is whole numbers and cells of equal rank are ordered by cell id. `tecto` spreads a cut over 250 km, about the smallest feature it is meant to supply, so the field is continuous at any detail.
- **Thicknesses, not masses, for the starting crust.** Upstream writes the starting columns as masses reckoned at a density of 2,700 for continental rock, then finds their thickness at 2,600. `tecto` starts from the cited thicknesses and takes the masses from its own densities.
- **Sea level by area.** The sea's mean depth weighs each cell by its area, and the bisection runs until it can go no further, not for ten steps.
- **No caches.** Upstream's `Memo` and its display-speed thresholds exist for the browser. Derived fields are recomputed where they are needed, in a fixed order.

## Layout

```text
tecto.go            the public package: parameters, Generate, the result
version.go          the version
cmd/tecto/          flags, calling the package, writing files
internal/sphere/    grid, lookups, field operations
internal/crust/     rock columns, thickness, density, isostasy, sea level
internal/plates/    segmentation, velocity, motion, merge, rifting, subduction
internal/erosion/   erosion
internal/hexmap/    rotation and sampling to the hex raster
```

The public surface is the root package and nothing else.
Everything under `internal/` can change freely; the root package's exported names are a promise to hamwold.
Packages are split when a boundary has earned it, not before.

The command uses the standard `flag` package.
It moves to `github.com/peterbourgon/ff/v4`, which hamwold uses, only if the standard package becomes an obstacle.

## Command line

```sh
tecto --seed 42 --myr 300 --detail 6 --columns 1920 --rows 800 --output world.tecto
tecto --seed 42 --myr 300 --png world.png
```

- `--myr` is the simulated time. Zero writes the initial crust.
- Progress and a summary go to stderr: plates, land fraction, height percentiles, and elapsed time.
- A run that produces a non-finite value, or loses a plate's crust, fails with a message. It does not write a file.
- The command holds no logic of its own. Anything it can do, the package can do.

## Verification

With no reference to compare against, each layer needs its own definition of correct.

| Layer | Check |
| ----- | ----- |
| Grid | V − E + F = 2. Exactly 12 cells have five neighbours and the rest six. Adjacency is mutual. Cell areas sum to 4π. |
| Lookups | Nearest cell and containing triangle agree with a brute-force search on random points. |
| Field operations | The gradient of a field linear in x is the tangent part of the x axis. Diffusion keeps the area-weighted mean and lowers the variance. Dilation then erosion contains the original set. |
| Crust | Thickness and elevation match hand-worked columns. Uniform crust gives uniform elevation. |
| Sea level | The ocean's volume at the solved level is the volume asked for. |
| Sampling | A field equal to latitude samples to hamwold's `grid.latitude` for every hex. The raster is continuous across the east–west wrap. |
| Segmentation | Every cell is in exactly one plate. Each plate is connected and no smaller than the minimum. |
| Motion | Rotations stay orthonormal. A plate rotated by a known angle keeps its area and moves its centre by that angle. |
| Merge, rifting, subduction | After a step every cell has crust. No mass is negative. Continental mass changes only by the amounts the step reports as created or destroyed. |
| Erosion | Mass is conserved exactly. An isolated peak gets lower and its neighbours higher. A flat field does not change. |
| Whole run | The same seed gives the same file, byte for byte. Land fraction and height percentiles stay in stated ranges across twenty seeds. |
| Ranges | High ground is nearer to convergent margins than land in general is. High ground is elongated: its connected regions are longer than they are wide. |

The last row is the test that the tectonics are doing their job.
The two measures are computed by a test helper and reported in the run summary, so they can be tracked as the model changes.

## Milestones

Each one ends with something that runs.

1. **The pipe.** Grid, lookups, initial crust, isostasy, sea level, rotation, sampling, file, and PNG.
   Hamwold gains a way to use `tecto`'s heights in place of `fracture`, behind a flag.
   *Done when* `hw map` renders a `tecto` world at zero Myr, and it looks about like today's maps.
2. **Plates that move.** Segmentation, velocity, motion, and merge.
   *Done when* plates drift for 150 Myr with the segmentation and motion checks passing. Gaps and overlaps are expected and reported.
3. **Margins.** Rifting, subduction, accretion, and the supercontinent cycle.
   *Done when* the mass budget closes and no cell is left empty over 500 Myr.
4. **Decision point.** Generate hamwold maps from `tecto` at 150, 300, and 500 Myr for several seeds, with hamwold's ruggedness noise on top.
   Compare them with current maps by eye and by the two range measures.
   If the ranges are not clearly better, stop here.
   Through this milestone the package is alpha and untagged, and hamwold reaches it through a `replace` directive.
   If the decision is to go on, the package's exported names are taken as stable, the alpha label is dropped, and versions are tagged from then on (see WORKFLOW.md).
5. **Erosion and tuning.** Erosion, then the constants: plate count, cycle length, ocean share, and smoothing lengths.
   Try detail 7.
6. **Hamwold integration.** Document the package and the file format, tag a version, and have hamwold require it in place of its `replace` directive.

Sizes, for scale: the upstream model code in scope is about 1,500 lines of JavaScript (`Crust`, `Lithosphere`, `Plate`, `Tectonophysics`, and parts of four others), plus the grid and the field operations it reaches.

## Not yet decided

- **Starting islands.** Hamwold needs islands of 150 to 1,800 hexes, which is 6 to 78 cells at detail 6. The model will make few of them. They may have to come from hamwold's noise, or from a pass that adds island arcs along ocean margins.
- **Land fraction.** The model is set up for Earth's share of continental crust, about 40%, and hamwold floods to 35% land. If the flood has to cut far into the continents or far out onto the ocean floor, the initial ocean share needs to follow hamwold's parameter.
- **The starting crust's height.** With upstream's ocean volume, the starting continents float about 1,800 m above the sea and the ocean floor lies about 3,200 m below it, with little in between. Hamwold uses only the order of the heights, so this does not matter to it yet. It is a constant to look at in milestone 5.
- **One continent at the start.** The starting crust is one large landmass on most seeds, because a sum of cuts through the sphere's centre is dominated by its largest features. The plates are expected to break it up.
- **The poles.** Hamwold covers everything beyond about 80° with ice. Whether the rotation should also keep large landmasses away from the poles is open.
- **How long to run.** Whether maps are best at one supercontinent cycle, several, or part of one is a question for milestone 4.

## Licence and attribution

tectonics.js is licensed under CC BY 4.0.
`tecto` is a new implementation of its algorithms, and credits the project and its author in the README and in the packages that follow its model.
Research the upstream code cites (Schellart 2010 for plate velocity; White, McKenzie and O'Nions 1992 for ocean crust thickness) is cited where the constants are used.
