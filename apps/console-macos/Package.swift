// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ConsoleMacOS",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(name: "ConsoleMacOS", targets: ["ConsoleMacOS"]),
        .library(name: "ConsoleCore", targets: ["ConsoleCore"])
    ],
    targets: [
        // MARK: ConsoleCore — models + API client matching the Rust/Node backend
        .target(
            name: "ConsoleCore",
            path: "Sources/ConsoleCore"
        ),
        .testTarget(
            name: "ConsoleCoreTests",
            dependencies: ["ConsoleCore"],
            path: "Tests/ConsoleCoreTests"
        ),

        // MARK: ConsoleMacOS — SwiftUI app (UI files ported from Codevisor)
        .executableTarget(
            name: "ConsoleMacOS",
            dependencies: ["ConsoleCore"],
            path: "Sources/ConsoleMacOS"
        )
    ]
)
