// swift-tools-version: 5.9
import PackageDescription

/// StreamMarkdown — streaming Markdown renderer vendored from Codevisor.
/// macOS 13 + Swift 5.9 compatible. Foundation / SwiftUI / AppKit only,
/// no cross-package dependencies.
let package = Package(
    name: "StreamMarkdown",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .library(name: "StreamMarkdown", targets: ["StreamMarkdown"])
    ],
    dependencies: [],
    targets: [
        .target(
            name: "StreamMarkdown",
            path: "Sources/StreamMarkdown"
        )
    ]
)
