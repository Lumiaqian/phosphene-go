#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p build
go tool mygo build -platform darwin/arm64 -skip-dmg
xcodebuild -project Phosphene.xcodeproj -scheme PhospheneExtension \
  -configuration Release -destination 'generic/platform=macOS' \
  -derivedDataPath build/xcode CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO \
  CODE_SIGN_IDENTITY='' DEVELOPMENT_TEAM='' build > build/extension-build.log 2>&1
app='dist/darwin-arm64/Phosphene Go.app'
extension="$app/Contents/Extensions/PhospheneExtension.appex"
mkdir -p "$app/Contents/Extensions"
ditto 'build/xcode/Build/Products/Release/PhospheneExtension.appex' "$extension"
codesign --force --sign - --entitlements scripts/extension.entitlements "$extension"
cp LICENSE "$app/Contents/Resources/LICENSE-Phosphene.txt"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
printf '%s\n' "$PWD/$app"
