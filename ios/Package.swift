// swift-tools-version: 6.0
// LinagerKit: everything of the iOS client that is not a screen (the pinned TLS connection, the API client, the state of the
// dashboard). It builds and is tested on the Mac with `swift test`; the application (App/, project.yml) only draws it.
import PackageDescription

let package = Package(
    name: "LinagerKit",
    platforms: [.iOS("26.0"), .macOS("14.0")],
    products: [.library(name: "LinagerKit", targets: ["LinagerKit"])],
    targets: [
        .target(name: "LinagerKit"),
        .testTarget(name: "LinagerKitTests", dependencies: ["LinagerKit"], resources: [.copy("Fixtures")]),
    ]
)
