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
The map is then turned about its middle to whichever way leaves the least land beyond 70° of latitude, since hamwold covers its poles with ice.

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
| Collision | Where continents have met, neither can go down. The meeting resists the two plates' moving past each other, and they lock together along a narrow belt | not in upstream |
| Arcs | A tenth of the ocean floor that goes down comes up through the plate above as new continental rock | not in upstream |
| Erosion | Rock moves from a cell of land to each lower neighbour, in proportion to the drop | `Crust.model_erosion` |
| Supercontinent cycle | Every 100 Myr the plates are discarded and segmented afresh from the current crust | `SupercontinentCycle.js` |

Mountains come from three of these steps: continental rock stacking where continents meet, and accretion and arcs above a subducting edge.
All act along plate margins, which is what makes ranges.
Erosion then spreads them and wears the old ones down.

The rock reactions (weathering, lithification, metamorphosis) are left out.
They move mass between rock types of similar density, so they change elevation little, and upstream has weathering switched off.
A measured run in milestone 5 settled it. With weathering off, the only sediment is the starting crust's: under a ten-thousandth of the continental rock, at most 18 m thick after 500 Myr on five seeds. Upstream turns sediment to rock where more than 150 m of it lies, and sedimentary rock to metamorphic under 300 MPa, where the model's has 0.3; so neither reaction would ever act. If all the sediment were turned to rock at once, no cell would move by more than 8 m.

## Departures from upstream

These are decided now, because nothing obliges the Go to repeat them.

- **Integer widths.** Cell and plate ids are `int32`. Upstream's `Uint16` ids cap the grid at detail 6.
- **Lengths, not cell counts.** Upstream dilates plate edges by 5 cells and drops plates under 200 cells, at a default detail of 5. Those become about 1,200 km and 10 million km², so the model behaves alike at any detail. Smoothing for the pressure field becomes a length too, replacing upstream's fixed detail-4 side grid.
- **The timestep is internal.** Upstream rifts and subducts one ring of cells per step, whatever the step's length. `tecto` picks its own step so the fastest plate moves less than one cell, and the caller gives only the total time.
- **Random numbers.** `math/rand/v2` PCG, with a separate stream for each use, derived from the seed. Hamwold can pass a seed drawn from its own domains.
- **Arithmetic.** `float64` for calculation; `float32` only for stored fields if memory calls for it.
- **Plate velocity.** Upstream calls its own velocity model mostly wrong, and negates a rotation without knowing why.
  `tecto` keeps its premise, that each sinking cell at a plate's edge would alone draw the plate out across that edge at a slab's terminal speed, and derives the rest.
  The pulls combine as turning forces about the world's centre, `r × v`, averaged over the cells that pull. Upstream computes `v × r` and negates it, which is the same thing, so its sign was right and its unexplained negation is the order of a cross product.
  Upstream's second rotation, about the plate's centre of mass, is applied about an axis through the world's centre and so is not the spin it is meant to be. It is replaced by resistance from the mantle under the whole plate: the plate turns at the rate `ω` for which `mean(I − r·rᵀ)·ω` over the plate equals the averaged pull. That gives both the plate's travel and its spin, and a spin no faster than the pulls warrant.
  Only cells of the plate pull. Upstream also counts the cells just outside it.
- **Segmentation.** Upstream leaves the ground its flood fills do not reach as one more plate, which may be in several pieces and of any size. `tecto` makes plates of the unclaimed pieces that are large enough, largest first, while there is room, and joins the rest to the plates beside them, so every plate is connected and no smaller than the minimum. Upstream's closing of each plate's outline is left out.
- **Smoothing.** The pressure field is smoothed by diffusion that keeps the field's mean, with cotangent weights, to a spread of 1,900 km. That is the spread of upstream's thirty rounds of neighbour averaging at detail 4.
- **Gradient.** The gradient at a cell is a least-squares fit to its neighbours of a slope and a bowl, and is exact for a field that is linear in space. Upstream's is a weighted average of the slopes to each neighbour.
- **Margins are settled in full each step.** Upstream adds one ring of cells to a plate's trailing edge and takes one ring from its sunken edge each step, and only cells whose neighbours agree with them. `tecto` shares out the whole of every gap among the plates beside it, each taking the cells nearest it, and takes all of a plate's crust that is under another plate, denser than the mantle, and joined to the plate's edge by other such crust. So no cell is ever left bare, and the result does not depend on the length of a step.
- **Accretion keeps mass.** The continental rock of a sunken cell goes at once onto the cell of the plate above it, scaled by the two cells' areas so that no kilogram is made or lost. Upstream writes it to a map of the world and applies that a step later. Upstream also turns all the continental rock of a plate metamorphic wherever the plate is under another; that is one of the rock reactions, and is left out with them.
- **The account of continental rock is kept on the plates.** A plate's crust lives on the plate's own grid, where nothing changes it but subduction and accretion, so the account there closes to rounding. Reading the plates onto the world's grid takes the nearest cell and does not keep the total exactly; the difference is reported as its own entry, each time the plates are redrawn and at the end.
- **The cycle.** The plates are drawn at the start, and again each 100 Myr if the run goes on. Steps end exactly on the end of a cycle. Upstream's cycle is 150 Myr; the shorter one makes more ranges, sooner (see the tuning under Not yet decided).
- **Speed is speed over the ground.** The step is set by the fastest-moving cell of any plate, not by the fastest rate of turning: a small plate may spin fast and move little.
- **Bare cells are bare.** Merging the plates leaves a cell that no plate covers with no crust, which the checks reject. Milestone 2 gave it new ocean floor as a placeholder.
- **Continents that meet lock together.** Upstream lets one continent slide over another without limit, and the doubled crust is a plateau as wide as the overlap.
  In `tecto`, each cell where crust that will never sink lies on other such crust resists the two plates' moving past each other there, 3,000 times as hard as the mantle resists a plate over the same area. The rates of the plates that have met are then found together, each driven as before. A few cells of collision are enough to hold two plates, so the doubled crust is a belt one to three cells wide. The plates can still turn about the belt, and go on together where the rest of their edges pull them.
- **Volcanic arcs.** In upstream, as in `tecto` before this, subduction under a continent raised nothing, because ocean floor that sinks carries almost no continental rock to accrete. Now a tenth of the mass of the floor that goes down is added to the cell of the plate above as felsic rock. The share is from the rate at which Earth's arcs add crust (Reymer and Schubert 1984). It makes a coastal range along a continent's subducting margin, and it is new continental rock, so the account has an entry for it.
- **Erosion wears land, to the sea.** Upstream moves rock down every slope, the sea floor's too, at a rate fixed per cell and per neighbour. `tecto` moves rock only from cells above sea level, in proportion to the drop to the neighbour or to the sea if the neighbour is under it, so sediment builds the shore out and does not drain off the continental slope. The rate is a diffusivity, 200 km² per million years, applied with the mesh's cotangent weights, so it is alike at any detail. A cell gives up at most a quarter of its continental rock in a step. Each kind of rock moves as itself and in proportion; upstream takes the top layers first.
- **Erosion acts on stacked plates alike.** The rock that leaves a cell of the world is taken from every plate with crust there, in the same share, and lands on the uppermost plate of the cell it goes to. The plates' total is kept exactly.
- **The map is turned away from the poles.** Upstream has no map to place. `tecto` puts the largest landmass in the middle of hamwold's map, and of the ways the map can then be turned about its middle, takes the one with the least land beyond 70° of latitude.
- **Half the crust is ocean floor.** Upstream starts with Earth's share, six tenths. `tecto` starts with five, so 0.46 of the sphere is land where 0.38 would be. Hamwold floods its maps to 0.35 and drowns the lowest quarter of that land, so its coast is its own flood line; in return its maps have a third more mountains (see "Tuning").
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
internal/plates/    segmentation, velocity, motion, merge, rifting, subduction, arcs, collision
internal/erosion/   erosion: which rock moves where
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
| Sampling | A field equal to latitude samples to hamwold's `grid.latitude` for every hex. The raster is continuous across the east–west wrap. Turning the map keeps its middle, and leaves no more land at the poles than not turning it: under half as much over twenty seeds. |
| Segmentation | Every cell is in exactly one plate. Each plate is connected and no smaller than the minimum. |
| Motion | Rotations stay orthonormal. A plate rotated by a known angle keeps its area and moves its centre by that angle. |
| Merge, rifting, subduction | After a step every cell has crust. No mass is negative. Continental mass changes only by the amounts the step reports as created or destroyed. |
| Collision | Two continents that overlap by a narrow belt close at under a twentieth of the speed one was driven at, and the one that was met is carried along. Ocean floor under a continent, and plates that do not touch, change no rate. |
| Arcs | The rock added above floor that sinks is the stated share of the floor's mass, and the account closes with it. |
| Erosion | Mass is conserved exactly, for each kind of rock. An isolated peak gets lower and its neighbours higher. A flat field does not change. Rock under the sea does not move, and land wears to sea level, not to the sea floor. |
| Whole run | The same seed gives the same file, byte for byte. Land fraction and height percentiles stay in stated ranges across twenty seeds, for the starting crust and after 300 Myr of plates. |
| Ranges | High ground is nearer to convergent margins than land in general is, where a margin is a place one plate lies over another at the end of the run. High ground is elongated: its connected regions are longer than they are wide. |

The last row is the test that the tectonics are doing their job.
The two measures are computed by a test helper and reported in the run summary, so they can be tracked as the model changes.

## Milestones

Each one ends with something that runs.
Milestones 1 to 5 are done. The decision at milestone 4 was to go on.

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
5. **Tuning.** Erosion came in with milestone 4's second attempt, along with collision and arcs. What is left is the constants: collision drag, the arcs' share, the rate of erosion, plate count, cycle length, ocean share, and smoothing lengths.
   Try detail 7.
   *Done:* the cycle is 100 Myr and erosion 200 km² per million years; the rest stay as they were. Each constant's value is explained where it is defined, and what was measured is under "Tuning" below.
6. **Hamwold integration.** Document the package and the file format, tag a version, and have hamwold require it in place of its `replace` directive.

Sizes, for scale: the upstream model code in scope is about 1,500 lines of JavaScript (`Crust`, `Lithosphere`, `Plate`, `Tectonophysics`, and parts of four others), plus the grid and the field operations it reaches.

## Not yet decided

- **Starting islands.** *Not a requirement.* Starting each player on an island of their own was an early design for the game, and it was abandoned. Hamwold's code may still mark such islands, but nothing depends on how many a map has. Do not count them, tune for them, or add anything to the model to make them. Earlier measurements in this plan counted them; the counts have been taken out, and one decision that leaned on them, the ocean share, is marked under "Tuning".
- **Land fraction.** *Decided after milestone 5.* Half the starting crust is continental, which makes 0.46 of the sphere land, and hamwold floods to 0.35. The flood so cuts into the continents: the lowest quarter of the model's land is under hamwold's sea. That was chosen for the mountains it brings (see "Tuning") and from the look of the maps. The share is a constant and does not follow hamwold's parameter.
- **The starting crust's height.** With upstream's ocean volume, the starting continents float about 1,800 m above the sea and the ocean floor lies about 3,200 m below it, with little in between. Hamwold uses only the order of the heights, so this does not matter to it yet. It is a constant to look at in milestone 5.
- **One continent at the start.** The starting crust is one large landmass on most seeds, because a sum of cuts through the sphere's centre is dominated by its largest features. The plates are expected to break it up.
- **The poles.** *Decided in milestone 5.* Hamwold covers everything beyond about 80° with ice, so the map is turned about its middle to leave the least land beyond 70° (see the departures). On hamwold's maps the share of what is not ocean that lies beyond 70°, ice included, fell from 0.06 to 0.09 to about 0.05. With half the crust continental it is still about 0.05 after the plates have run, though on the starting crust, which is mostly one landmass, turning the map then keeps only a sixth of the polar land away.
- **Plate speeds.** With rifting and subduction the plates no longer run away. At detail 6 over 500 Myr the fastest plate is typically moving at about 100 km per million years, which is the speed of Earth's fastest, with spells of 300 to 700. The slab constants are upstream's. Milestone 5 tried the pull halved and doubled and kept it.
- **Tuning.** *Milestone 5.* Each constant was run at other values, one at a time, through hamwold at its default size on eight seeds at 150, 300, and 500 Myr; the values that looked better, and their combinations, were run again on sixteen. One seed differs from another by a good deal, so only differences that held at all three ages were acted on.
  - Changed: the cycle, from 150 Myr to 100, and erosion, from 400 km² per million years to 200. Together, on sixteen seeds, mountains go from 1.3% of the land to 3.1% at 150 Myr, from 2.4% to 4.5% at 300, and from 5.1% to 7.5% at 500. Mountain regions are 4.5 times as long as wide at 150 Myr where they were 3.1, 3.9 where they were 3.4 at 300, and 3.1 where they were 3.5 at 500; they are 390, 530, and 640 km across. The share of the mountains in regions at least three times as long as wide is 0.64, 0.54, and 0.43, where it was 0.38, 0.45, and 0.44.
  - Kept, because the other values were worse: collision drag (300 and 1,000 let the continents slide; 10,000 gains nothing) and the slab's pull (halved and doubled).
  - Changed afterwards: the ocean share, from 0.6 to 0.5. It was first turned down partly for making few starting islands, which are not a requirement. Run again on sixteen seeds with the cycle and erosion as settled, mountains go from 3.1% of the land to 4.0% at 150 Myr, from 4.5% to 6.2% at 300, and from 7.5% to 10.0% at 500. Their regions are as long for their width (4.7, 3.8, and 3.1) and 25 to 60 km wider (440, 590, and 660 km), and the share in regions at least three times as long as wide is 0.56, 0.55, and 0.40. No more of the land lies near the poles. The cost is that the model's land is 0.46 of the sphere, so hamwold's coast is its own flood line and not the edge of the model's continents. A share of 0.7 has a third fewer mountains than 0.6.
  - Kept, because the other values made no difference that could be told from the seeds': the arcs' share (0.05, 0.2, 0.4), the number of plates (5, 10, 14), the least area of a plate (5 million km²), plate growth (600 km), and the smoothing length (1,000 and 3,000 km).
  - Still short: mountains are 4% to 10% of the land against 14% from the fractured field, and by 500 Myr the ranges are widening as belts pile up beside older ones. The highest cell of the model is 7 to 20 km above the sea. Hamwold bends that over; how it does so, and how much of the land it calls mountains, are its constants and not the model's.
- **Detail 7.** *Tried in milestone 5; the default stays 6.* A run takes eight to ten times as long (24, 43, and 61 s for 150, 300, and 500 Myr, against 2.5, 4.4, and 7.9 s) and about 290 MB against 85. On eight seeds through hamwold the mountains are as many as at detail 6. Their regions are longer for their width at 150 Myr (6.0 against 4.6) and at 500 (3.6 against 3.1) and the same at 300; at 500 Myr they are wider, 760 km against 590. The constants are lengths and areas, and the model's own figures (land, overlap, height percentiles, arcs) come out the same at either detail.
- **Ranges, after a second attempt.** Made after the decision point's findings below, with three additions: continents that meet lock together, volcanic arcs, and erosion (see the departures). On the same five seeds:
  - On the model's cells the high ground is 2.1 times as long as wide at 150 Myr, 2.4 at 300, and 2.9 at 500, where it was 1.9, 1.9, and 2.3.
  - The share of the sphere where plates overlap is 0.04 to 0.06, where it was 0.06 to 0.13, and land stays at 0.38 of the sphere throughout, where it fell to 0.31.
  - The highest cell is 6 to 13 km above the sea. Erosion bounds it, but it is still more than mountains stand; hamwold bends it over.
  - Arcs add about 5% to the continental rock in 500 Myr.
  - Hamwold was changed with it: it no longer ranks the mountains among all the land, but takes the model's rise above the general level of the continents as a height. Its mountain regions are then 3.4 to 3.5 times as long as wide and 320 to 520 km across, and nearly half the mountains are in regions at least three times as long as wide. From the fractured field they are 2.1 times as long as wide and 1,200 km across, with 15% in such regions, and the first attempt's plate maps were no better.
  - Mountains are 1% of the land at 150 Myr, 2% at 300, and 6% at 500, against 14% from the fractured field.
  - Still open: at 150 Myr, and on some seeds at 300, there are few ranges, because the plates have had little time to meet. Island arcs in the open ocean are faint. The constants (3,000, a tenth, 400 km² per million years) were set by eye on a few runs and are for milestone 5 to tune.
- **Plateaus, not yet ranges.** *This is the model as it was before the second attempt above.* Where continents meet, nothing stops one sliding over the other, so the doubled crust is as wide as the overlap: at 300 Myr about a tenth of the sphere is doubled, and by 500 Myr some is trebled and stands over 15 km high. The share of the sphere that is continental crust falls from 0.39 to about 0.29 over 500 Myr as it stacks. Accretion moves very little rock, a few hundredths of a percent of the continents, because crust dense enough to sink carries almost none. Whether the stacked margins read as ranges once hamwold has ranked them is the question for milestone 4; erosion in milestone 5 is what would spread them. A limit on how far continents can override each other is the first thing to try if they do not.
- **Resampling.** Reading the plates onto the world's grid gains or loses up to half a percent of the continental rock over 500 Myr at detail 6, and up to one percent at detail 5, with no trend either way.
- **The decision point's findings.** Measured in milestone 4 on five seeds, through hamwold at its default size:
  - On the model's own cells the high ground grows longer for its width as the plates run: 1.6 at the start, 1.9 at 150 and 300 Myr, 2.3 at 500 Myr.
  - On hamwold's maps it does not. Mountain regions are about twice as long as wide from the fractured field and from the plate model at any age. Hamwold ranks the heights, which flattens a plateau of doubled crust, and then its own noise decides where in the plateau the mountains go.
  - By eye the plate maps have continents with shapes and inland seas the fractured field never makes, and their mountains lie in broad belts toward one side of a continent, not in a massif at its middle. The belts are 500 to 1,500 km wide and broken into blobs.
  - An experiment, not kept: crust that floats was taken from its plate once it lay more than 200 km under another, and its rock piled on the plate above. The high ground then runs in narrow arcs along the margins, which is the shape wanted. But the piles have no limit (239 km high at 300 Myr), a continent that keeps converging is consumed (land falls to 0.19 of the sphere by 500 Myr), and through hamwold the gain is visible but modest. It would need erosion to spread the piles, and something to stop continents converging once they have met. The patch is in `tmp/collision-limit-experiment.patch` of the checkout it was made in.
- **How long to run.** The longer the run, the more mountains: 4% of the land at 150 Myr, 6% at 300, and 10% at 500. The ranges are longest for their width early. Which hamwold should use is for milestone 6.

## Licence and attribution

tectonics.js is licensed under CC BY 4.0.
`tecto` is a new implementation of its algorithms, and credits the project and its author in the README and in the packages that follow its model.
Research the upstream code cites (Schellart 2010 for plate velocity; White, McKenzie and O'Nions 1992 for ocean crust thickness) is cited where the constants are used.
