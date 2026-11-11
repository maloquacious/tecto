# Project Workflow

Guidance for agents when working in this repository.

This repository is `tecto`: a Go package, with a small command around it, that simulates plate tectonics on a sphere and produces elevation data for the Hāmwold map generator.
The plan, its scope, and its milestones are in [PORT_PLAN.md](PORT_PLAN.md). Read it before writing code.

## Priorities

Fix bugs before adding features.
When proposing what to work on next, first check for open bugs (`gh issue list --label bug --state open`).
If any are open, propose working on them instead of new features or documentation.
An explicit request from the user takes precedence; do the requested work.

Otherwise, work on the earliest unfinished milestone in PORT_PLAN.md.
Do not start a milestone while an earlier one's "done when" is unmet.

## Build and test

Prefer `go run` to `go build`: `go run ./cmd/tecto ...`.
Put any binary you do build, and every other scratch artifact, under `tmp/`.
That includes generated worlds, debug PNGs, and any copy of the tectonics.js source.
Never write build artifacts or output files to the repository root.
`tmp/` is git-ignored, and `go.mod` has an `ignore` directive that keeps `./...` out of it.

The checks, from the repository root:

```sh
go vet ./...
go test ./...
```

Before committing, both must pass if the commit changes Go code, `go.mod`, or `go.sum`.
Documentation-only changes need no checks.

## Verification

The model is verified in Go, by the checks listed under "Verification" in PORT_PLAN.md.

- Every algorithm lands with the tests that define what correct means for it: an analytic case, an invariant, or a conservation budget.
- Do not test against the JavaScript, and do not add a harness that runs it.
  Matching tectonics.js output is not a goal.
- A test that needs a grid uses the smallest detail that shows the behavior, so `go test ./...` stays fast.
  Long runs belong behind `testing.Short`.
- A run must be repeatable: the same seed and flags give the same file, byte for byte.
  Any change that alters the output for a given seed says so in its commit message.

## Versioning

- Semantic versioning.
  The current version lives in `version.go` as a `semver.Version` literal (`Major`, `Minor`, `Patch`, `PreRelease`), from `github.com/maloquacious/semver`.
- **Every commit that changes Go code (including `go.mod` and `go.sum`) must bump the version in `version.go`.**
  Documentation-only commits are encouraged to bump it but are not required to.
- Choose the bump by the change: patch for fixes, minor for new features or any breaking change while below v1.0.0.
  A change to the root package's exported names, to the output file's format, or to the output for a given seed is a breaking change.
- Go takes a module's version from its git tags and never reads `version.go`, so the two must be kept in step by hand.

### Through step 4: alpha, untagged

Steps are the sub-issues of [the port issue](https://github.com/maloquacious/tecto/issues/1); step 4 is the decision point.

- Keep `PreRelease: "alpha"`.
- Do not create git tags.
- Hāmwold depends on this module through a `replace` directive in its own `go.mod` that points at a local checkout.
  Do not publish a version, and do not point Hāmwold at a pseudo-version, to work around that.

### After step 4: stable, tagged

If step 4 decides to go on, the root package's exported names are taken as stable.

- Remove `PreRelease` from `version.go` in the first commit after the decision.
- Tag a commit when Hāmwold needs to depend on it.
  The tag is `v` followed by the version in `version.go` at that commit, such as `v0.5.0`; check that they match before pushing the tag.
- Never move or delete a pushed tag. The Go proxy keeps the first copy it sees, so a bad tag is fixed with a new version.
- Hāmwold then requires the tagged version, and its `replace` directive is removed.

If step 4 decides to stop, the project stays alpha and untagged.

## Git workflow

- Commit directly to `main`.
  No feature branches or pull requests; history is the recovery mechanism.
  This may change after step 4.
- Agents are authorized to commit and push each task to `origin` as it is completed, provided all checks pass and Go code changes include a version bump.
- Agents are authorized to create GitHub labels as needed.

## CI

Do not create GitHub Actions workflows or other GitHub CI actions.
Run the checks above locally instead.
