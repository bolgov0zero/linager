#!/bin/sh
# Writes bin/manifest.json for a tagged release: version, channel, checksums, sizes, DB migrations, notes.
# Every architecture in ARCHES (default: amd64 arm64) needs bin/linager_linux_<arch>. The top-level
# sha256 and size are the amd64 ones: agents of releases before arm64 support read only those.
set -eu
VERSION="${1:?usage: make-manifest.sh vX.Y.Z}"
ARCHES="${ARCHES:-amd64 arm64}"
case " $ARCHES " in *" amd64 "*) ;; *) echo "ARCHES must include amd64" >&2; exit 1 ;; esac
CHANNEL=stable
case "$VERSION" in *-*) CHANNEL=beta ;; esac

sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

ASSETS='{}'
for ARCH in $ARCHES; do
  BIN="bin/linager_linux_$ARCH"
  [ -f "$BIN" ] || { echo "missing $BIN" >&2; exit 1; }
  SIZE=$(wc -c < "$BIN" | tr -d ' ')
  ASSETS=$(printf '%s' "$ASSETS" | jq -c --arg a "$ARCH" --arg n "linager_linux_$ARCH" --arg sha "$(sha256 "$BIN")" --argjson size "$SIZE" \
    '. + {($a): {name: $n, sha256: $sha, size: $size}}')
done
# The Windows agent is optional: its file is there when the Windows job of the release workflow succeeded. The key is
# not a GOARCH value, so servers and agents that do not know it ignore it.
WIN=bin/linager_windows_amd64.exe
if [ -f "$WIN" ]; then
  WSIZE=$(wc -c < "$WIN" | tr -d ' ')
  ASSETS=$(printf '%s' "$ASSETS" | jq -c --arg n "linager_windows_amd64.exe" --arg sha "$(sha256 "$WIN")" --argjson size "$WSIZE" \
    '. + {windows_amd64: {name: $n, sha256: $sha, size: $size}}')
fi
MSI=bin/linager_windows_amd64.msi
if [ -f "$MSI" ]; then
  MSIZE=$(wc -c < "$MSI" | tr -d ' ')
  ASSETS=$(printf '%s' "$ASSETS" | jq -c --arg n "linager_windows_amd64.msi" --arg sha "$(sha256 "$MSI")" --argjson size "$MSIZE" \
    '. + {windows_msi: {name: $n, sha256: $sha, size: $size}}')
fi
# The desktop application (app_<GOOS>_<GOARCH>): the application updates itself from these entries (app/internal/update).
# macOS is Apple Silicon only. Each file is optional, like the Windows agent.
for ENTRY in app_darwin_arm64:Linager-macos-arm64.zip app_windows_amd64:Linager-windows-x64.zip app_linux_amd64:Linager-linux-x64.tar.gz; do
  KEY=${ENTRY%%:*}; NAME=${ENTRY#*:}
  if [ -f "bin/$NAME" ]; then
    ASIZE=$(wc -c < "bin/$NAME" | tr -d ' ')
    ASSETS=$(printf '%s' "$ASSETS" | jq -c --arg k "$KEY" --arg n "$NAME" --arg sha "$(sha256 "bin/$NAME")" --argjson size "$ASIZE" \
      '. + {($k): {name: $n, sha256: $sha, size: $size}}')
  fi
done
SHA=$(printf '%s' "$ASSETS" | jq -r .amd64.sha256)
SIZE=$(printf '%s' "$ASSETS" | jq -r .amd64.size)
MIGRATIONS=$(find migrations -name '*.sql' | wc -l | tr -d ' ')
PREV=$(git describe --tags --abbrev=0 "${VERSION}^" 2>/dev/null || true)
RANGE=${PREV:+$PREV..}$VERSION
NOTES=$(git log --no-merges --pretty='%s' "$RANGE" | head -15 | jq -R . | jq -s .)
# Agents older than this cannot be updated through the panel and are shown as unsupported.
MIN_AGENT="${MIN_AGENT:-v0.1.0}"
jq -n --arg v "$VERSION" --arg c "$CHANNEL" --arg m "$MIN_AGENT" --arg sha "$SHA" --argjson size "$SIZE" --argjson assets "$ASSETS" \
  --argjson mig "$MIGRATIONS" --argjson notes "$NOTES" --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{version:$v, channel:$c, min_agent:$m, sha256:$sha, size:$size, assets:$assets, migrations:$mig, notes:$notes, built_at:$t}' > bin/manifest.json
