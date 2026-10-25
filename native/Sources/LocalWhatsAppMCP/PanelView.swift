import AppKit
import SwiftUI
import LocalWhatsAppCore
import CoreImage.CIFilterBuiltins

struct PanelView: View {
    @ObservedObject var model: AppModel
    let quit: () -> Void
    @State private var sourceExpanded = false
    @State private var confirmDisconnect = false
    @State private var qrHovered = false
    @FocusState private var searchFocused: Bool
    @FocusState private var qrRefreshFocused: Bool

    var body: some View {
        GeometryReader { geometry in
            VStack(spacing: 0) {
                topControls
                switch model.page {
                case .home: home
                case .qr: connection
                case .chats: chats
                case .mcp: MCPConnectionsView(model: model)
                case .pairing: AgentPairingView(model: model)
                case .settings: settings
                }
            }
            .padding(.horizontal, 28)
            .padding(.bottom, bottomInset(for: geometry.size.height))
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .font(Theme.font())
        .foregroundStyle(Theme.text)
        .background(Theme.gradient)
        .preferredColorScheme(.dark)
        .onAppear {
            if model.whatsappLinked || model.whatsappDisconnectPending || (model.isDemo && ProcessInfo.processInfo.environment["LWMCP_DEMO_EXPANDED"] == "1") { sourceExpanded = true }
            if model.isDemo && ProcessInfo.processInfo.environment["LWMCP_DEMO_EXPANDED"] == "0" { sourceExpanded = false }
            if model.isDemo && ProcessInfo.processInfo.environment["LWMCP_DEMO_PAGE"] == "qr-hover" { qrHovered = true }
        }
        .onChange(of: model.whatsappDisconnectPending) { pending in
            if pending { sourceExpanded = true }
        }
        .onChange(of: model.whatsappLinked) { linked in
            if linked { sourceExpanded = true }
        }
        .alert("Disconnect WhatsApp?", isPresented: $confirmDisconnect) {
            Button("Cancel", role: .cancel) {}
            Button("Disconnect", role: .destructive, action: model.disconnectWhatsApp)
        } message: {
            Text("This Mac will sign out of WhatsApp and stop syncing. Your saved messages stay locally on this Mac.")
        }
    }
    private var topControls: some View {
        ZStack(alignment: .trailing) {
            if model.isDemo {
                Text("PREVIEW").font(Theme.font(9, weight: .medium)).tracking(1.2).foregroundStyle(Theme.secondary.opacity(0.55))
                    .frame(maxWidth: .infinity)
                    .help("Sample data. No account access.")
                    .accessibilityLabel("Preview with sample data. No account access.")
            }
            if model.page == .home {
                Button { model.page = .settings } label: { Image(systemName: "gearshape") }
                    .buttonStyle(IconButtonStyle()).accessibilityLabel("Settings").help("Settings")
            } else {
                Button {
                    if model.page == .pairing { model.finishConnectionSetup() }
                    else { model.page = .home }
                } label: { Image(systemName: "xmark") }
                    .buttonStyle(IconButtonStyle())
                    .accessibilityLabel(model.page == .pairing ? "Back to Local MCP" : "Back to WhatsApp")
                    .help(model.page == .pairing ? "Back to Local MCP" : "Back to WhatsApp")
            }
        }.frame(maxWidth: .infinity, alignment: .trailing).frame(height: 30)
            .padding(.horizontal, -14)
            .frame(height: 56, alignment: .top).padding(.top, 8)
    }
    private func heading(_ title: String, subtitle: String) -> some View {
        VStack(spacing: 9) {
            Text(title).font(Theme.heading()).tracking(-0.9)
                .frame(maxWidth: .infinity)
                .accessibilityAddTraits(.isHeader)
            Text(subtitle).font(Theme.font(14)).foregroundStyle(Theme.secondary)
                .multilineTextAlignment(.center).fixedSize(horizontal: false, vertical: true)
        }
    }
    private var homeHasFooterActions: Bool {
        !model.whatsappDisconnectPending && !model.whatsappDisconnecting &&
            (model.whatsappNeedsReconnect || !model.whatsappLinked || !model.state.scope.isConfirmed)
    }
    private func bottomInset(for height: CGFloat) -> CGFloat {
        if model.page == .qr || (model.page == .home && !homeHasFooterActions) { return 24 }
        return height < 700 ? 30 : 50
    }
    private var home: some View {
        VStack(spacing: 0) {
            sourceCard.padding(.top, 24)
            LocalMCPCard(model: model).padding(.top, 12)
            errorNotice.padding(.top, 14)
            Spacer(minLength: 24)
            localNote.padding(.bottom, homeHasFooterActions ? 26 : 0)
            if homeHasFooterActions {
                if model.whatsappNeedsReconnect {
                    primary("Reconnect WhatsApp", action: model.showConnection)
                } else if model.whatsappLinked {
                    if !model.state.scope.isConfirmed {
                        primary("Choose chats", action: model.editChats)
                    }
                } else {
                    primary(model.state.whatsapp.qr == nil ? "Connect WhatsApp" : "Continue connecting", action: model.showConnection)
                }
            }
        }
    }
    private var sourceCard: some View {
        VStack(spacing: 0) {
            Button { withAnimation(.easeInOut(duration: 0.18)) { sourceExpanded.toggle() } } label: {
                HStack(spacing: 12) {
                    WhatsAppMark().frame(width: 32, height: 32)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("WhatsApp").font(Theme.font(16, weight: .bold))
                        Text(model.homeSubtitle).font(Theme.font(11)).foregroundStyle(Theme.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Spacer(minLength: 6)
                    if !sourceExpanded {
                        Image(systemName: "chevron.right").font(.system(size: 12, weight: .semibold))
                            .foregroundStyle(Theme.secondary).accessibilityHidden(true)
                    }
                }.padding(12).contentShape(Rectangle())
            }.buttonStyle(.plain).accessibilityLabel("WhatsApp, \(model.homeSubtitle)")
                .accessibilityValue(sourceExpanded ? "Details expanded" : "Details collapsed")
            if sourceExpanded {
                Rectangle().fill(Theme.line).frame(height: 0.5).padding(.horizontal, 12)
                CardActionRow(
                    title: model.whatsappLinked ? "Stop syncing WhatsApp" : "Connect WhatsApp",
                    subtitle: model.whatsappLinked ? "Signs this Mac out of WhatsApp." : "Scan a QR code from your phone to link this Mac"
                ) {
                    if model.whatsappLinked || model.whatsappDisconnecting {
                        Button { confirmDisconnect = true } label: {
                            HStack(spacing: 5) {
                                if model.whatsappDisconnecting { ProgressView().controlSize(.mini) }
                                Text(disconnectButtonTitle)
                            }
                        }.buttonStyle(DisconnectButtonStyle(width: sourceActionButtonWidth)).disabled(model.busy || model.whatsappDisconnecting)
                    } else {
                        Button("Connect", action: model.showConnection).buttonStyle(QuietButtonStyle()).disabled(model.busy)
                    }
                }
                if model.state.scope.isConfirmed && model.whatsappLinked &&
                    !model.whatsappDisconnecting && !model.whatsappDisconnectPending {
                    CardActionRow(title: "Choose connected chats", subtitle: model.connectedChatsSummary) {
                        Button("Choose", action: model.editChats).buttonStyle(QuietButtonStyle(width: sourceActionButtonWidth)).disabled(model.busy)
                            .accessibilityLabel("Choose connected chats")
                    }
                }
            }
        }.modifier(RowCardSurface(emphasized: true))
    }
    private var disconnectButtonTitle: String {
        model.whatsappDisconnecting ? "Disconnecting…" : (model.whatsappDisconnectPending ? "Retry disconnect" : "Disconnect")
    }
    private var sourceActionButtonWidth: CGFloat {
        Theme.cardActionButtonWidth(label: disconnectButtonTitle, showsProgress: model.whatsappDisconnecting)
    }
    private var connection: some View {
        VStack(spacing: 0) {
            heading(model.whatsappConnected ? "WhatsApp connected" : "Connect WhatsApp", subtitle: model.whatsappConnected ? "Your messages can now sync to this Mac." : "Scan the QR code with your phone")
                .padding(.top, 13).padding(.bottom, 24)
            ScrollView {
                SoftCard {
                    VStack(alignment: .leading, spacing: 13) {
                        if !model.whatsappConnected {
                            instruction(1, "Open WhatsApp on your phone")
                            instruction(2, "Tap Settings (iPhone) or Menu ⋮ (Android)")
                            instruction(3, "Tap “Linked Devices”, then “Link a Device”")
                            instruction(4, "Scan the QR code below")
                        }
                        qrTile.frame(maxWidth: .infinity).padding(.top, 3)
                        Text(model.whatsappConnected ? "You’re connected" : "Code refreshes automatically")
                            .font(Theme.font(10)).foregroundStyle(Theme.secondary)
                            .frame(maxWidth: .infinity).padding(.top, 3)
                    }.padding(16)
                }
                errorNotice.padding(.top, 12)
            }.scrollIndicators(.hidden)
            Spacer(minLength: 24)
            localNote
        }
    }
    private func instruction(_ number: Int, _ text: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Text("\(number)").font(Theme.font(13, weight: .semibold))
                .frame(width: 24, height: 24).background(Color.white.opacity(0.08)).clipShape(Circle())
            Text(text).font(Theme.font(13)).fixedSize(horizontal: false, vertical: true).padding(.top, 1)
        }
    }
    private var qrTile: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 18).fill(.white)
            if model.connectionError != nil {
                VStack(spacing: 12) {
                    Button(action: model.connectWhatsApp) { Image(systemName: "arrow.clockwise").font(.system(size: 28)) }
                        .buttonStyle(.plain).accessibilityLabel("Retry WhatsApp connection").disabled(model.busy)
                    Text("Try again").font(Theme.font(12))
                }.foregroundStyle(Theme.background)
            } else if model.whatsappConnected {
                Image(systemName: "checkmark.circle.fill").font(.system(size: 52, weight: .light)).foregroundStyle(Theme.green)
                    .accessibilityLabel("WhatsApp connected")
            } else if model.busy {
                qrLoading
            } else if let payload = model.state.whatsapp.qr, !payload.isEmpty {
                Button(action: model.connectWhatsApp) {
                    ZStack {
                        QRCode(payload: payload, obscured: qrHovered || qrRefreshFocused).padding(24)
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 32, weight: .regular)).foregroundStyle(Theme.background)
                            .opacity(qrHovered || qrRefreshFocused ? 1 : 0)
                    }.frame(width: 220, height: 220)
                        .contentShape(RoundedRectangle(cornerRadius: 18))
                }.buttonStyle(.plain).focused($qrRefreshFocused)
                    .onHover { hovering in
                        withAnimation(.easeInOut(duration: 0.15)) { qrHovered = hovering }
                    }
                    .accessibilityLabel("Refresh QR code")
                    .accessibilityHint("Scan the code with WhatsApp on your phone, or activate to request a new code.")
                    .help("Refresh QR code")
            } else {
                qrLoading
            }
        }.frame(width: 220, height: 220).clipShape(RoundedRectangle(cornerRadius: 18))
    }
    private var qrLoading: some View {
        VStack(spacing: 12) {
            ProgressView().controlSize(.large).environment(\.colorScheme, .light)
            Text("Getting your QR code…").font(Theme.font(12))
        }.foregroundStyle(Theme.background)
    }
    private var chats: some View {
        VStack(spacing: 0) {
            heading("Choose chats", subtitle: "New messages are saved only for selected chats.")
                .padding(.top, 13).padding(.bottom, 22)
            HStack(spacing: 9) {
                Image(systemName: "magnifyingglass").foregroundStyle(Theme.secondary)
                TextField("Search name or phone number", text: $model.selection.query)
                    .textFieldStyle(.plain).focused($searchFocused).accessibilityLabel("Search conversations")
                if !model.selection.query.isEmpty {
                    Button { model.selection.query = "" } label: { Image(systemName: "xmark.circle.fill").foregroundStyle(Theme.secondary) }
                        .buttonStyle(.plain).accessibilityLabel("Clear search")
                }
            }.padding(.horizontal, 14).frame(height: 42).background(Color.white.opacity(0.06))
                .clipShape(RoundedRectangle(cornerRadius: 12))
            HStack(spacing: 12) {
                Text(model.catalogPending ? "Loading chats…" : "\(model.selection.selectedIDs.count) selected").foregroundStyle(Theme.secondary)
                Spacer()
                Button("Select all") { model.selection.selectAllResults() }
                    .keyboardShortcut("a", modifiers: [.command, .shift]).help("Select every matching chat, including off-screen results")
                Button("Clear") { model.selection.clearResults() }.foregroundStyle(Theme.secondary)
            }.font(Theme.font(11)).buttonStyle(.plain).padding(.vertical, 15).disabled(model.catalogPending || model.busy)
            ScrollView {
                LazyVStack(spacing: 8) {
                    if model.selection.results.isEmpty {
                        Text(model.selection.chats.isEmpty ? "Your chats will appear here as WhatsApp syncs." : "No matching chats")
                            .foregroundStyle(Theme.secondary).padding(.vertical, 30).multilineTextAlignment(.center)
                    }
                    ForEach(model.selection.results) { chat in
                        let selected = model.selection.selectedIDs.contains(chat.id)
                        Button { model.selection.toggle(chat.id) } label: {
                            HStack(spacing: 12) {
                                Image(systemName: chat.kind == "group" ? "person.2.fill" : "person.fill")
                                    .font(.system(size: 14)).foregroundStyle(Theme.secondary)
                                    .frame(width: 32, height: 32).background(Color.white.opacity(0.06)).clipShape(Circle())
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(chat.displayName).font(Theme.font(13, weight: .semibold)).lineLimit(2)
                                    Text(chat.kind == "group" ? "Group" : (chat.phone == chat.displayName ? "Direct chat" : (chat.phone ?? "Direct chat")))
                                        .font(Theme.font(11)).foregroundStyle(Theme.secondary).lineLimit(1)
                                }.frame(maxWidth: .infinity, alignment: .leading)
                                ZStack {
                                    Circle().stroke(selected ? Theme.text : Theme.secondary.opacity(0.4), lineWidth: 1.2)
                                    if selected { Image(systemName: "checkmark").font(.system(size: 9, weight: .bold)) }
                                }.frame(width: 18, height: 18)
                            }.padding(12).modifier(RowCardSurface(emphasized: selected))
                        }.buttonStyle(.plain).disabled(model.catalogPending || model.busy).accessibilityLabel(chat.displayName)
                            .accessibilityValue(selected ? "Selected" : "Not selected")
                            .accessibilityAddTraits(selected ? .isSelected : [])
                    }
                }
            }.scrollIndicators(.hidden)
            errorNotice.padding(.top, 10)
            localNote.padding(.vertical, 16)
            primary("Save selection", symbol: nil) { model.saveScope(all: false) }.disabled(model.catalogPending)
            Button("Continue with all chats") { model.saveScope(all: true) }
                .buttonStyle(.plain).foregroundStyle(Theme.secondary).padding(.top, 13).disabled(model.busy)
        }.onAppear { searchFocused = true }
    }
    private var settings: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 14) {
                HStack(spacing: 16) {
                    VStack(alignment: .leading, spacing: 5) {
                        Text("Start at Log In").font(Theme.font(14, weight: .semibold))
                        Text("Open automatically when you log in to your Mac")
                            .font(Theme.font(11)).foregroundStyle(Theme.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }.frame(maxWidth: .infinity, alignment: .leading)
                    Toggle("Start at Log In", isOn: Binding(get: { model.loginEnabled }, set: model.setLogin))
                        .labelsHidden().toggleStyle(.switch).tint(Theme.green).accessibilityLabel("Start at Log In")
                }
                if let message = model.loginMessage {
                    Text(message).font(Theme.font(11)).foregroundStyle(Theme.secondary)
                    Button("Open Login Items", action: model.openLoginSettings).buttonStyle(QuietButtonStyle())
                }
            }.padding(16).frame(maxWidth: .infinity, alignment: .leading)
                .modifier(RowCardSurface(emphasized: true)).padding(.top, 24)
            errorNotice.padding(.top, 14)
            Spacer(minLength: 24)
            Text("WhatsApp and local MCP keep running when you close this window. Quitting stops both.")
                .font(Theme.font(12)).foregroundStyle(Theme.secondary).multilineTextAlignment(.center).padding(.bottom, 24)
            Button("Quit LocalWhatsAppMCP", action: quit).buttonStyle(QuietButtonStyle())
        }
    }
    private func primary(_ title: String, symbol: String? = "arrow.right", action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 8) {
                if model.busy { ProgressView().controlSize(.small).tint(Theme.background) }
                Text(title)
                if let symbol { Image(systemName: symbol).font(.system(size: 14, weight: .medium)) }
            }
        }.buttonStyle(PrimaryButtonStyle()).disabled(model.busy || model.whatsappDisconnecting)
    }
    private var localNote: some View {
        Label("Your archive stays locally on this Mac.", systemImage: "lock.fill")
            .font(Theme.font(13)).foregroundStyle(Theme.secondary)
            .help("Available history depends on WhatsApp. A connected status does not mean every past message has arrived.")
    }
    @ViewBuilder private var errorNotice: some View {
        if let error = model.connectionError {
            HStack(alignment: .top, spacing: 8) {
                Image(systemName: "exclamationmark.circle")
                Text(error).font(Theme.font(11)).fixedSize(horizontal: false, vertical: true)
            }.foregroundStyle(Theme.red).padding(12).frame(maxWidth: .infinity, alignment: .leading)
                .background(Theme.red.opacity(0.07)).clipShape(RoundedRectangle(cornerRadius: 13))
        }
    }
}

private struct WhatsAppMark: View {
    var body: some View {
        Image(nsImage: AppAssets.whatsAppLogo).resizable().aspectRatio(contentMode: .fit)
            .frame(width: 28, height: 28).clipShape(RoundedRectangle(cornerRadius: 6))
            .accessibilityHidden(true)
    }
}

private struct QRCode: View {
    let payload: String
    var obscured = false
    var body: some View {
        if let image {
            Image(nsImage: image).resizable().interpolation(.none).aspectRatio(contentMode: .fit)
                .accessibilityLabel("WhatsApp QR code. Scan with WhatsApp on your phone.")
        } else { Image(systemName: "exclamationmark.triangle").foregroundStyle(Theme.red).accessibilityLabel("QR code unavailable") }
    }
    private var image: NSImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(payload.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let raster = output.transformed(by: CGAffineTransform(scaleX: 10, y: 10))
        let displayed = obscured
            ? raster.applyingFilter("CIGaussianBlur", parameters: [kCIInputRadiusKey: raster.extent.width * 8 / 172]).cropped(to: raster.extent)
            : raster
        guard let cg = CIContext().createCGImage(displayed, from: raster.extent) else { return nil }
        return NSImage(cgImage: cg, size: NSSize(width: cg.width, height: cg.height))
    }
}
