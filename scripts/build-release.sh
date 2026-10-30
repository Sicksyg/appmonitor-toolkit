#!/usr/bin/env zsh
#
# Build AppMonitor after staging its Homebrew-provided macOS helper tools.
#
# Usage:
#   scripts/build-release.sh [--stage-only]
#
# The staging phase:
#   1. Resolves ideviceinstaller, idevice_id, ideviceinfo, ipatool, ldid, and
#      iproxy.
#      from their Homebrew formula prefixes.
#   2. Walks each executable's non-system dylib dependencies recursively and
#      prints the full dependency list before copying anything.
#   3. Copies the tools to assets/bin/darwin-<arch>/ and the dependency
#      closure to assets/lib/darwin-<arch>/.
#   4. Rewrites tool dependencies to @rpath/<dylib>, gives each tool an
#      @executable_path/../lib/darwin-<arch> rpath, and rewrites each dylib's
#      dependencies to @loader_path/<dylib>.
#   5. Sets each copied dylib ID to @rpath/<dylib> and ad-hoc signs all
#      changed Mach-O files after their load commands have been changed.
#      Unchanged files are skipped using source and staged-file SHA-256 hashes.
#
# Normal mode then clears generated reports and cached AppMonitor tools,
# builds the Wails application and CLI, and opens the .app bundle.
# Run as the normal logged-in user, never with sudo: Wails writes generated
# bindings and build output as the invoking user.
# Staging replaces existing files instead of truncating them, so stale
# read-only/root-owned files can be refreshed when the asset directories are
# writable by the current user.
# --stage-only stops after staging, rewriting, and signing; it is useful for
# inspecting the asset files without building or launching AppMonitor.
#
# Environment:
#   APPMONITOR_ASSET_ROOT  Override the assets directory (useful for testing).
#
# Requirements:
#   zsh, Homebrew, Xcode command-line tools (otool, install_name_tool, lipo,
#   codesign), shasum, Wails, and these Homebrew formulae:
#   libimobiledevice, ipatool, ldid, and libusbmuxd.
#
# The current script stages the architecture of the running Homebrew
# installation (Apple Silicon arm64 or Intel x86_64). It does not create a
# universal binary or cross-build tools for another architecture.

set -euo pipefail

cd "${0:A:h:h}"

repo_root="$PWD"
if (( EUID == 0 )); then
	print -u2 "Do not run this script with sudo; Wails would create root-owned build files."
	print -u2 "Run it as your normal user: scripts/build-release.sh"
	exit 1
fi

build_bin_dir="$repo_root/build/bin"
if [[ -e "$build_bin_dir" && ! -w "$build_bin_dir" ]]; then
	print -u2 "Cannot build: $build_bin_dir is not writable by $(id -un)."
	print -u2 "This usually means a previous build was run with sudo and created root-owned output."
	print -u2 "Repair it with:"
	print -u2 "  sudo chown -R \"$(id -un)\" \"$build_bin_dir\""
	exit 1
fi

brew_prefix="$(brew --prefix)"
asset_root="${APPMONITOR_ASSET_ROOT:-$repo_root/assets}"
host_arch="$(uname -m)"
case "$host_arch" in
	arm64|aarch64)
		asset_arch="arm64"
	;;
	x86_64)
		asset_arch="amd64"
	;;
	*)
		print -u2 "Unsupported Homebrew architecture: $host_arch"
		exit 1
	;;
esac

bin_dir="$asset_root/bin/darwin-$asset_arch"
lib_dir="$asset_root/lib/darwin-$asset_arch"
mkdir -p "$bin_dir" "$lib_dir"
for output_dir in "$bin_dir" "$lib_dir"; do
	if [[ ! -w "$output_dir" ]]; then
		print -u2 "Cannot stage Homebrew tools: $output_dir is not writable by $(id -un)."
		print -u2 "Repair the staged asset directory with:"
		print -u2 "  sudo chown -R \"$(id -un)\" \"$bin_dir\" \"$lib_dir\""
		exit 1
	fi
done

typeset -A library_sources
typeset -A tool_sources
typeset -A library_needs_stage
typeset -A tool_needs_stage
typeset -A library_source_hashes
typeset -A tool_source_hashes

# Increment this when the staging or signing procedure changes.
stage_format=1
stage_cache_dir="$HOME/Library/Caches/AppMonitor/build-release/darwin-$asset_arch"
mkdir -p "$stage_cache_dir/libraries" "$stage_cache_dir/tools"
for cache_dir in "$stage_cache_dir/libraries" "$stage_cache_dir/tools"; do
	if [[ ! -w "$cache_dir" ]]; then
		print -u2 "Cannot write staged binary cache: $cache_dir"
		exit 1
	fi
done

file_sha256() {
	local output
	output="$(shasum -a 256 "$1")" || return 1
	print -r -- "${output%% *}"
}

stage_is_current() {
	local source="$1"
	local destination="$2"
	local state_file="$3"
	local require_executable="${4:-false}"
	local cached_format
	local cached_source_hash
	local cached_destination_hash
	local destination_hash

	stage_source_hash="$(file_sha256 "$source")" || {
		print -u2 "Unable to hash Homebrew source: $source"
		return 2
	}
	if [[ ! -f "$destination" || ! -f "$state_file" ]]; then
		return 1
	fi
	if [[ "$require_executable" == true && ! -x "$destination" ]]; then
		return 1
	fi
	if [[ ! -r "$state_file" ]]; then
		print -u2 "Unable to read staged binary cache: $state_file"
		return 2
	fi
	read -r cached_format cached_source_hash cached_destination_hash < "$state_file" || return 1
	if [[ "$cached_format" != "$stage_format" || "$cached_source_hash" != "$stage_source_hash" ]]; then
		return 1
	fi
	destination_hash="$(file_sha256 "$destination")" || {
		print -u2 "Unable to hash staged binary: $destination"
		return 2
	}
	[[ "$cached_destination_hash" == "$destination_hash" ]]
}

record_stage() {
	local source="$1"
	local destination="$2"
	local state_file="$3"
	local expected_source_hash="$4"
	local actual_source_hash
	local destination_hash
	local temporary_state="$state_file.tmp.$$"

	actual_source_hash="$(file_sha256 "$source")" || return 1
	if [[ "$actual_source_hash" != "$expected_source_hash" ]]; then
		print -u2 "Source changed while staging: $source"
		return 1
	fi
	destination_hash="$(file_sha256 "$destination")" || return 1
	print -r -- "$stage_format $actual_source_hash $destination_hash" > "$temporary_state"
	mv -f "$temporary_state" "$state_file"
}

is_system_library() {
	case "$1" in
		/usr/lib/*|/System/Library/*)
			return 0
		;;
		*)
			return 1
		;;
	esac
}

resolve_library() {
	local dependency="$1"
	local candidate
	local library_name="${dependency:t}"

	if [[ -f "$dependency" ]]; then
		print -r -- "$dependency"
		return 0
	fi

	candidate="$(find -L "$brew_prefix/opt" -type f -name "$library_name" -print -quit 2>/dev/null)"
	if [[ -n "$candidate" && -f "$candidate" ]]; then
		print -r -- "$candidate"
		return 0
	fi
	return 1
}

library_dependencies() {
	local image="$1"
	local output
	output="$(otool -L "$image")" || {
		print -u2 "Unable to inspect Mach-O dependencies: $image"
		return 1
	}
	print -r -- "$output" | awk 'NR > 1 { sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+\(.*/, ""); if ($0 != "") print }'
}

has_rpath() {
	local image="$1"
	local expected="$2"
	local output
	output="$(otool -l "$image")" || {
		print -u2 "Unable to inspect Mach-O load commands: $image"
		return 1
	}
	print -r -- "$output" | awk -v expected="$expected" '
		$1 == "cmd" && $2 == "LC_RPATH" {
			getline
			getline
			sub(/^[[:space:]]*path /, "")
			sub(/ \(offset.*/, "")
			if ($0 == expected) found = 1
		}
		END { exit !found }
	'
}

collect_library() {
	local source="$1"
	local library_name="${source:t}"
	local dependency
	local resolved
	local dependencies

	if [[ -n "${library_sources[$library_name]-}" ]]; then
		return 0
	fi
	library_sources[$library_name]="$source"

	lipo "$source" -verify_arch "$host_arch" >/dev/null 2>&1 || {
		print -u2 "Homebrew library has the wrong architecture ($host_arch): $source"
		return 1
	}

	dependencies="$(library_dependencies "$source")" || return 1
	for dependency in ${(f)dependencies}; do
		[[ -n "$dependency" ]] || continue
		if is_system_library "$dependency"; then
			continue
		fi
		resolved="$(resolve_library "$dependency")" || {
			print -u2 "Could not resolve dylib dependency '$dependency' required by $source"
			return 1
		}
		collect_library "$resolved"
	done
}

resolve_tool() {
	local tool="$1"
	local formula="$2"
	local formula_prefix
	local source
	local dependency
	local resolved
	local dependencies

	formula_prefix="$(brew --prefix "$formula" 2>/dev/null)" || {
		print -u2 "Homebrew formula '$formula' is required to stage $tool"
		return 1
	}
	source="$formula_prefix/bin/$tool"
	if [[ ! -x "$source" ]]; then
		source="$(command -v "$tool" 2>/dev/null || true)"
	fi
	if [[ -z "$source" || ! -x "$source" ]]; then
		print -u2 "Could not find executable '$tool' from Homebrew formula '$formula'"
		return 1
	fi
	lipo "$source" -verify_arch "$host_arch" >/dev/null 2>&1 || {
		print -u2 "Homebrew executable has the wrong architecture ($host_arch): $source"
		return 1
	}
	tool_sources[$tool]="$source"

	dependencies="$(library_dependencies "$source")" || return 1
	for dependency in ${(f)dependencies}; do
		[[ -n "$dependency" ]] || continue
		if is_system_library "$dependency"; then
			continue
		fi
		resolved="$(resolve_library "$dependency")" || {
			print -u2 "Could not resolve dylib dependency '$dependency' required by $tool"
			return 1
		}
		collect_library "$resolved"
	done
}

copy_library() {
	local library_name="$1"
	local source="${library_sources[$library_name]}"
	local destination="$lib_dir/$library_name"
	rm -f "$destination"
	cp -L "$source" "$destination"
}

patch_library() {
	local library_name="$1"
	local source="${library_sources[$library_name]}"
	local destination="$lib_dir/$library_name"
	local dependency
	local dependencies

	dependencies="$(library_dependencies "$source")" || return 1
	for dependency in ${(f)dependencies}; do
		[[ -n "$dependency" ]] || continue
		if is_system_library "$dependency"; then
			continue
		fi
		install_name_tool -change "$dependency" "@loader_path/${dependency:t}" "$destination"
	done
	install_name_tool -id "@rpath/$library_name" "$destination"
}

sign_library() {
	local library_name="$1"
	local destination="$lib_dir/$library_name"
	codesign --force --sign - --timestamp=none "$destination"
}

copy_tool() {
	local tool="$1"
	local source="${tool_sources[$tool]}"
	local destination="$bin_dir/$tool"
	rm -f "$destination"
	cp -L "$source" "$destination"
	chmod 755 "$destination"
}

patch_tool() {
	local tool="$1"
	local source="${tool_sources[$tool]}"
	local destination="$bin_dir/$tool"
	local dependency
	local dependencies
	local runtime_rpath="@executable_path/../lib/darwin-$asset_arch"

	dependencies="$(library_dependencies "$source")" || return 1
	for dependency in ${(f)dependencies}; do
		[[ -n "$dependency" ]] || continue
		if is_system_library "$dependency"; then
			continue
		fi
		install_name_tool -change "$dependency" "@rpath/${dependency:t}" "$destination"
	done
	if ! has_rpath "$destination" "$runtime_rpath"; then
		install_name_tool -add_rpath "$runtime_rpath" "$destination"
	fi
}

sign_tool() {
	local tool="$1"
	local destination="$bin_dir/$tool"
	codesign --force --sign - --timestamp=none "$destination"
}

resolve_tool ideviceinstaller libimobiledevice
resolve_tool idevice_id libimobiledevice
resolve_tool ideviceinfo libimobiledevice
resolve_tool ipatool ipatool
resolve_tool ldid ldid
resolve_tool iproxy libusbmuxd

print "Required Homebrew dylibs for $host_arch:"
for library_name in ${(ok)library_sources}; do
	print "  $library_name <- ${library_sources[$library_name]}"
done

# Copy the complete dependency closure and all root executables before editing
# any Mach-O metadata, so every source is resolved from the same Homebrew set.
for library_name in ${(ok)library_sources}; do
	source="${library_sources[$library_name]}"
	destination="$lib_dir/$library_name"
	state_file="$stage_cache_dir/libraries/$library_name.state"
	if stage_is_current "$source" "$destination" "$state_file"; then
		print "Unchanged $library_name; skipping copy, patching, and signing"
	else
		stage_status=$?
		if [[ "$stage_status" -ne 1 ]]; then
			exit "$stage_status"
		fi
		library_needs_stage[$library_name]=1
		library_source_hashes[$library_name]="$stage_source_hash"
		copy_library "$library_name"
	fi
done
for tool in ideviceinstaller idevice_id ideviceinfo ipatool ldid iproxy; do
	source="${tool_sources[$tool]}"
	destination="$bin_dir/$tool"
	state_file="$stage_cache_dir/tools/$tool.state"
	if stage_is_current "$source" "$destination" "$state_file" true; then
		print "Unchanged $tool; skipping copy, patching, and signing"
	else
		stage_status=$?
		if [[ "$stage_status" -ne 1 ]]; then
			exit "$stage_status"
		fi
		tool_needs_stage[$tool]=1
		tool_source_hashes[$tool]="$stage_source_hash"
		print "Copying $tool from $source"
		copy_tool "$tool"
	fi
done

# Rewrite Homebrew install names and add the extracted layout's executable
# rpath before applying ad-hoc signatures to changed Mach-O files.
for library_name in ${(ok)library_sources}; do
	if [[ "${library_needs_stage[$library_name]-}" == 1 ]]; then
		patch_library "$library_name"
	fi
done
for tool in ideviceinstaller idevice_id ideviceinfo ipatool ldid iproxy; do
	if [[ "${tool_needs_stage[$tool]-}" == 1 ]]; then
		patch_tool "$tool"
	fi
done
for library_name in ${(ok)library_sources}; do
	if [[ "${library_needs_stage[$library_name]-}" == 1 ]]; then
		sign_library "$library_name"
		record_stage "${library_sources[$library_name]}" "$lib_dir/$library_name" \
			"$stage_cache_dir/libraries/$library_name.state" "${library_source_hashes[$library_name]}"
	fi
done
for tool in ideviceinstaller idevice_id ideviceinfo ipatool ldid iproxy; do
	if [[ "${tool_needs_stage[$tool]-}" == 1 ]]; then
		sign_tool "$tool"
		record_stage "${tool_sources[$tool]}" "$bin_dir/$tool" \
			"$stage_cache_dir/tools/$tool.state" "${tool_source_hashes[$tool]}"
	fi
done

if [[ "${1-}" == "--stage-only" ]]; then
	exit 0
fi

report_dir="$HOME/Documents/AppMonitor Output"
app_support_dir="$HOME/Library/Application Support/AppMonitor"

# Remove the existing report directory and app support directory
# rm -rf "$report_dir"
if [[ -d "$app_support_dir" ]]; then
	rm -rf "$app_support_dir/bin" "$app_support_dir/frida" "$app_support_dir/lib"
fi

wails build -clean
go build -o build/appmonitor ./cmd
open build/bin/AppMonitor.app
