#!/bin/sh
# Builds the application for a device without a signature and packs it as build/Linager.ipa.
# Sideloadly, AltStore or SideStore sign it with your own Apple ID when they install it, so nothing about the signing is kept here.
set -eu
cd "$(dirname "$0")"

command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen is needed: brew install xcodegen" >&2; exit 1; }

xcodegen generate --quiet
rm -rf build/dd build/Payload build/Linager.ipa
xcodebuild -project Linager.xcodeproj -scheme Linager -sdk iphoneos -destination 'generic/platform=iOS' -configuration Release \
  CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY="" -derivedDataPath build/dd build | { xcpretty 2>/dev/null || cat; } >build/xcodebuild.log 2>&1 || {
  tail -40 build/xcodebuild.log >&2
  echo "the build failed (the whole log: ios/build/xcodebuild.log)" >&2
  exit 1
}

APP=build/dd/Build/Products/Release-iphoneos/Linager.app
[ -d "$APP" ] || { echo "no $APP" >&2; exit 1; }
mkdir -p build/Payload
cp -R "$APP" build/Payload/
(cd build && zip -qry Linager.ipa Payload)
rm -rf build/Payload
VERSION=$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$APP/Info.plist")
echo "ios/build/Linager.ipa ($VERSION, unsigned, $(du -h build/Linager.ipa | cut -f1))"
