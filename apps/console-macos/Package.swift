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
        // Ghostty terminal — requires Xcode 16+ / Swift 6 toolchain to resolve.
        // The app target can stay in Swift 5 language mode.
        .package(
            url: "https://github.com/Lakr233/libghostty-spm.git",
            from: "1.3.1"
        )
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
                .product(name: "CodeEditor", package: "CodeEditor"),
                .product(name: "GhosttyTerminal", package: "libghostty-spm")
            ],
            path: "Sources/ConsoleMacOS"
        )
    ]
)
