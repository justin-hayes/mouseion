#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_root="$repo_root/.tmp/go-test"

clean_orphans() {
	if [[ ! -d "$tmp_root" ]]; then
		printf 'No Go test temporary directories found.\n'
		return
	fi

	local found=0
	while IFS= read -r -d '' run_dir; do
		found=1
		if [[ ! -O "$run_dir" ]]; then
			printf 'Keeping directory not owned by this user: %s\n' "$run_dir"
			continue
		fi
		if [[ ! -f "$run_dir/owner-pid" ]]; then
			printf 'Keeping unrecognized directory: %s\n' "$run_dir"
			continue
		fi
		local owner_pid
		owner_pid="$(<"$run_dir/owner-pid")"
		if [[ ! "$owner_pid" =~ ^[0-9]+$ ]]; then
			printf 'Keeping directory with invalid owner marker: %s\n' "$run_dir"
			continue
		fi
		if kill -0 "$owner_pid" 2>/dev/null; then
			printf 'Keeping active Go test directory (pid %s): %s\n' "$owner_pid" "$run_dir"
			continue
		fi
		rm -rf -- "$run_dir"
		printf 'Removed abandoned Go test directory: %s\n' "$run_dir"
	done < <(find "$tmp_root" -mindepth 1 -maxdepth 1 -type d -name 'run.*' -print0)

	if [[ $found -eq 0 ]]; then
		printf 'No Go test temporary directories found.\n'
	fi
}

if [[ "${1:-}" == clean ]]; then
	[[ $# -eq 1 ]] || { printf 'Usage: %s clean\n' "$0" >&2; exit 2; }
	clean_orphans
	exit
fi

mode="${1:-}"
if [[ "$mode" != unit && "$mode" != integration ]]; then
	printf 'Usage: %s {unit|integration} [package ...]\n       %s clean\n' "$0" "$0" >&2
	exit 2
fi
shift
packages=("$@")
if [[ ${#packages[@]} -eq 0 ]]; then
	if [[ "$mode" == integration ]]; then
		packages=(./internal/...)
	else
		packages=(./...)
	fi
fi

mkdir -p "$tmp_root"
run_dir="$(mktemp -d "$tmp_root/run.XXXXXXXX")"
printf '%s\n' "$$" >"$run_dir/owner-pid"
cleanup_run() {
	rm -rf -- "$run_dir"
}
trap cleanup_run EXIT

if [[ "$mode" == integration ]]; then
	GOTMPDIR="$run_dir" go test -count=1 -tags=integration "${packages[@]}"
else
	GOTMPDIR="$run_dir" go test "${packages[@]}"
fi
