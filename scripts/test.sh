#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p build/tests
GO_CONTRACT_OUTPUT="$PWD/build/tests/go-catalog.json" go test -race ./internal/...
swiftc -swift-version 6 PhospheneExtension/PlaylistTypes.swift PhospheneExtension/PlaybackModel.swift PhospheneExtension/PlaybackPolicy.swift PhospheneExtension/GeometryVerdict.swift tests/main.swift -o build/tests/timeline
build/tests/timeline build/tests/go-catalog.json build/tests/swift-catalog.json
SWIFT_CONTRACT_OUTPUT="$PWD/build/tests/swift-catalog.json" go test ./internal/catalog -run TestContracts -count=1
go vet ./...
