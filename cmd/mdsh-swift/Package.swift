// swift-tools-version:5.9
//
// mdsh-swift is mdsh-gui's window, written in SwiftUI. The Go bridge in
// bridge/ does the work: parsing, running, editing and saving are the
// same code mdsh and mdsh-gui use. See README.md.
import PackageDescription

let package = Package(
    name: "mdsh-swift",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "mdsh-swift", targets: ["MdshSwift"]),
        .library(name: "MdshKit", targets: ["MdshKit"]),
    ],
    targets: [
        // MdshKit talks to the bridge and holds the types it sends.
        // It needs only Foundation.
        .target(name: "MdshKit"),
        // MdshSwift is the app.
        .executableTarget(name: "MdshSwift", dependencies: ["MdshKit"]),
        .testTarget(name: "MdshKitTests", dependencies: ["MdshKit"]),
    ]
)
