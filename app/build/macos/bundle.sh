#!/bin/sh
# Builds bin/Linager.app from bin/linager-app (macOS only): Info.plist, the icon made from build/appicon.png (the
# current Linager logo) and an ad-hoc signature (Apple Silicon refuses to run an unsigned program).
# usage: bundle.sh <version>
set -eu
VERSION="${1:-dev}"
cd "$(dirname "$0")/../.."
rm -rf bin/Linager.app bin/icon.iconset
mkdir -p bin/Linager.app/Contents/MacOS bin/Linager.app/Contents/Resources bin/icon.iconset
cp bin/linager-app bin/Linager.app/Contents/MacOS/linager-app
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" build/appicon.png --out "bin/icon.iconset/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z "$d" "$d" build/appicon.png --out "bin/icon.iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns bin/icon.iconset -o bin/Linager.app/Contents/Resources/Linager.icns
rm -rf bin/icon.iconset
cat > bin/Linager.app/Contents/Info.plist <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Linager</string>
  <key>CFBundleDisplayName</key><string>Linager</string>
  <key>CFBundleIdentifier</key><string>io.github.bolgov0zero.linager</string>
  <key>CFBundleExecutable</key><string>linager-app</string>
  <key>CFBundleIconFile</key><string>Linager</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>w/ ❤️ by bolgov0zero</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
</dict></plist>
PLIST
xattr -cr bin/Linager.app
codesign --force --deep -s - bin/Linager.app
echo bin/Linager.app
