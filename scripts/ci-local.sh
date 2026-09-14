#!/usr/bin/env bash
#
# Local CI runner for trainsty — the merge gate on this workstation.
#
# There is no GitHub Actions workflow and no required check on `main`
# (RULES.md §7.3), so this script IS the gate rather than a mirror of one. When a
# workflow exists, keep the job list here in lockstep with it and say so in both.
#
# Go jobs (fmt / vet / build / test / cover / deps / boundary) use the locally
# installed toolchain. The two scanners come from digest-pinned upstream images
# via podman (or docker) — nothing to install, images cached once. govulncheck and
# markdownlint come from the local PATH if present; neither is a project
# dependency, and both report themselves UNRUN rather than failing the run when
# absent.
#
# Blocking jobs fail the run on error: fmt, vet, build, test, cover, deps,
# boundary, gitleaks, the shell lint, and e2e.
# Report-only jobs print findings but never fail the run: govulncheck, vocab,
# docs-lint.
#
# NB no comment line in this file may BEGIN with the word "shellcheck" — a
# leading "# shellcheck ..." is parsed as a directive and errors with SC1073.
# This gate caught exactly that in its own header on its first run.
#
# Usage:
#   scripts/ci-local.sh                auto-detect changed scope vs BASE_REF, run matching jobs
#   scripts/ci-local.sh --all          every job except e2e
#   scripts/ci-local.sh --everything   --all PLUS the acceptance suite (the whole surface)
#   scripts/ci-local.sh --quick        blocking Go jobs only — what the pre-push hook runs
#   scripts/ci-local.sh --go           fmt / vet / build / test / cover
#   scripts/ci-local.sh --test         only go test -race
#   scripts/ci-local.sh --cover        only the scheduler coverage floor
#   scripts/ci-local.sh --deps         only the zero-dependency check
#   scripts/ci-local.sh --boundary     only the os/exec import boundary check
#   scripts/ci-local.sh --scanners     only gitleaks + shellcheck + govulncheck
#   scripts/ci-local.sh --e2e          only the acceptance suite (binds port 45678)
#   scripts/ci-local.sh --lint-docs    also markdownlint the changed docs (report-only)
#   scripts/ci-local.sh --fast         skip report-only jobs
#   scripts/ci-local.sh --no-scanners  skip gitleaks / shellcheck / govulncheck
#   scripts/ci-local.sh --sequential   run jobs one at a time (default: fan out by CPU count)
#   scripts/ci-local.sh --list         print the job plan and exit
#
# Two things about this repository shape the script, and both are deliberate:
#
#   1. `--quick` is a WEAKER gate, not just a faster one. It drops the acceptance
#      suite, which is the only layer that exercises the built binary, signal
#      handling, and the lock surviving a killed holder. A green push is not a
#      green `--everything`.
#   2. The acceptance suite binds port 45678 — the same machine-wide port a real
#      daemon holds. It is therefore NEVER auto-selected, and it reports itself
#      unrun rather than failing when the port is busy. This is trainsty's own
#      contention problem applied to trainsty's own test suite, which is worth
#      noticing rather than working around.
#
# Jobs run concurrently by default: independent jobs fan out bounded by CPU
# count, so wall-clock approaches the SLOWEST job rather than the SUM. Each job's
# output is captured and printed grouped after it finishes, so parallel output
# does not interleave. Blocking vs report-only semantics are identical either way.
# Use --sequential (or CI_LOCAL_SERIAL=1) to debug.
#
# Env:
#   BASE_REF          diff base for change detection (default: origin/main)
#   RUNTIME           container runtime (default: auto-detect podman, then docker)
#   CI_LOCAL_JOBS     max concurrent jobs (default: CPU count)
#   CI_LOCAL_SERIAL   non-empty → force sequential (same as --sequential)
#   CI_LOCAL_STRICT_UNRUN
#                     non-empty → a BLOCKING job that could not run FAILS the run.
#                     Set this for a merge gate; the push hook leaves it unset, so
#                     a machine with no container runtime can still push.
#
# =====HELP-END=====
#
# Every job_* below is invoked INDIRECTLY through the ${JOB_FUNC[@]} registry.
# The linter cannot trace that and flags each one as never invoked (SC2329), so it
# is disabled file-wide by the directive below, which must precede the first
# command. (And note the wording of this paragraph: see the NB in the header —
# a comment line starting with the tool's name becomes a directive.)
# shellcheck disable=SC2329
set -uo pipefail

# ---- image digests ----------------------------------------------------------
# Reused verbatim from the sibling repositories' ci-local.sh, where they are
# already verified. Both are multi-arch INDEX digests, not one architecture's
# manifest — a single-arch pin cannot exec on the other host type and fails a
# blocking gate for a reason unrelated to what it is checking.
GITLEAKS_IMAGE="ghcr.io/gitleaks/gitleaks@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f"
SHELLCHECK_IMAGE="docker.io/koalaman/shellcheck@sha256:bb596a0d169b85ddd81d8b6d3a2ff6d5baf5fca10b97f575ebc647c3dff62b3d"

# The port the acceptance suite and a real daemon both need. Hardcoded here for
# the same reason it is hardcoded in the product (ADR-011).
TRAINSTY_PORT=45678

# Resolved without `dirname`, so the script still knows where it is when run with
# no PATH. A bare name (scripts/ on PATH) has no slash to strip.
_src="${BASH_SOURCE[0]}"
[ "${_src#*/}" = "$_src" ] && _src="./$_src"
REPO_ROOT="$(cd "${_src%/*}/.." && pwd)"
BASE_REF="${BASE_REF:-origin/main}"

# ---- pretty logging ---------------------------------------------------------
if [ -t 1 ]; then
  BOLD=$'\033[1m'; RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BLUE=$'\033[34m'; DIM=$'\033[2m'; RESET=$'\033[0m'
else
  BOLD=""; RED=""; GREEN=""; YELLOW=""; BLUE=""; DIM=""; RESET=""
fi
step()  { printf "\n%s==>%s %s%s%s\n" "$BLUE" "$RESET" "$BOLD" "$1" "$RESET"; }
info()  { printf "%s   %s%s\n" "$DIM" "$1" "$RESET"; }
pass()  { printf "%s ✓ %s%s\n" "$GREEN" "$1" "$RESET"; }
warn()  { printf "%s ! %s%s\n" "$YELLOW" "$1" "$RESET"; }
fail()  { printf "%s ✗ %s%s\n" "$RED" "$1" "$RESET"; }

# ---- result tracking --------------------------------------------------------
declare -a BLOCKING_FAILED=()   # blocking job failed → non-zero run exit
declare -a REPORT_FINDINGS=()   # report-only job surfaced findings → run exit unaffected
declare -a JOBS_RUN=()
declare -a JOBS_UNRUN=()        # selected but could not run (no toolchain/runtime/source)
declare -a BLOCKING_UNRUN=()    # the blocking subset → fails a STRICT run

# A job that discovers at RUNTIME it cannot run returns this. In-band with a real
# exit status, so a tool that genuinely exited 97 would be misreported as unrun;
# nothing here exits 97.
#
# A blocking job returning this leaves the run exiting 0 by DEFAULT — the push
# hook must stay usable on a machine with no podman and, right now, on a
# repository with no Go code at all. Under CI_LOCAL_STRICT_UNRUN it exits 1: a
# merge gate is the one caller for which "the secret scan never ran" must not
# read as a pass.
EXIT_UNRUN=97
STRICT_UNRUN="${CI_LOCAL_STRICT_UNRUN:-}"

# ---- toolchain pre-flight ---------------------------------------------------
# Reports drift rather than switching: Go has no per-user version manager here,
# and a gate that refuses to run is worse than one running slightly off-pin.
check_go_pin() {
  local want have
  command -v go >/dev/null 2>&1 || { warn "go is not installed — every Go job will report itself unrun"; return 0; }
  have="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  want="$(awk '/^go [0-9]/ {print $2; exit}' "$REPO_ROOT/go.mod" 2>/dev/null || true)"
  if [ -z "$want" ]; then info "go $have — no go.mod floor to check yet"; return 0; fi
  # String compare is not enough (1.9 vs 1.20), so compare as version fields.
  if [ "$(printf '%s\n%s\n' "$want" "$have" | sort -V | head -1)" = "$want" ]; then
    info "go $have — satisfies the go.mod floor ($want)"
  else
    warn "go $have is BELOW the go.mod floor ($want) — build failures here are the toolchain, not the code"
  fi
}

have_go_source() { [ -f "$REPO_ROOT/go.mod" ] && [ -n "$(find "$REPO_ROOT" -name '*.go' -not -path '*/.*' -print -quit 2>/dev/null)" ]; }

# The single reason every Go job is unrun today. Stated once so ten jobs cannot
# drift in how they explain it.
require_go_source() {
  command -v go >/dev/null 2>&1 || { warn "no go toolchain — skipping $1"; return 1; }
  if [ ! -f "$REPO_ROOT/go.mod" ]; then
    warn "no go.mod — skipping $1 (tasks.md T001 creates it)"; return 1
  fi
  if ! have_go_source; then
    warn "no .go files yet — skipping $1 (tasks.md T002 onward add them)"; return 1
  fi
}

# ---- container runtime detection --------------------------------------------
detect_runtime() {
  if [ -n "${RUNTIME:-}" ]; then echo "$RUNTIME"; return; fi
  command -v podman >/dev/null 2>&1 && { echo podman; return; }
  command -v docker >/dev/null 2>&1 && { echo docker; return; }
  echo ""
}
CONTAINER_RUNTIME="$(detect_runtime)"
require_runtime() {
  [ -n "$CONTAINER_RUNTIME" ] || { warn "no container runtime (podman/docker) — skipping $1"; return 1; }
}

# ---- job registry + dispatch ------------------------------------------------
declare -a JOB_KIND=() JOB_LABEL=() JOB_FUNC=()
add_job() { JOB_KIND+=("$1"); JOB_LABEL+=("$2"); JOB_FUNC+=("$3"); }

record_result() { # kind label exitcode
  local kind="$1" label="$2" ec="$3"
  if [ "$ec" -eq "$EXIT_UNRUN" ]; then
    warn "$label did NOT run"; JOBS_UNRUN+=("$label")
    [ "$kind" = blocking ] && BLOCKING_UNRUN+=("$label")
    return 0
  fi
  JOBS_RUN+=("$label")
  if [ "$ec" -eq 0 ]; then
    pass "$label"
  elif [ "$kind" = blocking ]; then
    fail "$label FAILED"; BLOCKING_FAILED+=("$label")
  else
    warn "$label surfaced findings (report-only)"; REPORT_FINDINGS+=("$label")
  fi
}

dispatch_serial() {
  local i
  for i in "${!JOB_FUNC[@]}"; do
    step "${JOB_LABEL[$i]}  ${DIM}(${JOB_KIND[$i]})${RESET}"
    "${JOB_FUNC[$i]}"; record_result "${JOB_KIND[$i]}" "${JOB_LABEL[$i]}" "$?"
  done
}

dispatch_parallel() { # max_concurrent
  local max="$1" i ec logdir
  logdir="$(mktemp -d "${TMPDIR:-/tmp}/trainsty-ci.XXXXXX")" || { warn "mktemp failed — running sequentially"; dispatch_serial; return; }
  info "running ${#JOB_FUNC[@]} jobs, up to $max concurrent (--sequential to disable)"
  for i in "${!JOB_FUNC[@]}"; do
    ( "${JOB_FUNC[$i]}" >"$logdir/$i.log" 2>&1; echo "$?" >"$logdir/$i.ec" ) &
    while [ "$(jobs -rp | wc -l)" -ge "$max" ]; do wait -n 2>/dev/null || break; done
  done
  wait
  # Aggregate in registry order so parallel output reads exactly like serial.
  for i in "${!JOB_FUNC[@]}"; do
    ec="$(cat "$logdir/$i.ec" 2>/dev/null || echo 1)"   # missing ec → failure (fail closed)
    step "${JOB_LABEL[$i]}  ${DIM}(${JOB_KIND[$i]})${RESET}"
    cat "$logdir/$i.log" 2>/dev/null || true
    record_result "${JOB_KIND[$i]}" "${JOB_LABEL[$i]}" "$ec"
  done
  rm -rf "$logdir"
}

# ---- selection state --------------------------------------------------------
# Flags are ADDITIVE (union), not last-wins: `--test --deps` runs both.
DO_FMT=false; DO_VET=false; DO_BUILD=false; DO_TEST=false; DO_COVER=false
DO_DEPS=false; DO_BOUNDARY=false; DO_GITLEAKS=false; DO_SHELLCHECK=false
DO_VULN=false; DO_VOCAB=false; DO_DOCSLINT=false; DO_E2E=false
SELECTED=false; SKIP_REPORT=false; SKIP_SCANNERS=false; LIST_ONLY=false
LINT_DOCS=false; SEQUENTIAL=false

select_go()       { DO_FMT=true; DO_VET=true; DO_BUILD=true; DO_TEST=true; DO_COVER=true; }
select_scanners() { DO_GITLEAKS=true; DO_SHELLCHECK=true; DO_VULN=true; }
select_all()      { select_go; select_scanners; DO_DEPS=true; DO_BOUNDARY=true; DO_VOCAB=true; }

for arg in "$@"; do
  case "$arg" in
    --all)          select_all;                        SELECTED=true ;;
    --everything)   select_all; DO_E2E=true;           SELECTED=true ;;
    --quick)        select_go; DO_DEPS=true; DO_BOUNDARY=true; SKIP_REPORT=true; SELECTED=true ;;
    --go)           select_go;                         SELECTED=true ;;
    --fmt)          DO_FMT=true;                       SELECTED=true ;;
    --vet)          DO_VET=true;                       SELECTED=true ;;
    --test)         DO_TEST=true;                      SELECTED=true ;;
    --cover)        DO_COVER=true;                     SELECTED=true ;;
    --deps)         DO_DEPS=true;                      SELECTED=true ;;
    --boundary)     DO_BOUNDARY=true;                  SELECTED=true ;;
    --scanners)     select_scanners;                   SELECTED=true ;;
    --shellcheck)   DO_SHELLCHECK=true;                SELECTED=true ;;
    --e2e)          DO_E2E=true;                       SELECTED=true ;;
    --lint-docs)    LINT_DOCS=true ;;
    --fast)         SKIP_REPORT=true ;;
    --no-scanners)  SKIP_SCANNERS=true ;;
    --sequential)   SEQUENTIAL=true ;;
    --list)         LIST_ONLY=true ;;
    -h|--help)      awk 'NR>1 && /^# =====HELP-END/ {exit} NR>1 && /^#/ {sub(/^# ?/,""); print}' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) fail "unknown flag: $arg"; exit 2 ;;
  esac
done
$SELECTED && MODE_LABEL="selected" || MODE_LABEL="auto"
[ -n "${CI_LOCAL_SERIAL:-}" ] && SEQUENTIAL=true

# ---- change detection -------------------------------------------------------
# Committed diff vs BASE_REF combined with the working tree (staged, unstaged and
# untracked), so an uncommitted change is classified the way a PR would be.
changed_files() {
  { git -C "$REPO_ROOT" diff --name-only "$BASE_REF"...HEAD 2>/dev/null
    git -C "$REPO_ROOT" status --porcelain 2>/dev/null | sed 's/^...//'
  } | sort -u
}
GO_RE='\.go$|^go\.(mod|sum)$'
SHELL_RE='^scripts/[^/]+\.sh$|^\.githooks/'
CH_GO=false; CH_MOD=false; CH_SHELL=false; CH_DOCS=""
classify_changes() {
  local files; files="$(changed_files)"
  echo "$files" | grep -qE "$GO_RE"                 && CH_GO=true
  echo "$files" | grep -qE '^go\.(mod|sum)$'        && CH_MOD=true
  echo "$files" | grep -qE "$SHELL_RE"              && CH_SHELL=true
  CH_DOCS="$(echo "$files" | grep -E '\.md$' || true)"
  return 0
}

plan_jobs() {
  if ! $SELECTED; then
    classify_changes
    # Go source changed → the whole Go set. The boundary and vocab checks read
    # .go files, so they belong to the same trigger.
    if $CH_GO; then select_go; DO_BOUNDARY=true; DO_VOCAB=true; fi
    # go.mod changed → re-check the zero-dependency promise and stdlib vulns.
    # This is the trigger that matters most: adding a dependency is exactly the
    # change that must not pass unnoticed (constitution, Principle I).
    $CH_MOD    && { DO_DEPS=true; DO_VULN=true; }
    $CH_SHELL  && DO_SHELLCHECK=true
    DO_GITLEAKS=true      # always: a secret can land in any file
    $LINT_DOCS && [ -n "$CH_DOCS" ] && DO_DOCSLINT=true
    # e2e is NEVER auto-selected: it binds the machine-wide port 45678.
  elif $LINT_DOCS; then
    classify_changes; [ -n "$CH_DOCS" ] && DO_DOCSLINT=true
  fi
  if $SKIP_REPORT;   then DO_VULN=false; DO_VOCAB=false; DO_DOCSLINT=false; fi
  if $SKIP_SCANNERS; then DO_GITLEAKS=false; DO_SHELLCHECK=false; DO_VULN=false; fi
  return 0
}

# =========================== the jobs ========================================

# gofmt: formatting is mechanical, so a filename in the output is a failure
# rather than a suggestion. gofmt wins over the house four-space rule — see
# knowledge/conventions/go.md for why that exception exists.
job_fmt() {
  require_go_source "gofmt" || return "$EXIT_UNRUN"
  local out
  out="$(cd "$REPO_ROOT" && gofmt -l . 2>&1)" || return 1
  [ -z "$out" ] && return 0
  fail "these files are not gofmt-clean:"; printf '%s\n' "$out"; return 1
}

job_vet() {
  require_go_source "go vet" || return "$EXIT_UNRUN"
  ( cd "$REPO_ROOT" && info "go vet ./..." && go vet ./... )
}

job_build() {
  require_go_source "go build" || return "$EXIT_UNRUN"
  ( cd "$REPO_ROOT" && info "go build ./..." && go build ./... )
}

# THE gate. Bare `go test` and `go test -race` are different gates here: the
# whole product is one shared structure read by concurrent handlers, so a data
# race is a wrong Grant — two suites running at once, the exact failure trainsty
# exists to prevent. Never relax this to plain `go test`.
job_test() {
  require_go_source "go test -race" || return "$EXIT_UNRUN"
  ( cd "$REPO_ROOT" && info "go test -race ./..." && go test -race ./... )
}

# The constitution's coverage floor, on the only package it names: lock, queue
# and release logic. scheduler/ is pure precisely so this is cheap to reach.
COVER_FLOOR=80
job_cover() {
  require_go_source "coverage floor" || return "$EXIT_UNRUN"
  [ -d "$REPO_ROOT/scheduler" ] || { warn "no scheduler/ package yet — skipping coverage floor"; return "$EXIT_UNRUN"; }
  local out pct
  out="$(cd "$REPO_ROOT" && go test -cover ./scheduler/ 2>&1)" || { printf '%s\n' "$out"; return 1; }
  printf '%s\n' "$out"
  pct="$(printf '%s' "$out" | grep -oE 'coverage: [0-9.]+%' | head -1 | grep -oE '[0-9.]+')"
  [ -n "$pct" ] || { fail "could not read a coverage percentage from go test output"; return 1; }
  # Integer compare after truncating: 79.9% must fail an 80% floor.
  if [ "${pct%%.*}" -lt "$COVER_FLOOR" ]; then
    fail "scheduler/ coverage ${pct}% is below the ${COVER_FLOOR}% floor (.specify/memory/constitution.md)"
    return 1
  fi
  info "scheduler/ coverage ${pct}% ≥ ${COVER_FLOOR}%"
}

# Principle I, made mechanical: go.mod must declare no dependency. This is the
# cheapest gate in the repository and it guards the property that lets the binary
# install anywhere with no toolchain and no lockfile.
job_deps() {
  [ -f "$REPO_ROOT/go.mod" ] || { warn "no go.mod — skipping the zero-dependency check"; return "$EXIT_UNRUN"; }
  local req
  req="$(grep -vE '^\s*(//|$)' "$REPO_ROOT/go.mod" | grep -E '^\s*require|^\s+[a-z0-9.-]+\.[a-z]+/' || true)"
  if [ -n "$req" ]; then
    fail "go.mod declares dependencies — the constitution's Principle I forbids one without a Complexity Tracking entry:"
    printf '%s\n' "$req"
    info "if this is deliberate, justify it in the plan BEFORE merging, then update this gate"
    return 1
  fi
  [ -f "$REPO_ROOT/go.sum" ] && { fail "go.sum exists but go.mod declares nothing — stale file, delete it"; return 1; }
  info "go.mod declares no dependencies"
}

# plan.md's grep-able rule: os/exec may be imported by runner/ and daemonctl/
# only. Both are client-side — `wrap` runs the developer's suite and `start`
# re-execs to detach. An import anywhere else would mean the DAEMON spawns
# something, which is Principle II's one prohibition (ADR-012 explains why the
# two allowed sites are not violations).
job_boundary() {
  require_go_source "os/exec boundary" || return "$EXIT_UNRUN"
  local offenders
  offenders="$(cd "$REPO_ROOT" && grep -rln '"os/exec"' --include='*.go' . 2>/dev/null \
    | sed 's|^\./||' \
    | grep -vE '^(runner|daemonctl)/' \
    | grep -vE '_test\.go$' || true)"
  if [ -n "$offenders" ]; then
    fail "os/exec imported outside runner/ and daemonctl/ — Principle II and ADR-012:"
    printf '%s\n' "$offenders"
    return 1
  fi
  info "os/exec confined to runner/ and daemonctl/"
}

# gitleaks: secret scan across the working tree.
job_gitleaks() {
  require_runtime "gitleaks" || return "$EXIT_UNRUN"
  local args=(detect --source=/repo --no-git --no-banner --redact)
  [ -f "$REPO_ROOT/.gitleaks.toml" ] && args+=(--config=/repo/.gitleaks.toml)
  "$CONTAINER_RUNTIME" run --rm -v "$REPO_ROOT:/repo:ro" "$GITLEAKS_IMAGE" "${args[@]}"
}

# Lint every shell script in the repository — including this one.
# (No comment line below may START with the word shellcheck: a leading
# "# shellcheck ..." is parsed as a directive, SC1073.)
job_shellcheck() {
  require_runtime "shell lint" || return "$EXIT_UNRUN"
  # Collected explicitly rather than passed as globs: an empty .githooks/ makes
  # `.githooks/*` a literal argument, and the linter then fails on a file that
  # does not exist — which reads as a lint failure rather than an empty directory.
  local -a files=()
  local f
  for f in "$REPO_ROOT"/scripts/*.sh "$REPO_ROOT"/.githooks/*; do
    [ -f "$f" ] || continue
    files+=("${f#"$REPO_ROOT"/}")
  done
  [ "${#files[@]}" -gt 0 ] || { warn "no shell scripts to lint"; return "$EXIT_UNRUN"; }
  info "linting: ${files[*]}"
  ( cd "$REPO_ROOT" && "$CONTAINER_RUNTIME" run --rm -v "$REPO_ROOT:/repo:ro" -w /repo \
    "$SHELLCHECK_IMAGE" "${files[@]}" )
}

# govulncheck: stdlib vulnerabilities. Valuable precisely BECAUSE there are no
# dependencies — with an empty go.mod the only vulnerable code that can reach the
# binary is the standard library and the toolchain. A developer tool, not a
# project dependency, so Principle I is unaffected. Report-only: a stdlib
# advisory is not a reason to block a commit, but it is a reason to know.
job_vuln() {
  require_go_source "govulncheck" || return "$EXIT_UNRUN"
  command -v govulncheck >/dev/null 2>&1 || {
    warn "govulncheck not installed — skipping (go install golang.org/x/vuln/cmd/govulncheck@latest)"
    return "$EXIT_UNRUN"
  }
  ( cd "$REPO_ROOT" && govulncheck ./... )
}

# The glossary's ban lists are enforceable or they are decoration. Greps Go
# identifiers for words knowledge/domains/ubiquitous-language.md forbids as names
# for a modelled concept. Report-only, because a banned word can legitimately
# appear in a comment explaining why it is banned.
job_vocab() {
  require_go_source "vocabulary" || return "$EXIT_UNRUN"
  local hits=0 w
  for w in Wrapper CaptureSource Mutex Semaphore Slot Ticket Zombie; do
    local found
    found="$(cd "$REPO_ROOT" && grep -rn --include='*.go' -E "\b(type|func|var|const)\s+[A-Za-z_]*${w}" . 2>/dev/null || true)"
    if [ -n "$found" ]; then
      warn "identifier uses '$w', banned by the glossary as a name for a modelled concept:"
      printf '%s\n' "$found"; hits=$((hits + 1))
    fi
  done
  [ "$hits" -eq 0 ] && { info "no banned vocabulary in Go identifiers"; return 0; }
  return 1
}

job_docslint() {
  command -v markdownlint >/dev/null 2>&1 || {
    warn "markdownlint not installed — skipping docs lint"; return "$EXIT_UNRUN"
  }
  # shellcheck disable=SC2086  # CH_DOCS is a deliberately word-split file list
  ( cd "$REPO_ROOT" && markdownlint $CH_DOCS )
}

# The acceptance suite. Explicit-only, and it needs the machine-wide port free.
job_e2e() {
  require_go_source "acceptance suite" || return "$EXIT_UNRUN"
  if lsof -nP -iTCP:"$TRAINSTY_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    warn "port $TRAINSTY_PORT is in use — the acceptance suite cannot run beside a live daemon"
    info "stop it with 'trainsty stop', or check with 'lsof -nP -iTCP:$TRAINSTY_PORT'"
    return "$EXIT_UNRUN"
  fi
  # Serially, always: every acceptance case binds the same port. -p 1 is the
  # package-level bound; the cases themselves must not call t.Parallel().
  ( cd "$REPO_ROOT" && info "go test -race -p 1 -run TestAcceptance ./..." \
    && go test -race -p 1 -run TestAcceptance ./... )
}

# ============================ run =============================================
step "trainsty local CI  ${DIM}(mode: $MODE_LABEL, base: $BASE_REF)${RESET}"
check_go_pin
plan_jobs

# Registry in canonical order: blocking first, then report-only.
$DO_FMT        && add_job blocking "gofmt"                       job_fmt
$DO_VET        && add_job blocking "go vet"                      job_vet
$DO_BUILD      && add_job blocking "go build"                    job_build
$DO_TEST       && add_job blocking "go test -race"               job_test
$DO_COVER      && add_job blocking "coverage floor (scheduler/)" job_cover
$DO_DEPS       && add_job blocking "zero dependencies"           job_deps
$DO_BOUNDARY   && add_job blocking "os/exec boundary"            job_boundary
$DO_GITLEAKS   && add_job blocking "gitleaks"                    job_gitleaks
$DO_SHELLCHECK && add_job blocking "shell lint"                  job_shellcheck
$DO_E2E        && add_job blocking "acceptance suite"            job_e2e
$DO_VULN       && add_job report   "govulncheck"                 job_vuln
$DO_VOCAB      && add_job report   "vocabulary"                  job_vocab
$DO_DOCSLINT   && add_job report   "docs-lint (markdownlint)"    job_docslint

if [ "${#JOB_FUNC[@]}" -eq 0 ]; then
  warn "no jobs selected for this change — nothing to do"
  info "run 'scripts/ci-local.sh --all' to force the full set"
  exit 0
fi

if $LIST_ONLY; then
  step "Job plan"
  for i in "${!JOB_FUNC[@]}"; do printf "   %-32s %s\n" "${JOB_LABEL[$i]}" "${JOB_KIND[$i]}"; done
  exit 0
fi

MAX_PAR="${CI_LOCAL_JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"
case "$MAX_PAR" in ''|*[!0-9]*) MAX_PAR=4 ;; esac
[ "$MAX_PAR" -ge 1 ] || MAX_PAR=1
if $SEQUENTIAL || [ "${#JOB_FUNC[@]}" -le 1 ]; then dispatch_serial; else dispatch_parallel "$MAX_PAR"; fi

# ---- summary ----------------------------------------------------------------
step "Summary"
info "ran: ${JOBS_RUN[*]:-none}"
info "took: $((SECONDS / 60))m $((SECONDS % 60))s"
[ ${#REPORT_FINDINGS[@]} -gt 0 ] && warn "report-only findings: ${REPORT_FINDINGS[*]} (non-blocking; review before shipping)"
if [ ${#BLOCKING_FAILED[@]} -gt 0 ]; then
  fail "BLOCKING failed: ${BLOCKING_FAILED[*]}"
  exit 1
fi
# A job that could not run is not a job that passed. Qualify the green rather
# than letting "all blocking jobs passed" stand for a run where nothing executed.
if [ ${#JOBS_UNRUN[@]} -gt 0 ]; then
  warn "did NOT run: ${JOBS_UNRUN[*]}"
  if [ -n "$STRICT_UNRUN" ] && [ ${#BLOCKING_UNRUN[@]} -gt 0 ]; then
    fail "BLOCKING did not run: ${BLOCKING_UNRUN[*]} — a merge cannot claim these passed"
    exit 1
  fi
  pass "all blocking jobs that could run passed"
  exit 0
fi
pass "all blocking jobs passed"
exit 0
