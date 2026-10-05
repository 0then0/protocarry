#!/bin/sh
# Prepare native binaries and archives, never publish a GitHub release.
set -eu
platform="$(go env GOOS)_$(go env GOARCH)"
case "$platform" in darwin_arm64|linux_arm64) ;; *) echo "Unvalidated release platform: $platform" >&2; exit 1 ;; esac
go test -mod=readonly -race ./...
go vet ./...
archive_dir="bin/release/$platform"
mkdir -p "$archive_dir"
CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=false -ldflags='-s -w' -o "$archive_dir/protocarry" ./cmd/protocarry
"$archive_dir/protocarry" version
archive="bin/protocarry_0.1.0_${platform}.tar.gz"
tar -czf "$archive" -C "$archive_dir" protocarry -C "$(pwd)" README.md LICENSE
archive_name="$(basename "$archive")"
(
  cd bin
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$archive_name" > "$archive_name.sha256"
  else
    shasum -a 256 "$archive_name" > "$archive_name.sha256"
  fi
)
printf '%s\n' "$archive"
