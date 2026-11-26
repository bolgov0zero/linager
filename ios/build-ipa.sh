#!/bin/sh
# Builds the application for a device without a signature and packs it as build/Linager.ipa.
# Sideloadly, AltStore or SideStore sign it with your own Apple ID when they install it, so nothing about the signing is kept here.
# The release workflow sets the version of the application: IOS_MARKETING_VERSION (digits and dots, "0.10.0") and IOS_BUILD_NUMBER (a whole
# number that only grows); without them the numbers of ios/project.yml are used.
set -eu
cd "$(dirname "$0")"
mkdir -p build

command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen is needed: brew install xcodegen" >&2; exit 1; }

# The RDP core is Rust. rustup is needed with the target of the device: brew install rustup, then
#   rustup toolchain install stable --profile minimal -t aarch64-apple-ios
# (Homebrew keeps rustup out of the PATH, so its place is added here.)
PATH="/opt/homebrew/opt/rustup/bin:$HOME/.cargo/bin:$PATH"
command -v rustup >/dev/null 2>&1 || { echo "rustup is needed for the RDP core: brew install rustup, then rustup toolchain install stable --profile minimal -t aarch64-apple-ios" >&2; exit 1; }
rustup target list --installed 2>/dev/null | grep -q '^aarch64-apple-ios$' || { echo "the Rust target of the device is missing: rustup target add aarch64-apple-ios" >&2; exit 1; }
if ! cargo build --release --target aarch64-apple-ios --locked --manifest-path rdp/Cargo.toml >build/rdp.log 2>&1; then
  tail -30 build/rdp.log >&2
  echo "the RDP core failed to build (the whole log: ios/build/rdp.log)" >&2
  exit 1
fi

# numbers given by the release: a wrong shape is refused here, the stores of sideloading tools reject it later and less clearly
case "${IOS_MARKETING_VERSION:-0.0.0}" in *[!0-9.]*|.*|*.) echo "IOS_MARKETING_VERSION must be digits and dots: ${IOS_MARKETING_VERSION}" >&2; exit 1 ;; esac
case "${IOS_BUILD_NUMBER:-0}" in *[!0-9]*) echo "IOS_BUILD_NUMBER must be a whole number: ${IOS_BUILD_NUMBER}" >&2; exit 1 ;; esac

xcodegen generate --quiet
rm -rf build/dd build/Payload build/Linager.ipa
if ! xcodebuild -project Linager.xcodeproj -scheme Linager -sdk iphoneos -destination 'generic/platform=iOS' -configuration Release -skipPackagePluginValidation \
  CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY="" -derivedDataPath build/dd \
  ${IOS_MARKETING_VERSION:+MARKETING_VERSION=$IOS_MARKETING_VERSION} ${IOS_BUILD_NUMBER:+CURRENT_PROJECT_VERSION=$IOS_BUILD_NUMBER} build >build/xcodebuild.log 2>&1; then
  grep -E "error:" build/xcodebuild.log | sed 's/\x1b\[[0-9;]*m//g' | sort -u | head -30 >&2
  echo "the build failed (the whole log: ios/build/xcodebuild.log)" >&2
  exit 1
fi

APP=build/dd/Build/Products/Release-iphoneos/Linager.app
[ -d "$APP" ] || { echo "no $APP" >&2; exit 1; }
mkdir -p build/Payload
cp -R "$APP" build/Payload/
(cd build && zip -qry Linager.ipa Payload)
rm -rf build/Payload
VERSION=$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$APP/Info.plist")
BUILD=$(/usr/libexec/PlistBuddy -c 'Print CFBundleVersion' "$APP/Info.plist")
echo "ios/build/Linager.ipa ($VERSION ($BUILD), unsigned, $(du -h build/Linager.ipa | cut -f1))"
