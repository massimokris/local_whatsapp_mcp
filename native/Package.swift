// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "LocalWhatsAppMCP",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "LocalWhatsAppMCP", targets: ["LocalWhatsAppMCP"])],
    targets: [
        .target(name: "LocalWhatsAppCore"),
        .executableTarget(name: "LocalWhatsAppMCP", dependencies: ["LocalWhatsAppCore"]),
        .testTarget(name: "LocalWhatsAppCoreTests", dependencies: ["LocalWhatsAppCore"])
    ]
)
