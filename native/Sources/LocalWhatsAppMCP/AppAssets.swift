import AppKit

enum AppAssets {
    // Keep a terminal icon on systems that do not include apple.terminal.
    static let connectionSymbolName = NSImage(systemSymbolName: "apple.terminal", accessibilityDescription: nil) != nil
        ? "apple.terminal" : "terminal"

    static let whatsAppLogo: NSImage = {
        guard let url = bundle.url(forResource: "WhatsAppLogo", withExtension: "png"),
              let image = NSImage(contentsOf: url), image.isValid else {
            preconditionFailure("The WhatsApp logo is missing or damaged. Reinstall the complete application.")
        }
        return image
    }()

    static let bundle: Bundle = {
        let resourceBundleName = "LocalWhatsAppMCP_LocalWhatsAppMCP.bundle"
        // Installed apps keep resources in Contents/Resources. SwiftPM places the
        // same resource bundle beside its executable during local development.
        let candidates = [
            Bundle.main.resourceURL?.appendingPathComponent(resourceBundleName),
            Bundle.main.executableURL?.deletingLastPathComponent().appendingPathComponent(resourceBundleName)
        ]
        for candidate in candidates.compactMap({ $0 }) {
            if let bundle = Bundle(url: candidate) { return bundle }
        }
        preconditionFailure("The app's resource bundle is missing. Reinstall the complete application.")
    }()
}
