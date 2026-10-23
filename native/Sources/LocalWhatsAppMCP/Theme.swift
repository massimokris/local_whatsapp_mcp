import AppKit
import SwiftUI

enum Theme {
    static let background = adaptive(0xFAFAFA, 0x0F0F0F)
    static let panel = adaptive(0xEEEEEE, 0x1A1A1A)
    static let selected = adaptive(0xF3F3F3, 0x131313)
    static let line = adaptive(0xDDDDDD, 0x242424)
    static let text = adaptive(0x151515, 0xF5F5F5)
    static let secondary = adaptive(0x737373, 0x808080)
    static let green = adaptive(0x16852C, 0x28CD41)
    static let red = adaptive(0xD72C22, 0xFF3B30)
    static let amber = adaptive(0xA06500, 0xFFB000)
    static func font(_ size: CGFloat = 12, weight: NSFont.Weight = .regular) -> Font {
        Font(NSFont.monospacedSystemFont(ofSize: size, weight: weight))
    }
    static func status(_ value: String?) -> Color {
        switch value {
        case "ready", "connected", "syncing": return green
        case "error", "failed", "disconnected": return red
        default: return amber
        }
    }
    private static func adaptive(_ light: UInt32, _ dark: UInt32) -> Color {
        Color(nsColor: NSColor(name: nil) { appearance in
            let rgb = appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua ? dark : light
            return NSColor(srgbRed: CGFloat((rgb >> 16) & 255) / 255, green: CGFloat((rgb >> 8) & 255) / 255, blue: CGFloat(rgb & 255) / 255, alpha: 1)
        })
    }
}
struct Hairline: View { var body: some View { Rectangle().fill(Theme.line).frame(height: 0.5) } }
struct Eyebrow: View {
    let text: String
    var body: some View { Text(text.uppercased()).font(Theme.font(10)).foregroundStyle(Theme.secondary) }
}
struct PanelButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.contentShape(Rectangle())
            .background(configuration.isPressed ? Theme.line : Color.clear)
            .onHover { inside in if inside { NSCursor.pointingHand.push() } else { NSCursor.pop() } }
    }
}
struct FooterButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View { FooterSurface(label: configuration.label, pressed: configuration.isPressed) }
}
private struct FooterSurface<Label: View>: View {
    let label: Label
    let pressed: Bool
    @State private var hovered = false
    @Environment(\.isEnabled) var enabled
    var body: some View {
        label.foregroundStyle((hovered || pressed) && enabled ? Color.black : Theme.text)
            .background((hovered || pressed) && enabled ? Color.white : Color.clear)
            .onHover { hovered = $0 }
    }
}
struct ActionRow: View {
    let title: String
    var symbol = "arrow.right"
    var primary = false
    let action: () -> Void
    var body: some View {
        Button(action: action) {
            HStack(spacing: 12) { Text(title).font(Theme.font(weight: primary ? .bold : .regular)); Spacer(); Image(systemName: symbol).font(.system(size: 13)) }
                .padding(.horizontal, 16).frame(minHeight: 44)
                .background(primary ? Theme.panel : Color.clear)
        }.buttonStyle(PanelButtonStyle())
    }
}
