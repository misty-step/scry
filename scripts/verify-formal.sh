#!/usr/bin/env bash
# verify-formal.sh: Scry's formal and property-based verification in one go.
#
#   1. Quint, on every specs/*.qnt: typecheck, the spec's own tests, and
#      random simulation of inv_all with the Rust backend.
#      VERIFY_APALACHE=1 also runs bounded model checking with Apalache
#      (quint verify), each run capped by APALACHE_TIMEOUT seconds.
#   2. Go rapid stateful property tests and the explicit state machine
#      tables (-race), plus a deeper rapid pass on the packages that use it.
#   3. Go native fuzz smoke tests, FUZZTIME each.
#
# Knobs (environment): QUINT_SAMPLES (5000), QUINT_STEPS (30),
# VERIFY_APALACHE (0), APALACHE_STEPS (2), APALACHE_TIMEOUT (90),
# FUZZTIME (3s), RAPID_CHECKS (300), RAPID_STEPS (100).
# Exits non-zero if anything fails.
set -uo pipefail

cd "$(dirname "$0")/.."

QUINT_SAMPLES=${QUINT_SAMPLES:-5000}
QUINT_STEPS=${QUINT_STEPS:-30}
VERIFY_APALACHE=${VERIFY_APALACHE:-0}
APALACHE_STEPS=${APALACHE_STEPS:-2}
APALACHE_TIMEOUT=${APALACHE_TIMEOUT:-90}
FUZZTIME=${FUZZTIME:-3s}
RAPID_CHECKS=${RAPID_CHECKS:-300}
RAPID_STEPS=${RAPID_STEPS:-100}

results=()
failed=0

step() { printf '\n==> %s\n' "$*"; }

# run LABEL CMD...: runs a check, records PASS/FAIL and its duration.
run() {
	local label=$1
	shift
	step "$label"
	printf '$ %s\n' "$*"
	local start=$SECONDS
	if "$@"; then
		results+=("PASS  $label ($((SECONDS - start))s)")
	else
		results+=("FAIL  $label ($((SECONDS - start))s)")
		failed=1
	fi
}

# ---------------------------------------------------------------- tools

find_quint() {
	command -v quint >/dev/null 2>&1 && return 0
	local env
	for env in "${QUINT_ENV:-}" "$HOME/chief-evidence/quint-env.sh" /home/user/chief-evidence/quint-env.sh; do
		if [ -n "$env" ] && [ -f "$env" ]; then
			# shellcheck disable=SC1090
			. "$env"
			command -v quint >/dev/null 2>&1 && return 0
		fi
	done
	local dir
	for dir in "$HOME"/.nvm/versions/node/*/bin "$HOME/.npm-global/bin" /usr/local/bin; do
		if [ -x "$dir/quint" ]; then
			PATH="$dir:$PATH"
			return 0
		fi
	done
	return 1
}

if ! find_quint; then
	echo "verify-formal: quint not found. Install it with: npm i -g @informalsystems/quint" >&2
	echo "  (Apalache checks, VERIFY_APALACHE=1, also need Java 17+ on PATH.)" >&2
	exit 127
fi
if ! command -v go >/dev/null 2>&1; then
	echo "verify-formal: go not found on PATH" >&2
	exit 127
fi
echo "quint $(quint --version 2>/dev/null) at $(command -v quint)"
go version

# ---------------------------------------------------------------- 1. Quint

shopt -s nullglob
specs=(specs/*.qnt)
if [ ${#specs[@]} -eq 0 ]; then
	echo "verify-formal: no specs/*.qnt" >&2
	exit 1
fi
for spec in "${specs[@]}"; do
	name=$(basename "$spec" .qnt)
	run "quint typecheck $name" quint typecheck "$spec"
	run "quint test $name" quint test "$spec"
	run "quint run $name inv_all (${QUINT_SAMPLES}x${QUINT_STEPS})" \
		quint run "$spec" --backend=rust --invariant=inv_all --max-samples="$QUINT_SAMPLES" --max-steps="$QUINT_STEPS"
done

if [ "$VERIFY_APALACHE" = "1" ]; then
	if ! command -v java >/dev/null 2>&1; then
		echo "verify-formal: VERIFY_APALACHE=1 needs Java 17+ on PATH" >&2
		results+=("FAIL  apalache (no java)")
		failed=1
	else
		# quint verify starts an Apalache server (a JVM) and may leave it
		# running, more so when timeout cuts it short: one this script
		# started is stopped at the end.
		apalache_running=0
		pgrep -f 'apalache[.]jar server' >/dev/null 2>&1 && apalache_running=1
		for spec in "${specs[@]}"; do
			name=$(basename "$spec" .qnt)
			run "apalache $name inv_all (bounded, ${APALACHE_STEPS} steps)" \
				timeout "${APALACHE_TIMEOUT}s" quint verify "$spec" --invariant=inv_all \
				--max-steps="$APALACHE_STEPS"
			if grep -q 'val inv_inductive' "$spec"; then
				run "apalache $name inv_inductive (inductive)" \
					timeout "${APALACHE_TIMEOUT}s" quint verify "$spec" --inductive-invariant=inv_inductive
			fi
		done
		if [ "$apalache_running" = "0" ]; then
			pkill -f 'apalache[.]jar server' >/dev/null 2>&1 || true
		fi
		rm -rf _apalache-out
	fi
fi

# ---------------------------------------------------------------- 2. rapid

run "go rapid + transition tables (-race)" \
	go test -race -count=1 -run 'Rapid|TransitionTable|ValidateState' ./...
# rapid's flags exist only in the packages that import it.
rapid_pkgs=$(grep -l 'pgregory.net/rapid' $(git ls-files '*_test.go') 2>/dev/null | xargs -r -n1 dirname | sort -u | sed 's|^|./|')
if [ -n "$rapid_pkgs" ]; then
	# shellcheck disable=SC2086
	run "go rapid deep (${RAPID_CHECKS} checks x ${RAPID_STEPS} steps)" \
		go test -count=1 -run 'Rapid' $rapid_pkgs -rapid.checks="$RAPID_CHECKS" -rapid.steps="$RAPID_STEPS"
fi

# ---------------------------------------------------------------- 3. fuzz

# Each target alone: -fuzz must match exactly one target per package.
fuzz_targets=(
	"./internal/srs FuzzFSRSNext"
	"./internal/srs FuzzFSRSSequence"
	"./internal/billing FuzzStripeParseEvent"
	"./internal/billing FuzzStripeVerifySignature"
)
for entry in "${fuzz_targets[@]}"; do
	read -r pkg target <<<"$entry"
	run "fuzz $target ($FUZZTIME)" go test -run='^$' -fuzz="^${target}\$" -fuzztime="$FUZZTIME" "$pkg"
done

# ---------------------------------------------------------------- summary

printf '\n==> summary\n'
printf '%s\n' "${results[@]}"
if [ "$failed" -ne 0 ]; then
	echo "verify-formal: FAILED" >&2
	exit 1
fi
echo "verify-formal: all checks passed"
