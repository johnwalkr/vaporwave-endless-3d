#!/bin/bash
# One-time setup on a Mac: builds the Go core, generates the Xcode project, opens it.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin"
command -v go >/dev/null || { echo "Install Go 1.24 or newer first: brew install go"; exit 1; }
command -v xcodegen >/dev/null || { echo "Installing XcodeGen..."; brew install xcodegen; }
./build-go.sh
xcodegen generate
open VaporwaveEndless3D.xcodeproj
