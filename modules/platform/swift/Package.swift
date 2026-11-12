// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Platform",
    platforms: [.macOS(.v13)],
    products: [.library(name: "Platform", type: .dynamic, targets: ["Platform"])],
    dependencies: [],
    targets: [
        .target(name: "Platform"),
    ],
    swiftLanguageModes: [.v5],
)
