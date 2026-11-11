# Tectonics.js → Go CLI port plan

## Recommendation

Build **`tecto`**, a native, CPU-only Go command-line simulator.
Port the numerical engine in dependency order, using the original JavaScript as a differential-test reference.
Deliver tectonic evolution and reproducible checkpoints first; add the coupled climate, biosphere, and orbital models afterward.
Do not translate the browser application file for file.

Assumptions: the target is a standalone Go binary, not a terminal globe viewer or a wrapper around a JavaScript runtime.
The eventual scope is the source project's implemented simulation behavior, not just a terrain generator.
The first useful release deliberately covers a smaller tectonics-only slice.

The local workspace was empty at inspection.
This is a plan, not an implementation or a newly initialized repository.

## Inspected baseline and feasibility

Inspected upstream `master` at [e43927ce88e766b63e35e896f36bfb882a4c8de4](https://github.com/davidson16807/tectonics.js/commit/e43927ce88e766b63e35e896f36bfb882a4c8de4), whose latest commit is dated August 22, 2020.
Pin this revision rather than allowing the reference implementation to move during the port.

Important source boundaries:

| Source | Responsibility | Go treatment |
| --- | --- | --- |
| [`precompiled/rasters/`](https://github.com/davidson16807/tectonics.js/tree/e43927ce88e766b63e35e896f36bfb882a4c8de4/precompiled/rasters) | Grid topology, nearest-cell lookup, matrices, typed fields, diffusion, morphology, transport, scratch buffers | Port the subset reachable from simulation and supported exports; expand as needed |
| [`noncompiled/academics/`](https://github.com/davidson16807/tectonics.js/tree/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/academics) | Physical and geometric calculations | Ordinary Go functions, separated from state orchestration |
| [`noncompiled/generators/CrustGenerator.js`](https://github.com/davidson16807/tectonics.js/blob/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/generators/CrustGenerator.js) | Height distributions and rock-column initialization | Native seeded generation |
| [`noncompiled/models/lithosphere/`](https://github.com/davidson16807/tectonics.js/tree/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/models/lithosphere) | Crust, plates, motion, erosion, reactions, rifting, subduction, supercontinent cycle | First simulation milestone |
| [`noncompiled/models/World.js`](https://github.com/davidson16807/tectonics.js/blob/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/models/World.js) and submodels | Coupled lithosphere, hydrosphere, atmosphere, biosphere | Typed world state and explicit initialization/update order |
| [`noncompiled/models/universe/`](https://github.com/davidson16807/tectonics.js/tree/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/models/universe) | Stars, orbital/spin cycles, insolation | Later full-model milestone |
| [`noncompiled/models/Simulation.js`](https://github.com/davidson16807/tectonics.js/blob/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/models/Simulation.js) | Wall-clock-driven stepping, RNG, simulation metadata | Replace timing with explicit simulated-time stepping |
| [`noncompiled/file-io/`](https://github.com/davidson16807/tectonics.js/tree/e43927ce88e766b63e35e896f36bfb882a4c8de4/noncompiled/file-io) | JSON saves, CSV, image import | Native checkpoints and CSV first; legacy import and image handling later |
| Views, controls, Vue, jQuery, WebGL shaders | Interactive application and rendering | Do not port; optional static maps are a separate feature |

The model uses CPU-side JavaScript field operations; WebGL is not required for tectonic calculations.
However, Three.js is not entirely presentation-only: it constructs the simulation mesh and a fixed detail-4 coarse mesh inside `FluidMechanics.js`.

**Executed feasibility probe:** loaded the vendored Three.js geometry code, compiled raster bundle, and tectonics dependencies into a Node VM, with `self` aliased to the VM global.
No DOM or WebGL renderer was needed.
On a detail-3 grid, seeds 42 and 43 for terrain ranking and height sampling, and twelve 0.1-Myr steps using the upstream lithosphere-test dependency configuration, the model ended with two plates, zero nonfinite crust entries, and approximately −1.18% drift in the source's average conserved-crust metric.
This is a smoke baseline, not a full upstream test run, browser-world reproduction, performance benchmark, or scientific validation.

Measured mesh sizes from the vendored Three.js implementation:

| Detail | Vertices/cells | Triangle faces |
| --- | --- | --- |
| 0 | 12 | 20 |
| 3 | 642 | 1,280 |
| 4 | 2,562 | 5,120 |
| 5, browser default | 10,242 | 20,480 |
| 6, browser cap | 40,962 | 81,920 |

## Product contract

### First useful release: tectonics core

- Generate a spherical crust from a seed and resolution.
- Initialize and evolve plates, including erosion, weathering, lithification, metamorphosis, accretion, rifting, and subduction.
- Use the native hydrology sea-level calculation for the generated displacement field; do not make the upstream test's artificial deep sea level the production default.
- Advance by a specified timestep and count, independent of machine speed.
- Save and resume complete native state.
- Export per-cell coordinates, elevation, plate assignment, crust age, and selected rock quantities as CSV.
- Report numerical diagnostics and nonfinite values rather than silently producing unusable output.

Temperature and precipitation are not available in tectonics-only mode.
The CLI must not fill those columns with plausible-looking fabricated values.

### Full-model follow-up

Add the source's atmosphere, snow/ocean coverage, vegetation diagnostics, stars, orbital/spin cycles, and insolation.
Support the original six-column world CSV: latitude, longitude, elevation, plate, temperature, precipitation.
Keep clear that several source models are simplified diagnostics rather than full dynamic solvers: hydrosphere change methods do not evolve water transport, biosphere change methods are empty, precipitation is a latitude-based estimate, and the active albedo path uses a constant value.

### Excluded from initial scope

Interactive 3D display, terminal animation, photorealistic rendering, generated names, arbitrary plugins, GPU acceleration, distributed simulation, alternative tectonic physics, and numerical-method improvements.
Heightmap import, static PNG maps, and legacy browser-save import are follow-up features, not prerequisites for the core port.

## Proposed CLI

Illustrative interface; these commands do not exist yet:

```sh
tecto generate --seed 42 --resolution 4 --output world.tecto.json
tecto run --input world.tecto.json --dt-myr 0.1 --steps 1000 --output evolved.tecto.json
tecto inspect --input evolved.tecto.json
tecto export --input evolved.tecto.json --format csv --output cells.csv
```

Keep configuration separate from checkpoints.
Configuration describes desired initial conditions; checkpoints contain the exact state required to continue an existing run.
Add configuration files when the parameter set outgrows practical flags, using a versioned JSON schema and documented units.

- Diagnostics/progress go to stderr; requested machine-readable output goes to stdout or an explicit file.
- Reject invalid resolutions, nonpositive timesteps, negative step counts, malformed state, and incompatible schema versions before advancing.
- Treat `--dt-myr` as a floating-point count of simulated megayears, converted to seconds internally.
  Geological intervals do not fit Go's nanosecond-based `time.Duration`.
- An interrupt should stop at a completed step and preserve a valid checkpoint; use temporary-file replacement for saves.
- Report the seed, mesh size, elapsed simulated time, timestep, model mode, and schema/model version.
- Start with standard-library subcommand parsing.
  Do not add a CLI framework unless the actual interface needs one.

## Go architecture

Use Go 1.26 as the initial toolchain baseline, matching the installed 1.26.4 toolchain.
Confirm the minimum supported version before implementation and apply the Modern Go Guidelines tooling when writing Go.

```text
cmd/tecto/              CLI parsing and process lifecycle
internal/grid/         Icosahedral mesh, topology, spatial lookup
internal/field/        Scalar/vector operations, morphology, transport
internal/physics/      Geometry and numerical physical calculations
internal/model/        Crust, plates, world, stepping; later coupled models
internal/generate/     Seeded initialization and source-compatible RNG
internal/state/        Versioned checkpoints and CSV exports
internal/reference/    Development-only JS harness and fixture tooling
testdata/              Small, attributed differential fixtures
```

These are ownership boundaries, not a requirement to create every directory on day one.
Keep model subcomponents together until splitting them removes real complexity.
Use concrete types, not an interface hierarchy matching the JavaScript constructors.
Runtime Go binaries must not depend on Node, npm, a browser, C preprocessing, GLSL translation, or Three.js.

Simulation step order initially follows the source: invalidate derived fields, calculate deltas, then apply them.
Within lithosphere application, preserve integration → plate movement → supercontinent-cycle update → merge → rifting → subduction, including the source's delayed accretion contribution.
Eagerly recomputing everything or rearranging these stages can change results.

## Compatibility decisions to settle early

### 1. Numeric precision

JavaScript scalar arithmetic is double precision, while writes to `Float32Array` round to float32.
A literal Go translation using float32 for all arithmetic will not match that behavior.

Store raster and crust fields as float32 initially, retain double-precision intermediate arithmetic where the JavaScript does, and reproduce material rounding boundaries.
Preserve the source's eight crust channels and contiguous channel layout; vector fields use separate component arrays.
Document which operations allow input/output aliasing, since in-place operations are common upstream.
Do not optimize allocations or parallelize reductions before parity is characterized.

### 2. Mesh identity and traversal order

Matching the cell count is insufficient.
Vertex numbering, face order, ascending neighbor traversal, coordinate convention (Y-up and longitude from `atan2(-z, x)`), nearest-cell tie breaking, matrix layout, and segmentation ties affect simulation output.

Use exported JavaScript meshes as fixtures before implementing native mesh generation.
Match the vendored Three.js subdivision, normalization, and vertex-merging behavior rather than substituting an arbitrary icosphere implementation.
Preserve the fixed detail-4 coarse grid in the fluid-pressure calculation.
Use deterministically ordered slices rather than Go map iteration in numerical loops.

Keep cell IDs wide enough in Go to avoid accidental overflow, but restrict compatibility mode to source-supported resolutions 0–6.
The source uses uint16 cell IDs and uint8 plate IDs, with sentinel values including 65535 and 255.
Supporting larger meshes or more plates is a separate change, not a free consequence of choosing wider Go integers.

### 3. Randomness and seed semantics

The bundled RNG is a Mersenne Twister implementation with its own initialization, uniform and normal sampling, and a cached second normal sample.
Go's default RNG will not generate matching planets.

Port only the RNG operations the model needs, with fixtures for parsed seeds, integer streams, uniform samples, normal samples, and resumed streams.
If browser seed equivalence is promised, include the random draws consumed by generated names before terrain generation, or begin parity comparisons from captured post-name-generation RNG state.
Define a documented native seed contract independently rather than accidentally promising browser-equivalent output.

### 4. Explicit simulation time versus display policy

The browser's `Simulation.update()` uses `performance.now()` and skips updates slower than 5 fps.
Lithosphere evolution also returns early below a timestep threshold derived from what a viewer could perceive, and universe sampling contains frame-rate-based thresholds.
These are application policies mixed into the model, not CLI requirements.

First reproduce the pinned reference behavior in the harness at named timesteps.
Then replace wall-clock stepping with explicit simulated seconds and separate each display-derived policy from the physical model.
Test both sides of affected thresholds.
Treat any changed sampling or update policy as a documented semantic change with its own comparison results, not as a silent translation fix.

### 5. Derived-state caches and checkpoints

`Memo` marks a field clean before evaluating it to break circular dependencies.
Its evaluation order is observable in the coupled climate model.
Preserve this behavior first; design a cleaner dependency schedule only after equivalence is demonstrated.

Native checkpoints must contain all persistent state needed for deterministic continuation: crust channels, masks, plate transforms, cycle ages, elapsed time, RNG state including cached distribution samples, parameters, and later persistent climate/orbit state.
Audit which cached values are safely reconstructible.
The source serializer stores `buffer:`-prefixed Base64 array buffers, while several model `getParameters()` methods omit dynamic or cached state.
Do not assume an upstream JSON save is a complete deterministic checkpoint.
In particular, audit supercontinent-cycle restoration and atmospheric temperature persistence before offering import compatibility.

Use an explicit byte order for binary payloads and validate lengths, IDs, and finite values.
A versioned JSON envelope with Base64-encoded typed buffers is a reasonable first format; compressed containers can wait for measured need.

## Implementation sequence and exit gates

| Phase | Work | Exit gate | Rough effort |
| --- | --- | --- | --- |
| 0. Freeze and characterize | Pin source; inventory reachable functions, units, dependency notices, default parameters; establish headless JS harness; capture deterministic meshes and states | Repeatable reference outputs, named compatibility boundaries, and an inventory of preexisting failures | 3–5 days |
| 1. Mesh and field foundation | Go module/CLI skeleton; vectors/matrices; imported then native meshes; adjacency/spatial lookup; reachable raster operations and scratch ownership | Exact topology/ID fixtures; asymmetric operator cases; nearest-cell ties; aliasing and float-rounding checks | 1–2 weeks |
| 2. Generation and observability | Compatible RNG; spherical terrain ranking; hypsography; rock-column construction; isostasy; sea-level solve; initial CSV/state inspection | Same captured initial state within specified field tolerances; meaningful generated-world CSV | About 1 week |
| 3. Tectonic evolution | Plate segmentation/rotation/remapping; crust reactions and erosion; merge/rifting/subduction/accretion; cycle timing; explicit stepping | Stage-level and multi-step comparisons, no unexpected nonfinite fields, conservation and topology diagnostics | 2–3 weeks |
| 4. Reproducible core release | Complete native checkpoints; run/resume/inspect/export; validation; interrupts; small cross-platform builds | Continuous and split/save/load runs produce identical native results; CLI smoke tests pass; core release is useful without JS | About 1 week |
| 5. Coupled world parity | Orbital/spin cycles, star/insolation calculations, atmosphere, snow, biosphere diagnostics, cache/order fidelity; richer checkpoint state | Small/large timestep branches and coupled-field outputs compared against reference | 2–3 weeks |
| 6. Hardening | Profile representative runs; optimize measured bottlenecks; documentation, attribution, cross-platform CI; optional legacy import | Published resource envelope, reproducible builds, and documented compatibility limits | About 1 week |

**Planning range:** approximately 8–12 engineer-weeks for the full-model CLI, with a useful tectonics-only release around weeks 5–8.
These are provisional estimates for one experienced engineer, not measured implementation timings.
Re-estimate after phase 0; source defects, exact browser-seed equivalence, legacy-save support, and higher resolutions can materially increase effort.

Do not begin climate integration while basic topology, initial crust, or one-step tectonics parity remains unexplained.
Do not make the optional image/rendering features part of this critical path.

## Validation strategy

Use three complementary forms of evidence:

1. **Differential fixtures:** compare Go with the pinned JavaScript on identical imported state, grid, parameters, RNG state, and timestep.
   Compare initialization and individual step stages before long trajectories.
   Discrete IDs and topology should be exact where the input calculations are equivalent; field tolerances must be justified per quantity using absolute and relative error, not one permissive global epsilon.
   If float differences change segmentation or nearest-cell decisions, diagnose the boundary rather than simply loosening an ID assertion.
2. **Independent invariants:** closed-sphere topology, reciprocal adjacency, valid ID ranges, bounded masks, finite state, nonnegative rock mass, valid rotations, age progression, and conserved-material budgets.
   Rifting/subduction change some material inventories, so test the source's conserved continental channels and explicit source/sink accounting rather than asserting that all crust mass is constant.
   The existing upstream tests allow roughly 1% delta conservation error and 3% multi-step conserved-crust drift; use those as historical context, not proof of scientific accuracy or universal release limits.
3. **End-to-end CLI behavior:** seed reproducibility, malformed-input rejection, stable CSV fields and units, interrupt-safe saves, and uninterrupted versus checkpoint/resume equivalence.
   Require exact repeatability on a defined native build/platform first; characterize cross-platform floating-point differences before making stronger promises.

Use asymmetric vectors/matrices, boundary cells, tied distances, multiple seeds, resolution changes, timestep boundaries, first-step initialization, and supercontinent-cycle rollover.
Test normal-sample cache restoration explicitly.
Run the original QUnit suite through a compatible harness during phase 0; the executed smoke probe did not run that suite.
Translate useful public behavioral tests, not every helper or incidental implementation assertion.
Keep Node-based comparisons development-only, with small checked-in fixtures so normal `go test ./...` remains self-contained.

Once Go code exists, run targeted tests during each stage, then `go test ./...`, `go vet ./...`, and CLI smoke runs.
Use the race detector if concurrency is introduced; keep the initial solver serial.
Benchmark and profile detail 3, 5, and 6, reporting step time, initialization time, peak memory, and allocations.
Per-plate full-grid crust and scratch storage are likely memory pressure points, but measure before changing layouts.
Do not promise a speedup from language choice alone.

## Licensing and provenance

The repository's [license](https://github.com/davidson16807/tectonics.js/blob/e43927ce88e766b63e35e896f36bfb882a4c8de4/LICENSE.txt) is **CC BY 4.0**, not MIT.
Preserve attribution, license/disclaimer notices, source links, and an indication of modifications.
Audit copied algorithms, vendored Three.js/random code, fixtures, and any submodules individually before deciding distribution notices and a license for new contributions; do not assume one root license covers every dependency.

The source build preprocesses raster/shader files and includes separate GLSL academics, translation-tool, and research submodules.
Port from canonical numerical source, use committed compiled bundles only as reference inputs, and fetch submodules only if reachable functionality requires them.
Shaders, the GLSL build pipeline, and research documents need not become runtime dependencies.

## First concrete work item

Implement **phase 0 only**: pin the upstream source and required notices, create the headless reference harness, record deterministic detail-3/4 fixtures and stage-level outputs, inventory units and persistent state, and turn unexplained behavior into explicit compatibility decisions.
The first native vertical slice should then be **seed → crust → sea level/elevation → CSV**, followed by **one tectonic step → checkpoint → deterministic resume**.
