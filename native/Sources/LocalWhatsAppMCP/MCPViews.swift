import SwiftUI
import LocalWhatsAppCore

struct LocalMCPCard: View {
    @ObservedObject var model: AppModel
    var body: some View {
        Button { model.page = .mcp } label: {
            HStack(spacing: 12) {
                Image(systemName: "network").font(.system(size: 23)).foregroundStyle(Theme.secondary)
                    .frame(width: 32, height: 32)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Local MCP").font(Theme.font(16, weight: .bold))
                    Text(model.mcpSummary.statusText).font(Theme.font(11)).foregroundStyle(Theme.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Spacer(minLength: 6)
                Image(systemName: "chevron.right").font(.system(size: 12, weight: .semibold)).foregroundStyle(Theme.secondary)
            }.padding(12).modifier(RowCardSurface(emphasized: true))
        }.buttonStyle(.plain).accessibilityLabel("Local MCP, \(model.mcpSummary.statusText). Manage agent connections.")
    }
}

struct MCPConnectionsView: View {
    @ObservedObject var model: AppModel
    @State private var agentToRemove: AgentConnection?
    @State private var confirmRemoval = false

    var body: some View {
        Group {
            if model.agents.isEmpty {
                AgentPairingView(model: model)
            } else {
                connections
            }
        }
        .alert("Delete this connection?", isPresented: $confirmRemoval, presenting: agentToRemove) { agent in
            Button("Cancel", role: .cancel) {}
            Button("Delete", role: .destructive) { model.removeAgent(agent) }
        } message: { agent in
            Text("\(agent.name) will no longer be able to read or send messages.")
        }
    }

    private var connections: some View {
        VStack(spacing: 0) {
            MCPHeading(title: "Local MCP", subtitle: "Manage your agents’ access to WhatsApp")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(model.agents) { agent in connectionCard(agent) }
                    if !model.mcpSummary.isReady { MCPNotice(text: model.mcpSummary.statusText) }
                    if let notice = model.pairingNotice { MCPNotice(text: notice) }
                    MCPError(model: model)
                }.padding(.bottom, 12)
            }.scrollIndicators(.hidden)
            Spacer(minLength: 16)
            Button("Add new connection", action: model.beginPairing)
                .buttonStyle(PrimaryButtonStyle()).disabled(!model.canCreateConnection)
        }
    }

    private func connectionCard(_ agent: AgentConnection) -> some View {
        let expanded = model.expandedAgentID == agent.id
        let showingCredentials = model.credentialsConnectionID == agent.id
        return VStack(spacing: 0) {
            Button {
                withAnimation(.easeInOut(duration: 0.18)) { model.toggleAgent(agent) }
            } label: {
                HStack(spacing: 12) {
                    Image(systemName: AppAssets.connectionSymbolName)
                        .font(.system(size: 23)).foregroundStyle(Theme.secondary)
                        .frame(width: 32, height: 32).accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(agent.name).font(Theme.font(16, weight: .bold)).lineLimit(2)
                        TimelineView(.periodic(from: .now, by: 60)) { context in
                            Text(AgentConnectionSummary(agent: agent).text(at: context.date))
                                .font(Theme.font(11)).foregroundStyle(Theme.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }.frame(maxWidth: .infinity, alignment: .leading)
                    Image(systemName: "chevron.right").font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(Theme.secondary).rotationEffect(.degrees(expanded ? 90 : 0))
                        .accessibilityHidden(true)
                }.padding(12).contentShape(Rectangle())
            }.buttonStyle(.plain).accessibilityLabel(agent.name)
                .accessibilityValue(expanded ? "Details expanded" : "Details collapsed")
            if expanded {
                Rectangle().fill(Theme.line).frame(height: 0.5).padding(.horizontal, 12)
                VStack(alignment: .leading, spacing: 0) {
                    CardActionRow(title: "Delete connection", subtitle: "Remove agent’s access to WhatsApp") {
                        Button("Delete") { agentToRemove = agent; confirmRemoval = true }
                            .buttonStyle(DisconnectButtonStyle(width: Theme.cardActionButtonWidth()))
                            .accessibilityLabel("Delete connection for \(agent.name)")
                            .disabled(model.busy || !model.serviceAvailable)
                    }
                    CardActionRow(title: "Allow message sending", subtitle: "Let the agent send messages for you") {
                        Toggle("Allow message sending", isOn: Binding(
                            get: { agent.sendEnabled == true },
                            set: { model.changeSending(agent, enabled: $0) }
                        )).labelsHidden().toggleStyle(.switch).tint(Theme.green)
                            .accessibilityLabel("Allow message sending for \(agent.name)")
                            .disabled(model.busy || !model.serviceAvailable)
                    }
                    CardActionRow(title: "Credentials", subtitle: AgentConnectionSummary(agent: agent).lastUsedText) {
                        Button(showingCredentials ? "Hide" : "Show") { model.toggleCredentials(agent) }
                            .buttonStyle(QuietButtonStyle(width: Theme.cardActionButtonWidth()))
                            .accessibilityLabel("\(showingCredentials ? "Hide" : "Show") credentials for \(agent.name)")
                            .disabled(model.busy || (!model.serviceAvailable && !showingCredentials))
                    }
                    if showingCredentials {
                        if model.loadingCredentials {
                            ProgressView("Loading credentials…").font(Theme.font(12))
                                .frame(maxWidth: .infinity).padding(.bottom, 14)
                        } else if let credentials = model.connectionCredentials, credentials.id == agent.id {
                            MCPCredentialsRows(
                                credentials: credentials,
                                urlCopied: model.credentialsURLCopied,
                                tokenCopied: model.accessTokenCopied,
                                copyURL: model.copyCredentialsURL,
                                copyToken: model.copyAccessToken
                            ).padding(.horizontal, 14).padding(.bottom, 14)
                        }
                    }
                }
            }
        }.modifier(RowCardSurface(emphasized: true))
    }
}

struct AgentPairingView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            MCPHeading(title: "Connect your agent", subtitle: "Give your agent access to WhatsApp chats")
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    VStack(alignment: .leading, spacing: 13) {
                        setupStep(1, "Open your AI agent on this Mac.")
                        setupStep(2, "Copy the instructions and paste them into your agent’s chat.")
                        setupStep(3, "Send the message and follow any setup steps your agent provides.")
                    }.padding(16).frame(maxWidth: .infinity, alignment: .leading)
                        .modifier(RowCardSurface(emphasized: true))
                        .padding(.bottom, 32)
                    Button(action: model.copyInstructions) {
                        HStack(spacing: 8) {
                            if model.busy && !model.loadingCredentials {
                                ProgressView().controlSize(.small).tint(Theme.background)
                            }
                            Text(model.instructionsCopied ? "Instructions copied" : "Copy instructions")
                            if model.instructionsCopied { Image(systemName: "checkmark") }
                        }
                    }.buttonStyle(PrimaryButtonStyle()).disabled(!model.canCreateConnection)
                    if model.instructionsCopied {
                        Text("Instructions copied. Paste them into your agent’s chat within 10 minutes.")
                            .font(Theme.font(12)).foregroundStyle(Theme.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    Button("Do it manually", action: model.toggleManualSetup)
                        .buttonStyle(.plain).font(Theme.font(13)).foregroundStyle(Theme.secondary)
                        .frame(maxWidth: .infinity)
                        .accessibilityValue(model.showingManualSetup ? "Expanded" : "Collapsed")
                        .disabled(model.busy || (!model.canCreateConnection && !model.showingManualSetup))
                    if model.showingManualSetup {
                        Rectangle().fill(Theme.line).frame(height: 0.5)
                        Text("Add this URL and token to your agent’s MCP settings.")
                            .font(Theme.font(12)).foregroundStyle(Theme.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                            .frame(maxWidth: .infinity, alignment: .leading)
                        if model.loadingCredentials {
                            ProgressView("Creating connection…").font(Theme.font(12))
                                .frame(maxWidth: .infinity).padding(.vertical, 16)
                        } else if let credentials = model.connectionCredentials {
                            MCPCredentialsRows(
                                credentials: credentials,
                                urlCopied: model.credentialsURLCopied,
                                tokenCopied: model.accessTokenCopied,
                                copyURL: model.copyCredentialsURL,
                                copyToken: model.copyAccessToken
                            )
                        }
                    }
                    if let notice = model.pairingNotice { MCPNotice(text: notice) }
                    if !model.mcpSummary.isReady { MCPNotice(text: model.mcpSummary.statusText) }
                    MCPError(model: model)
                }.padding(.bottom, 12)
            }.scrollIndicators(.hidden)
            Spacer(minLength: 16)
            if model.showingManualSetup {
                Button("Done", action: model.finishConnectionSetup)
                    .buttonStyle(PrimaryButtonStyle()).disabled(model.busy)
            }
        }
    }

    private func setupStep(_ number: Int, _ text: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Text("\(number)").font(Theme.font(13, weight: .semibold))
                .frame(width: 24, height: 24).background(Color.white.opacity(0.08)).clipShape(Circle())
            Text(text).font(Theme.font(13)).fixedSize(horizontal: false, vertical: true).padding(.top, 1)
        }
    }
}

struct MCPCredentialsRows: View {
    let credentials: ConnectionCredentials
    let urlCopied: Bool
    let tokenCopied: Bool
    let copyURL: () -> Void
    let copyToken: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            credentialValue("URL", value: credentials.url, copied: urlCopied, action: copyURL)
            credentialValue("Token", value: credentials.accessToken, copied: tokenCopied, action: copyToken)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }

    private func credentialValue(_ title: String, value: String, copied: Bool, action: @escaping () -> Void) -> some View {
        HStack(spacing: 8) {
            Text(title).font(Theme.font(12, weight: .semibold))
                .frame(width: 38, alignment: .leading)
            Text(value).font(.system(size: 11, design: .monospaced)).foregroundStyle(Theme.secondary)
                .lineLimit(1).truncationMode(.middle).textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button(action: action) {
                Image(systemName: copied ? "checkmark" : "doc.on.doc")
            }.buttonStyle(IconButtonStyle())
                .accessibilityLabel(copied ? "\(title) copied" : "Copy \(title)")
                .help(copied ? "Copied" : "Copy \(title)")
        }
    }
}

private struct MCPHeading: View {
    let title: String
    let subtitle: String
    var body: some View {
        VStack(spacing: 9) {
            Text(title).font(Theme.heading()).tracking(-0.9).accessibilityAddTraits(.isHeader)
            Text(subtitle).font(Theme.font(14)).foregroundStyle(Theme.secondary)
                .multilineTextAlignment(.center).fixedSize(horizontal: false, vertical: true)
        }.frame(maxWidth: .infinity).padding(.top, 13).padding(.bottom, 24)
    }
}

private struct MCPNotice: View {
    let text: String
    var body: some View {
        Text(text).font(Theme.font(12)).foregroundStyle(Theme.secondary).fixedSize(horizontal: false, vertical: true)
    }
}

private struct MCPError: View {
    @ObservedObject var model: AppModel
    var body: some View {
        if let message = model.error ?? model.state.error {
            Label(message, systemImage: "exclamationmark.circle").font(Theme.font(12)).foregroundStyle(Theme.red)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}
