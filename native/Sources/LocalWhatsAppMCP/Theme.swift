import AppKit
import SwiftUI

enum Theme {
    static let background = color(0x17161C)
    static let gradient = LinearGradient(colors: [color(0x1B1E26), color(0x16151B), color(0x241D2B)], startPoint: .topTrailing, endPoint: .bottomLeading)
    static let panel = color(0x2A282F)
    static let line = Color.white.opacity(0.13)
    static let text = color(0xFAF9FC)
    static let secondary = color(0xA3A0AD)
    static let green = color(0x25D366)
    static let red = color(0xFF565E)
    static func font(_ size: CGFloat = 14, weight: NSFont.Weight = .regular) -> Font {
        Font(NSFont.systemFont(ofSize: size, weight: weight))
    }
    static func heading(_ size: CGFloat = 30) -> Font {
        font(size, weight: .semibold)
    }
    static func cardActionButtonWidth(label: String = "Disconnect", showsProgress: Bool = false) -> CGFloat {
        let font = NSFont.systemFont(ofSize: 13, weight: .semibold)
        let labelWidth = (label as NSString).size(withAttributes: [.font: font]).width
        return ceil(labelWidth) + 24 + (showsProgress ? 20 : 0)
    }
    static func color(_ value: UInt32) -> Color {
        Color(red: Double((value >> 16) & 255) / 255, green: Double((value >> 8) & 255) / 255, blue: Double(value & 255) / 255)
    }
}

struct CardActionRow<Control: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder let control: () -> Control

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 5) {
                Text(title).font(Theme.font(13, weight: .semibold))
                Text(subtitle).font(Theme.font(11)).foregroundStyle(Theme.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }.frame(maxWidth: .infinity, alignment: .leading)
            control()
        }.padding(14).frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct RowCardSurface: ViewModifier {
    var emphasized: Bool
    func body(content: Content) -> some View {
        content
            .background(Color.white.opacity(emphasized ? 0.07 : 0.025))
            .clipShape(RoundedRectangle(cornerRadius: 17))
            .overlay(RoundedRectangle(cornerRadius: 17).stroke(emphasized ? Theme.line : .clear, lineWidth: 0.7))
            .contentShape(RoundedRectangle(cornerRadius: 17))
    }
}

struct SoftCard<Content: View>: View {
    @ViewBuilder let content: () -> Content
    var body: some View {
        content()
            .background(Theme.panel)
            .clipShape(RoundedRectangle(cornerRadius: 26, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: 26, style: .continuous)
                    .stroke(LinearGradient(colors: [.white.opacity(0.4), .white.opacity(0.065)], startPoint: .topLeading, endPoint: .bottomTrailing), lineWidth: 0.8)
            }
    }
}

struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(15, weight: .semibold))
            .foregroundStyle(enabled ? Theme.background : Theme.text.opacity(0.38))
            .frame(maxWidth: .infinity).frame(minHeight: 50)
            .background(enabled ? Theme.text.opacity(configuration.isPressed ? 0.82 : 1) : Color.white.opacity(0.2))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .contentShape(RoundedRectangle(cornerRadius: 14))
            .scaleEffect(configuration.isPressed && enabled ? 0.99 : 1)
    }
}

struct QuietButtonStyle: ButtonStyle {
    var width: CGFloat? = nil
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(13, weight: .semibold))
            .foregroundStyle(Theme.text)
            .padding(.horizontal, 14).padding(.vertical, 9)
            .frame(width: width)
            .background(Color.white.opacity(configuration.isPressed ? 0.16 : 0.085))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

struct IconButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 15, weight: .regular))
            .foregroundStyle(Theme.secondary)
            .frame(width: 30, height: 30)
            .background(Color.white.opacity(configuration.isPressed ? 0.08 : 0))
            .clipShape(Circle())
    }
}

struct DisconnectButtonStyle: ButtonStyle {
    var width: CGFloat? = nil
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(Theme.font(13, weight: .semibold))
            .foregroundStyle(Theme.red.opacity(enabled ? 1 : 0.5))
            .padding(.horizontal, 12).padding(.vertical, 9)
            .frame(width: width)
            .background(Theme.red.opacity(configuration.isPressed ? 0.2 : 0.12))
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
    }
}
