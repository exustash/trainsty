module github.com/exustash/trainsty

// 1.20 is the floor, and it is load-bearing: http.NewResponseController arrived
// in 1.20 and is what lets /register hold a 30-minute stream open without
// disabling write timeouts server-wide (specs/001-serialize-e2e-runs/research.md
// → R1). embed (1.16) and everything else needed predates it. A low floor is
// deliberate: the product's value is installing anywhere with no setup.
go 1.20

// No require block, ever. The standard library is the whole toolbox
// (.specify/memory/constitution.md → Principle I). Adding a dependency needs a
// Complexity Tracking entry in the plan BEFORE the import, and
// scripts/ci-local.sh fails the build if this file grows one.
