#!/bin/bash
# Builds the Go core (../capi) as static libraries for iPhone and the simulator
# and packages them as Generated/VE3DCore.xcframework. Xcode runs this before
# every build; Go's build cache makes it quick when nothing changed.
set -euo pipefail
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$HOME/go/bin"
cd "$(dirname "$0")"
IOS_DIR=$(pwd)
GEN="$IOS_DIR/Generated"
MIN=16.0

if ! command -v go >/dev/null; then
  echo "error: Go is not installed. Install Go 1.24 or newer (brew install go) and build again." >&2
  exit 1
fi

build() { # goarch sdk clang-target outdir
  local sdkpath clang
  sdkpath=$(xcrun --sdk "$2" --show-sdk-path)
  clang=$(xcrun --sdk "$2" --find clang)
  mkdir -p "$4"
  (cd .. && CGO_ENABLED=1 GOOS=ios GOARCH="$1" CC="$clang" \
    CGO_CFLAGS="-isysroot $sdkpath -target $3 -O2" CGO_LDFLAGS="-isysroot $sdkpath -target $3" \
    go build -trimpath -buildmode=c-archive -o "$4/libve3d.a" ./capi)
}

B="$GEN/build"
build arm64 iphoneos "arm64-apple-ios$MIN" "$B/device"
build arm64 iphonesimulator "arm64-apple-ios$MIN-simulator" "$B/sim-arm64"
build amd64 iphonesimulator "x86_64-apple-ios$MIN-simulator" "$B/sim-x86_64"
mkdir -p "$B/sim" "$GEN/include"
lipo -create "$B/sim-arm64/libve3d.a" "$B/sim-x86_64/libve3d.a" -output "$B/sim/libve3d.a"
cp "$B/device/libve3d.h" "$GEN/include/libve3d.h"
rm -rf "$GEN/VE3DCore.xcframework"
xcodebuild -create-xcframework \
  -library "$B/device/libve3d.a" -headers "$GEN/include" \
  -library "$B/sim/libve3d.a" -headers "$GEN/include" \
  -output "$GEN/VE3DCore.xcframework" >/dev/null
echo "Go core built: $GEN/VE3DCore.xcframework"
