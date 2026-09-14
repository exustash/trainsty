#!/usr/bin/env bash
#
# Build the release archives for a tagged trainsty version.
#
# This is OD-4's channel 1: a downloadable binary, which is the only channel that
# satisfies the constitution's Principle I on its own — `go install` requires a Go
# toolchain and a Homebrew tap requires a package manager, and Principle I says
# installation MUST never REQUIRE either. They are allowed alongside this; they
# cannot replace it.
#
# The build is CGO_ENABLED=0 so each binary is genuinely static and runs on a
# machine with no toolchain, no libc surprises and no runtime. Verified for all
# four targets below.
#
# The version is NOT passed in: `trainsty version` reads it from the build via
# runtime/debug.ReadBuildInfo, so the tag is the single source of truth and this
# script has nothing to keep in sync. Stripping with -s -w does not remove it.
#
# NB no comment line in this file may BEGIN with the word "shellcheck" — a
# leading "# shellcheck ..." is parsed as a directive and errors with SC1073.
#
# Usage:
#   scripts/release.sh            build archives for the current HEAD's tag
#   scripts/release.sh v1.0.0     build archives for an explicit tag
#
# Output lands in dist/ and is git-ignored. Publishing is a separate, deliberate
# step (RULES.md §1.3) — this script never pushes and never calls gh.

set -euo pipefail

cd "$(dirname "$0")/.."

readonly DIST="dist"
readonly TARGETS=(
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "linux/arm64"
)

main() {
    local tag="${1:-}"

    if [[ -z "$tag" ]]; then
        tag="$(git describe --tags --exact-match 2>/dev/null || true)"
    fi
    if [[ -z "$tag" ]]; then
        die "no tag on HEAD and none given. Tag first, or pass one: scripts/release.sh v1.0.0"
    fi
    if ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
        die "tag $tag does not exist"
    fi

    # A dirty tree would produce binaries whose recorded revision does not describe
    # them, and `trainsty version` would say so with a -dirty marker. Refuse rather
    # than ship that.
    if [[ -n "$(git status --porcelain)" ]]; then
        die "the working tree is dirty — commit or stash before building a release"
    fi

    rm -rf "$DIST"
    mkdir -p "$DIST"

    echo "==> building $tag"
    local target os arch bin archive
    for target in "${TARGETS[@]}"; do
        os="${target%/*}"
        arch="${target#*/}"
        bin="$DIST/trainsty"
        archive="trainsty_${tag}_${os}_${arch}.tar.gz"

        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
            go build -trimpath -ldflags="-s -w" -o "$bin" .

        # Archived from inside dist/ so the tarball holds a bare `trainsty`, not a
        # nested path the developer has to dig through.
        tar -czf "$DIST/$archive" -C "$DIST" trainsty
        rm -f "$bin"
        printf '    %-34s %s\n' "$archive" "$(du -h "$DIST/$archive" | cut -f1)"
    done

    # Checksums matter more here than for most tools: this binary terminates process
    # groups, so a developer downloading it over curl should be able to verify it.
    ( cd "$DIST" && shasum -a 256 ./*.tar.gz > SHA256SUMS )
    echo "==> $DIST/SHA256SUMS"
    cat "$DIST/SHA256SUMS"

    echo
    echo "Built, not published. To publish:"
    echo "  gh release create $tag $DIST/*.tar.gz $DIST/SHA256SUMS --title $tag --notes-file <notes>"
}

die() {
    echo "release: $*" >&2
    exit 1
}

main "$@"
