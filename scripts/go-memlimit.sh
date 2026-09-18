#!/usr/bin/env bash
# Print (or apply) GOMEMLIMIT at 75% of currently available RAM.
# Leaves GOMAXPROCS and compile -p to the toolchain. The 16GB MacBook
# kernel-panicked when Go was allowed to use 100% of RAM.
# shellcheck shell=bash
set -euo pipefail

magelift_go_memlimit() {
	local percent available_kib page pages mib
	percent="${MAGELIFT_GOMEMLIMIT_PERCENT:-75}"
	if ! [[ "$percent" =~ ^[0-9]+$ ]] || ((percent < 1 || percent > 99)); then
		printf 'go-memlimit: MAGELIFT_GOMEMLIMIT_PERCENT must be 1-99, got %s\n' "$percent" >&2
		return 1
	fi
	available_kib=""
	if [[ -n "${MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB:-}" ]]; then
		available_kib="$MAGELIFT_GOMEMLIMIT_AVAILABLE_KIB"
	elif [[ -r /proc/meminfo ]]; then
		available_kib="$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)"
		if [[ -z "$available_kib" ]]; then
			available_kib="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
		fi
	elif command -v sysctl >/dev/null 2>&1 && sysctl -n hw.memsize >/dev/null 2>&1; then
		page="$(pagesize 2>/dev/null || sysctl -n hw.pagesize)"
		pages="$(vm_stat | awk '
			/Pages free:/ { gsub(/\./, "", $NF); free = $NF }
			/Pages inactive:/ { gsub(/\./, "", $NF); inactive = $NF }
			/Pages speculative:/ { gsub(/\./, "", $NF); spec = $NF }
			/Pages purgeable:/ { gsub(/\./, "", $NF); purg = $NF }
			END { print free + inactive + spec + purg }
		')"
		if [[ -n "$pages" && "$pages" -gt 0 && -n "$page" ]]; then
			available_kib=$((pages * page / 1024))
		else
			available_kib=$(($(sysctl -n hw.memsize) / 1024))
		fi
	fi
	if [[ -z "$available_kib" ]] || ! [[ "$available_kib" =~ ^[0-9]+$ ]] || ((available_kib < 1)); then
		printf 'go-memlimit: cannot measure available RAM\n' >&2
		return 1
	fi
	mib=$((available_kib * percent / 100 / 1024))
	if ((mib < 256)); then
		mib=256
	fi
	printf '%dMiB\n' "$mib"
}

magelift_apply_go_memlimit() {
	local flags
	unset GOMAXPROCS
	if [[ -n "${GOFLAGS:-}" ]]; then
		flags="$(printf '%s' "$GOFLAGS" | sed -E 's/(^|[[:space:]])-p[= ]1([[:space:]]|$)/ /g')"
		flags="${flags#"${flags%%[![:space:]]*}"}"
		flags="${flags%"${flags##*[![:space:]]}"}"
		if [[ -n "$flags" ]]; then
			export GOFLAGS="$flags"
		else
			unset GOFLAGS
		fi
	fi
	GOMEMLIMIT="$(magelift_go_memlimit)"
	export GOMEMLIMIT
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	magelift_go_memlimit
fi
