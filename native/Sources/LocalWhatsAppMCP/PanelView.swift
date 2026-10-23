import SwiftUI
import LocalWhatsAppCore
import CoreImage.CIFilterBuiltins

struct PanelView: View {
    @ObservedObject var model: AppModel
    let quit: () -> Void
    let dismiss: () -> Void
    @FocusState private var searchFocused: Bool

    var body: some View {
        VStack(spacing: 0) {
            header
            Hairline()
            if model.isDemo {
                HStack { Image(systemName: "eye"); Text("DEMO · sample data, no account access"); Spacer() }
                    .font(Theme.font(10)).foregroundStyle(Theme.amber).padding(.horizontal, 16).frame(height: 28)
                Hairline()
            }
            if let error = model.error ?? model.state.error {
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: "exclamationmark.circle")
                    Text(error).fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: 0)
                    Button { model.error = nil } label: { Image(systemName: "xmark") }.help("Dismiss error").accessibilityLabel("Dismiss error")
                }.foregroundStyle(Theme.red).padding(12)
                Hairline()
            }
            content.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .disabled(model.busy)
            Hairline()
            footer
        }
        .font(Theme.font()).tracking(0.4).lineSpacing(3)
        .foregroundStyle(Theme.text).background(Theme.background)
        .clipShape(RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(Theme.line, lineWidth: 0.5))
        .onExitCommand(perform: dismiss)
    }
    private var header: some View {
        HStack(spacing: 10) {
            if model.page != .home {
                Button { model.page = .home } label: { Image(systemName: "chevron.left").frame(width: 24, height: 28) }
                    .buttonStyle(PanelButtonStyle()).help("Back to status").accessibilityLabel("Back to status")
            } else { Image(systemName: "bubble.left.and.bubble.right").font(.system(size: 16)) }
            Text(title).font(Theme.font(weight: .bold))
            Spacer()
            if model.busy { ProgressView().controlSize(.small) }
            Circle().fill(Theme.status(model.state.whatsapp.status)).frame(width: 6, height: 6)
                .accessibilityLabel("WhatsApp \(model.statusLabel)")
            Button(action: dismiss) { Image(systemName: "xmark").font(.system(size: 11)).frame(width: 24, height: 28) }
                .buttonStyle(PanelButtonStyle()).help("Close panel; background sync continues").accessibilityLabel("Close panel")
        }.padding(.horizontal, 16).frame(height: 48)
    }
    private var title: String {
        switch model.page { case .home: return "LocalWhatsAppMCP"; case .chats: return "Choose chats"; case .agents: return "Agent connections"; case .pairing: return "Connect an agent"; case .cloud: return "Cloud access"; case .settings: return "Settings" }
    }
    @ViewBuilder private var content: some View {
        switch model.page {
        case .home: home
        case .chats: chats
        case .agents: agents
        case .pairing: pairing
        case .cloud: cloud
        case .settings: settings
        }
    }
    private var home: some View {
        ScrollView {
            VStack(spacing: 0) {
                statusRow("WhatsApp", model.statusLabel, color: Theme.status(model.state.whatsapp.status))
                if let detail = model.state.whatsapp.detail { note(detail) }
                if let payload = model.state.whatsapp.qr {
                    QRCode(payload: payload).padding(.vertical, 12)
                    note("On your phone, open WhatsApp → Settings → Linked devices → Link a device. Scan this code.")
                    ActionRow(title: "Get a new QR code", symbol: "arrow.clockwise", action: model.connectWhatsApp)
                } else if !model.whatsappReady {
                    note("Connect your WhatsApp to import available history. Your archive stays on this Mac.")
                    ActionRow(title: "Connect WhatsApp", primary: true, action: model.connectWhatsApp)
                }
                Hairline()
                if !model.state.scope.isConfirmed {
                    VStack(alignment: .leading, spacing: 10) {
                        Eyebrow(text: "Initial capture")
                        Text("Received messages are saved while you set up. Agents have no access until you confirm chat scope.")
                        if model.whatsappReady { Text("Choose which chats to keep receiving and share with your agents.") }
                    }.padding(16).frame(maxWidth: .infinity, alignment: .leading)
                    if model.whatsappReady {
                        ActionRow(title: "Continue with all chats", primary: true) { model.saveScope(all: true) }
                        ActionRow(title: "Select specific chats", action: model.editChats)
                        note("All chats includes future conversations and groups.")
                    }
                } else {
                    statusRow("Chat scope", model.state.scope.mode == "all" ? "All chats" : "\(model.state.scope.chatIDs.count) selected")
                    ActionRow(title: "Change chat scope", symbol: "checklist", action: model.editChats)
                    Hairline()
                    statusRow("Local MCP", (model.state.mcp.status ?? "Starting").capitalized, color: Theme.status(model.state.mcp.status))
                    ActionRow(title: model.state.agents.isEmpty ? "Connect an agent" : "\(model.state.agents.count) agent connections", symbol: "link", primary: model.state.agents.isEmpty) { model.page = model.state.agents.isEmpty ? .pairing : .agents }
                    Hairline()
                    statusRow("Cloud access", model.state.cloud.enabled == true ? (model.state.cloud.status ?? "Starting").capitalized : "Off", color: model.state.cloud.enabled == true ? Theme.status(model.state.cloud.status) : Theme.secondary)
                    ActionRow(title: model.state.cloud.enabled == true ? "Manage cloud access" : "Connect cloud agent", symbol: "cloud") { model.page = .cloud }
                }
                Hairline()
                VStack(alignment: .leading, spacing: 8) {
                    Eyebrow(text: "Archive freshness")
                    if let last = model.state.whatsapp.lastEventAt { Text("Last observed event: \(formattedDate(last))") }
                    Text("Available history depends on WhatsApp. A running sync does not prove that every message has been imported.").foregroundStyle(Theme.secondary)
                }.padding(16).frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }
    private var chats: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").foregroundStyle(Theme.secondary)
                TextField("Search name or phone number", text: $model.selection.query)
                    .textFieldStyle(.plain).focused($searchFocused).accessibilityLabel("Search conversations")
                if !model.selection.query.isEmpty {
                    Button { model.selection.query = "" } label: { Image(systemName: "xmark.circle.fill") }.buttonStyle(.plain).accessibilityLabel("Clear search")
                }
            }.padding(.horizontal, 16).frame(height: 44).background(Theme.panel)
            HStack {
                Text("\(model.selection.selectedIDs.count) selected · \(model.selection.results.count) results").foregroundStyle(Theme.secondary)
                Spacer(minLength: 4)
                Button("Select all") { model.selection.selectAllResults() }.keyboardShortcut("a", modifiers: [.command, .shift])
                    .help("Select every search result, including off-screen chats")
                Button("Clear") { model.selection.clearResults() }.help("Deselect matching results")
            }.font(Theme.font(10)).padding(.horizontal, 16).frame(height: 40)
            Hairline()
            ScrollView {
                LazyVStack(spacing: 0) {
                    if model.selection.results.isEmpty { note(model.selection.chats.isEmpty ? "Waiting for the chat catalog. Available history may still be arriving." : "No chats match this search.") }
                    ForEach(model.selection.results) { chat in
                        HStack(spacing: 12) {
                            Button { model.selection.toggle(chat.id) } label: {
                                HStack(spacing: 10) {
                                    Image(systemName: chat.kind == "group" ? "person.2" : "person").foregroundStyle(Theme.secondary).frame(width: 20)
                                    VStack(alignment: .leading, spacing: 3) {
                                        Text(chat.displayName).lineLimit(2).fixedSize(horizontal: false, vertical: true)
                                        Text(chat.phone == chat.displayName ? "Direct chat" : (chat.phone ?? (chat.kind == "group" ? "Group" : "Direct chat"))).font(Theme.font(10)).foregroundStyle(Theme.secondary).lineLimit(1)
                                    }
                                    Spacer(minLength: 8)
                                }.contentShape(Rectangle())
                            }.buttonStyle(.plain).accessibilityHidden(true)
                            Toggle(chat.displayName, isOn: Binding(get: { model.selection.selectedIDs.contains(chat.id) }, set: { value in
                                if value { model.selection.selectedIDs.insert(chat.id) } else { model.selection.selectedIDs.remove(chat.id) }
                            })).labelsHidden().toggleStyle(.checkbox).accessibilityLabel(chat.displayName)
                        }.padding(.horizontal, 16).padding(.vertical, 10)
                            .frame(minHeight: 60).background(model.selection.selectedIDs.contains(chat.id) ? Theme.selected : Theme.background)
                            .help(chat.displayName)
                        Hairline()
                    }
                }
            }
            Hairline()
            note("Only selected chats will receive new content and be available to agents. Previously saved content stays on this Mac.")
            ActionRow(title: "Save \(model.selection.selectedIDs.count) selected chats", symbol: "checkmark", primary: true) { model.saveScope(all: false) }.disabled(model.catalogPending)
            ActionRow(title: "Continue with all chats") { model.saveScope(all: true) }
        }.onAppear { searchFocused = true }
    }
    private var agents: some View {
        ScrollView {
            VStack(spacing: 0) {
                note("Every connection shares your chat scope. Sending is controlled separately for each agent.")
                if model.state.agents.isEmpty { note("No agents connected yet. Create instructions and give them to your local agent.") }
                ForEach(model.state.agents) { agent in
                    VStack(alignment: .leading, spacing: 12) {
                        HStack {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(agent.name).font(Theme.font(weight: .bold))
                                Text("\(agent.client ?? "Agent") · \(agent.connected == true ? "Connected" : "Not recently active")").font(Theme.font(10)).foregroundStyle(Theme.secondary)
                            }
                            Spacer()
                            Button("Disconnect") { model.disconnect(agent) }.help("Revoke \(agent.name)'s credential")
                        }
                        HStack {
                            Text("Enable message sending")
                            Spacer()
                            Toggle("Enable message sending for \(agent.name)", isOn: Binding(get: { agent.sendEnabled == true }, set: { model.changeSending(agent, enabled: $0) })).labelsHidden().toggleStyle(.switch)
                        }
                        Text("New text and attachments, within your chat scope.").font(Theme.font(10)).foregroundStyle(Theme.secondary)
                    }.padding(16)
                    Hairline()
                }
                ActionRow(title: "Connect an agent", symbol: "plus", primary: true) { model.clearInvitation(); model.page = .pairing }
                ActionRow(title: "Connect cloud agent", symbol: "cloud") { model.clearInvitation(); model.page = .cloud }
            }
        }
    }
    private var pairing: some View {
        ScrollView {
            VStack(spacing: 0) {
                note("Give the setup instructions to your local agent. It installs its own credential and gains reading access immediately.")
                VStack(spacing: 16) {
                    LabeledContent("Connection name") { TextField("My agent", text: $model.agentName).textFieldStyle(.roundedBorder) }
                    HStack { Text("Client"); Spacer(); Picker("Client", selection: $model.agentClient) { Text("Codex").tag("codex"); Text("Claude Code").tag("claude"); Text("Other · stdio adapter").tag("stdio") }.labelsHidden().pickerStyle(.menu).frame(width: 235) }
                }.padding(16)
                Hairline()
                ActionRow(title: "Generate setup instructions", symbol: "link", primary: true) { model.makeInstructions(cloud: false) }
                    .disabled(model.agentName.trimmingCharacters(in: .whitespaces).isEmpty)
                instructions
                note("The invitation is short-lived and can be used once. Long-lived credentials are installed securely by the helper. Sending starts off.")
            }
        }
    }
    private var cloud: some View {
        ScrollView {
            VStack(spacing: 0) {
                statusRow("Tunnel", model.state.cloud.enabled == true ? (model.state.cloud.status ?? "Starting").capitalized : "Off", color: Theme.status(model.state.cloud.status))
                if let hostname = model.state.cloud.hostname, !hostname.isEmpty { note(hostname) }
                if let detail = model.state.cloud.detail, !detail.isEmpty { note(detail) }
                note("Your local agent sets up the provider and stable hostname. The app then manages the tunnel. The archive stays on this Mac.")
                note("Cloud access is unavailable when this Mac is asleep, offline, or the app is quit.")
                Hairline()
                VStack(spacing: 16) {
                    HStack { Text("Tunnel provider"); Spacer(); Picker("Tunnel provider", selection: $model.provider) { Text("ngrok").tag("ngrok"); Text("Cloudflare Tunnel").tag("cloudflared") }.labelsHidden().pickerStyle(.menu).frame(width: 235) }
                    LabeledContent("Connection name") { TextField("Cloud agent", text: $model.agentName).textFieldStyle(.roundedBorder) }
                }.padding(16)
                ActionRow(title: "Generate cloud setup instructions", symbol: "cloud", primary: true) { model.makeInstructions(cloud: true) }
                instructions
                if model.state.cloud.enabled == true { Hairline(); ActionRow(title: "Turn off cloud access", symbol: "xmark", action: model.disableCloud) }
            }
        }
    }
    private var settings: some View {
        ScrollView {
            VStack(spacing: 0) {
                HStack { Text("Appearance"); Spacer(); Picker("Appearance", selection: $model.appearance) { ForEach(AppModel.Appearance.allCases, id: \.self) { Text($0.rawValue).tag($0) } }.labelsHidden().frame(width: 150) }.padding(.horizontal, 16).frame(height: 52)
                Hairline()
                HStack { Text("Open at login"); Spacer(); Toggle("Open at login", isOn: Binding(get: { model.loginEnabled }, set: model.setLogin)).labelsHidden().toggleStyle(.switch) }.padding(.horizontal, 16).frame(height: 52)
                if let message = model.loginMessage {
                    note(message)
                    ActionRow(title: "Open Login Items", symbol: "arrow.up.right", action: model.openLoginSettings)
                }
                Hairline()
                statusRow("Chat scope", model.scopeLabel)
                ActionRow(title: "Change chat scope", symbol: "checklist", action: model.editChats)
                    .disabled(!model.state.scope.isConfirmed && !model.whatsappReady)
                Hairline()
                ActionRow(title: "Manage agent connections", symbol: "link") { model.page = .agents }.disabled(!model.state.scope.isConfirmed)
                ActionRow(title: "Manage cloud access", symbol: "cloud") { model.page = .cloud }.disabled(!model.state.scope.isConfirmed)
                Hairline()
                note("Closing this panel keeps sync running. Quit stops WhatsApp sync, the local service, and the tunnel. Normal Mac sleep is respected.")
                ActionRow(title: "Refresh connection status", symbol: "arrow.clockwise") { Task { await model.refresh() } }
                Hairline()
                note("LocalWhatsAppMCP · v1\nLocal archive and device connection on this Mac. No AI model or API key is required.")
            }
        }
    }
    @ViewBuilder private var instructions: some View {
        if let text = model.instructions {
            Hairline()
            VStack(alignment: .leading, spacing: 12) {
                HStack { Eyebrow(text: "Give this to your local agent"); Spacer(); Button("Copy", action: model.copyInstructions) }
                Text(text).font(Theme.font(11)).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                if let expiry = model.invitationExpires { Text("Expires \(formattedDate(expiry))").foregroundStyle(Theme.secondary) }
            }.padding(16).background(Theme.selected)
        }
    }
    private var footer: some View {
        HStack(spacing: 0) {
            footerButton("Status", symbol: "waveform.path", page: .home)
            footerButton("Agents", symbol: "link", page: .agents).disabled(!model.state.scope.isConfirmed)
            footerButton("Settings", symbol: "gearshape", page: .settings)
            Button(action: quit) { Image(systemName: "power").frame(width: 48, height: 44) }.buttonStyle(FooterButtonStyle()).help("Quit and stop background services").accessibilityLabel("Quit LocalWhatsAppMCP")
        }
    }
    private func footerButton(_ title: String, symbol: String, page: AppModel.Page) -> some View {
        Button { model.page = page } label: { HStack(spacing: 6) { Image(systemName: symbol); Text(title) }.frame(maxWidth: .infinity).frame(height: 44) }
            .buttonStyle(FooterButtonStyle()).accessibilityAddTraits(model.page == page ? .isSelected : [])
    }
    private func note(_ text: String) -> some View { Text(text).foregroundStyle(Theme.secondary).fixedSize(horizontal: false, vertical: true).frame(maxWidth: .infinity, alignment: .leading).padding(16) }
    private func statusRow(_ title: String, _ value: String, color: Color = Theme.text) -> some View {
        HStack(alignment: .center) { Text(title).foregroundStyle(Theme.secondary); Spacer(minLength: 12); Text(value).foregroundStyle(color).multilineTextAlignment(.trailing) }.padding(.horizontal, 16).frame(minHeight: 44)
    }
    private func formattedDate(_ value: String) -> String {
        guard let date = ISO8601DateFormatter().date(from: value) else { return value }
        return date.formatted(date: .abbreviated, time: .shortened)
    }
}
private struct QRCode: View {
    let payload: String
    var body: some View {
        if let image = image {
            Image(nsImage: image).interpolation(.none).frame(width: 232, height: 232).padding(12).background(Color.white)
                .accessibilityLabel("WhatsApp linking QR code. Scan with WhatsApp on your phone.")
        } else { Text("Unable to display this QR code. Request a new code.").foregroundStyle(Theme.red) }
    }
    private var image: NSImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(payload.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let scale = max(2, floor(232 / output.extent.width) * 2)
        let raster = output.transformed(by: CGAffineTransform(scaleX: scale, y: scale))
        guard let cg = CIContext().createCGImage(raster, from: raster.extent) else { return nil }
        return NSImage(cgImage: cg, size: NSSize(width: CGFloat(cg.width) / 2, height: CGFloat(cg.height) / 2))
    }
}
