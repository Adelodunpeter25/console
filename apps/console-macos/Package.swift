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
    dependencies: [
        .package(
            url: "https://github.com/ZeeZide/CodeEditor.git",
            branch: "main"
        ),
        // Ghostty terminal — vendored locally, downgraded to swift-tools-version 5.9.
        // Temporarily disabled while Swift 5.9 compatibility is finalized.
        // .package(path: "Packages/libghostty-spm")
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

        // MARK: ConsoleMacOS — SwiftUI app
        .executableTarget(
            name: "ConsoleMacOS",
            dependencies: [
                "ConsoleCore",
                .product(name: "CodeEditor", package: "CodeEditor")
            ],
            path: "Sources/ConsoleMacOS",
            // Terminal pane temporarily excluded while libghostty-spm
            // Swift 5.9 compatibility is finalized.
            exclude: [
                "Features/Terminal"
            ]
        )
    ]
)
