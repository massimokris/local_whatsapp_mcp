import AppKit
import Combine
import LocalWhatsAppCore
import ServiceManagement

@MainActor
final class AppModel: ObservableObject {
    enum Page: String { case home, qr, chats, mcp, pairing, settings }
    @Published var state = ServiceState()
    @Published var page: Page = .home {
        didSet {
            pendingPanelOpen = false
            if oldValue != page {
                navigationRevision += 1
                clearInvitation()
                clearConnectionCredentials()
                showingManualSetup = false
                setupManualConnectionID = nil
                setupGeneration += 1
                expandedAgentID = nil
                pairingNotice = nil
            }
        }
    }
    @Published var selection = ChatSelection()
    @Published var busy = false
    @Published private var disconnectRequested = false
    @Published var catalogPending = false
    @Published var error: String?
    @Published var loginEnabled = false
    @Published var loginMessage: String?
    @Published private(set) var serviceAvailable = false
    @Published private(set) var invitation: PairingInvitation?
    @Published private(set) var connectionCredentials: ConnectionCredentials?
    @Published private(set) var loadingCredentials = false
    @Published private(set) var credentialsConnectionID: String?
    @Published private(set) var accessTokenCopied = false
    @Published private(set) var credentialsURLCopied = false
    @Published private(set) var showingManualSetup = false
    @Published private(set) var expandedAgentID: String?
    @Published var pairingNotice: String?
    @Published var instructionsCopied = false
    let isDemo: Bool
    let runtime: HelperRuntime
    private let pasteboard: NSPasteboard
    private var polling: Task<Void, Never>?
    private var refreshing = false
    private var stateRevision = 0
    private var stopping = false
    private var wakeRequested = false
    private var didLoad = false
    private var navigationRevision = 0
    private var pendingPanelOpen = false
    private var seedAllSelection = false
    private var lastCatalogRefresh = Date.distantPast
    private var invitationExpiry: Task<Void, Never>?
    private var invitationGeneration = 0
    private var credentialsGeneration = 0
    private var setupManualConnectionID: String?
    private var setupGeneration = 0

    init(environment: [String: String] = ProcessInfo.processInfo.environment, pasteboard: NSPasteboard = .general, runtime: HelperRuntime? = nil) {
        self.pasteboard = pasteboard
        isDemo = CommandLine.arguments.contains("--demo") || environment["LWMCP_DEMO"] == "1"
        let helpers = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers")
        let applicationSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        self.runtime = runtime ?? HelperRuntime(helper: helpers.appendingPathComponent("localwhatsappmcp"), wacli: helpers.appendingPathComponent("wacli"), dataDirectory: applicationSupport.appendingPathComponent("LocalWhatsAppMCP"))
        if isDemo { loadDemo(page: environment["LWMCP_DEMO_PAGE"]) }
    }
    var whatsappLinked: Bool { state.whatsapp.isLinked }
    var whatsappConnected: Bool { state.whatsapp.isConnected }
    var whatsappNeedsReconnect: Bool {
        !whatsappDisconnectPending && !whatsappDisconnecting && (["attention", "error", "failed"].contains(state.whatsapp.status ?? "") ||
            (state.whatsapp.linked == true && !whatsappConnected && !["connecting", "linking"].contains(state.whatsapp.status ?? "")))
    }
    var whatsappDisconnectPending: Bool { state.whatsapp.disconnectPending == true }
    var whatsappDisconnecting: Bool { disconnectRequested || state.whatsapp.status == "disconnecting" }
    var connectionError: String? {
        error ?? state.error ?? ((whatsappNeedsReconnect || (whatsappDisconnectPending && !whatsappDisconnecting)) ? state.whatsapp.detail : nil)
    }
    var statusLabel: String {
        if whatsappDisconnecting { return "Disconnecting…" }
        if !serviceAvailable && whatsappLinked { return "Reconnecting…" }
        if state.whatsapp.syncState == "error" && whatsappConnected { return "Needs attention" }
        switch state.whatsapp.status {
        case "ready", "connected": return "Connected to this Mac"
        case "syncing", "importing", "receiving_history": return "Syncing messages…"
        case "linking": return "Waiting for your phone"
        case "connecting": return whatsappLinked ? "Reconnecting…" : "Connecting…"
        case "disconnecting": return "Disconnecting…"
        case "attention", "error", "failed": return "Needs attention"
        default: return whatsappLinked ? "Connection paused" : "Not connected"
        }
    }
    var homeSubtitle: String {
        guard serviceAvailable, whatsappConnected, state.whatsapp.syncState != "error", !whatsappDisconnecting, !whatsappDisconnectPending else { return statusLabel }
        return syncSummary.text()
    }
    var connectedChatsSummary: String {
        guard let count = syncSummary.connectedChatCount else { return "Loading connected chats…" }
        return "\(count.formatted(.number.locale(Locale(identifier: "en_US")))) \(count == 1 ? "chat" : "chats") connected"
    }
    private var syncSummary: WhatsAppSyncSummary {
        let catalog = isDemo || lastCatalogRefresh != .distantPast ? selection.chats : nil
        return WhatsAppSyncSummary(whatsapp: state.whatsapp, scope: state.scope, catalog: catalog)
    }
    var mcpSummary: MCPConnectionSummary { .init(state: state, serviceAvailable: serviceAvailable) }
    var agents: [AgentConnection] {
        state.agents.sorted {
            let comparison = $0.name.localizedStandardCompare($1.name)
            return comparison == .orderedSame ? $0.id < $1.id : comparison == .orderedAscending
        }
    }
    var canCreateConnection: Bool { mcpSummary.isReady && !busy && !stopping }
    private var isConnectionPage: Bool { page == .mcp || page == .pairing }
    private var nextAgentName: String { AgentConnectionSummary.nextName(in: state.agents) }
    func panelOpened() {
        guard !isDemo, !stopping else { return }
        // Opening the panel is the user's request to link an unlinked account.
        // Resolve it from the next service response, never the default/cached state.
        pendingPanelOpen = true
        Task { await refresh() }
    }
    func panelClosed() {
        clearInvitation()
        clearConnectionCredentials()
        showingManualSetup = false
        expandedAgentID = nil
    }
    func start() {
        guard !isDemo else { return }
        configureLogin()
        polling = Task { [weak self] in
            while !Task.isCancelled {
                guard let self, !self.stopping else { return }
                await self.refresh()
                try? await Task.sleep(nanoseconds: 2_000_000_000)
            }
        }
    }
    func refresh() async {
        expireInvitation()
        guard !isDemo, !refreshing, !busy, !stopping else { return }
        refreshing = true
        let refreshRevision = stateRevision
        var receivedState = false
        defer { refreshing = false }
        do {
            try await runtime.start()
            let data = try await runtime.control(method: "GET", path: "/state")
            let latest = try JSONDecoder().decode(ServiceState.self, from: data)
            guard refreshRevision == stateRevision else { return }
            receivedState = true
            serviceAvailable = true
            let resumeInitialSetup = !didLoad && navigationRevision == 0
            state = latest
            reconcileConnections()
            if !didLoad { selection.selectedIDs = Set(latest.scope.chatIDs); didLoad = true }
            if error?.hasPrefix("Connecting to WhatsApp.") == true { error = nil }
            resolvePanelOpening(resumeInitialSetup: resumeInitialSetup)
            if page == .chats || (whatsappLinked && Date().timeIntervalSince(lastCatalogRefresh) > 15) {
                let catalog = try await runtime.control(method: "GET", path: "/catalog")
                guard refreshRevision == stateRevision else { return }
                selection.chats = try JSONDecoder().decode(Catalog.self, from: catalog).chats
                if seedAllSelection { selection.selectedIDs = Set(selection.chats.map(\.id)); seedAllSelection = false }
                catalogPending = false
                lastCatalogRefresh = Date()
            }
        } catch {
            if refreshRevision == stateRevision && !receivedState {
                serviceAvailable = false
                state.whatsapp.syncState = "unknown"
            }
            if !busy && refreshRevision == stateRevision { self.error = "Connecting to WhatsApp. \(error.localizedDescription)" }
        }
    }
    func reconcileConnections() {
        if invitation?.matchingAgent(in: state.agents) != nil {
            clearInvitation()
            if isConnectionPage { finishConnectionSetup() }
        }
        let activeIDs = Set(state.agents.map(\.id))
        if let id = credentialsConnectionID, !activeIDs.contains(id) {
            clearConnectionCredentials()
            pairingNotice = "This connection has been deleted."
        }
        if let id = setupManualConnectionID, !activeIDs.contains(id) {
            setupManualConnectionID = nil
            showingManualSetup = false
            pairingNotice = "This connection has been deleted."
        }
        if let id = expandedAgentID, !activeIDs.contains(id) { expandedAgentID = nil }
    }
    private func resolvePanelOpening(resumeInitialSetup: Bool = false) {
        let destination = whatsappDisconnecting && (pendingPanelOpen || page == .qr)
            ? WhatsAppPanelOpening.home
            : WhatsAppPanelOpening(state: state.whatsapp, currentPageIsQR: page == .qr, foregroundOpen: pendingPanelOpen)
        // A foreground open during another operation must not launch a second one.
        if destination == .qr(startConnection: true), busy { return }
        pendingPanelOpen = false
        switch destination {
        case .keepCurrentPage:
            // Resume unfinished chat selection once on launch. Later polls and
            // window reopens must respect an explicit exit or another page.
            if resumeInitialSetup, page == .home, needsInitialChatSelection { editChats() }
        case .home:
            if page == .qr, needsInitialChatSelection { editChats() }
            else { page = .home }
        case .qr(let startConnection):
            page = .qr
            if startConnection { connectWhatsApp() }
        }
    }
    private var needsInitialChatSelection: Bool {
        whatsappLinked && !state.scope.isConfirmed && !whatsappDisconnectPending && !whatsappDisconnecting
    }
    func showConnection() {
        page = .qr
        if !whatsappConnected && (state.whatsapp.qr == nil || whatsappNeedsReconnect) { connectWhatsApp() }
    }
    func connectWhatsApp() {
        page = .qr
        if isDemo {
            state.whatsapp = .init(status: "linking", qr: "LOCALWHATSAPPMCP-DEMO-NOT-A-PAIRING-CODE")
            return
        }
        mutate(method: "POST", path: "/whatsapp/connect", body: [:])
    }
    func disconnectWhatsApp() {
        guard !busy, !whatsappDisconnecting else { return }
        if isDemo {
            state.whatsapp = .init(status: "disconnected", linked: false)
            state.error = nil
            error = nil
            return
        }
        disconnectRequested = true
        mutate(method: "POST", path: "/whatsapp/disconnect", body: [:], finished: { self.disconnectRequested = false })
    }
    func editChats() {
        catalogPending = !isDemo
        seedAllSelection = state.scope.mode == "all" && !isDemo
        selection.selectedIDs = state.scope.mode == "all" ? Set(selection.chats.map(\.id)) : Set(state.scope.chatIDs)
        selection.query = ""
        page = .chats
        Task { await refresh() }
    }
    func saveScope(all: Bool) {
        guard !busy else { return }
        let initialConfirmation = !state.scope.isConfirmed
        let navigation = navigationRevision
        let continueSetup = {
            // Saving may finish after the user leaves, including leaving and
            // reopening the selector. Persist scope without replacing that route.
            guard !self.stopping, self.navigationRevision == navigation, self.state.scope.isConfirmed else { return }
            self.page = initialConfirmation ? .mcp : .home
        }
        if isDemo {
            state.scope = Scope(mode: all ? "all" : "selected", chatIDs: all ? [] : selection.selectedIDs.sorted())
            if state.mcp.status == "setup_required" { state.mcp.status = "ready" }
            continueSetup()
            return
        }
        mutate(method: "PUT", path: "/scope", body: ["mode": all ? "all" : "selected", "chat_ids": all ? [] : selection.selectedIDs.sorted()], completion: continueSetup)
    }
    func wake() {
        guard !isDemo, !stopping else { return }
        if busy {
            stateRevision += 1
            state.whatsapp.syncState = "unknown"
            wakeRequested = true
            return
        }
        mutate(method: "POST", path: "/lifecycle/wake", body: [:])
    }
    private func resumeQueuedWake() {
        guard wakeRequested else { return }
        wakeRequested = false
        wake()
    }
    func beginPairing() {
        setupGeneration += 1
        page = .pairing
        clearInvitation()
        clearConnectionCredentials()
        showingManualSetup = false
        setupManualConnectionID = nil
        pairingNotice = nil
        error = nil
    }
    func finishConnectionSetup() {
        setupGeneration += 1
        clearInvitation()
        clearConnectionCredentials()
        showingManualSetup = false
        setupManualConnectionID = nil
        pairingNotice = nil
        page = .mcp
    }
    func copyInstructions() {
        expireInvitation()
        guard canCreateConnection, isConnectionPage else { return }
        if let invitation {
            copyInvitation(invitation)
            return
        }
        pairingNotice = nil
        error = nil
        let name = nextAgentName
        let knownIDs = Set(state.agents.map(\.id))
        let generation = invitationGeneration
        if isDemo {
            let expiresAt = ISO8601DateFormatter().string(from: Date().addingTimeInterval(600))
            let instructions = """
            Connect yourself to the LocalWhatsAppMCP server running on this Mac at http://127.0.0.1:32191/mcp using Streamable HTTP.


            Exchange this single-use pairing code by sending POST http://127.0.0.1:32191/pairing/redeem with Content-Type: application/json and this JSON body: {"token":"PREVIEW-ONLY-CODE"}


            The response contains id, url, access_token, and token_type (Bearer). Save access_token privately without printing it in the conversation or logs, and configure your own MCP client to use the returned url with Authorization: Bearer <access_token>.

            Reload your MCP connection if needed and call check_status to verify it. This pairing code expires at \(expiresAt). Sending starts disabled; the app controls sending permission and chat scope.

            After check_status succeeds, give the user a short overview of what you can do with their connected WhatsApp chats and one or two example requests.

            This is a preview; no credential was created.
            """
            showInvitation(instructions: instructions, expiresAt: expiresAt, invitationID: UUID().uuidString, knownIDs: knownIDs)
            if let invitation { copyInvitation(invitation) }
            return
        }
        busy = true
        Task {
            defer { busy = false; resumeQueuedWake() }
            do {
                let body = try JSONSerialization.data(withJSONObject: ["name": name])
                let data = try await runtime.control(method: "POST", path: "/pairing", body: body)
                let result = try JSONDecoder().decode(SetupInstructions.self, from: data)
                guard isConnectionPage, !stopping, generation == invitationGeneration else { return }
                showInvitation(instructions: result.instructions, expiresAt: result.expiresAt, invitationID: result.invitationID ?? "", knownIDs: knownIDs)
                if let invitation { copyInvitation(invitation) }
            } catch {
                if isConnectionPage, generation == invitationGeneration { self.error = error.localizedDescription }
            }
        }
    }
    private func copyInvitation(_ invitation: PairingInvitation) {
        pasteboard.clearContents()
        pasteboard.setString(invitation.instructions, forType: .string)
        instructionsCopied = true
    }
    private func showInvitation(instructions: String, expiresAt: String?, invitationID: String, knownIDs: Set<String>) {
        guard let pending = PairingInvitation(instructions: instructions, expiresAt: expiresAt, invitationID: invitationID, knownAgentIDs: knownIDs), !pending.isExpired(at: Date()) else {
            error = "Setup instructions could not be prepared. Please try again."
            return
        }
        invitation = pending
        invitationExpiry = Task { [weak self] in
            do { try await Task.sleep(nanoseconds: UInt64(min(3600, max(0, pending.expiresAt.timeIntervalSinceNow)) * 1_000_000_000)) }
            catch { return }
            self?.expireInvitation()
        }
    }
    private func expireInvitation() {
        guard let invitation, invitation.isExpired(at: Date()) else { return }
        clearInvitation()
        pairingNotice = "Instructions expired. Copy new instructions to connect your agent."
    }
    func clearInvitation() {
        invitationGeneration += 1
        invitationExpiry?.cancel()
        invitationExpiry = nil
        invitation = nil
        instructionsCopied = false
    }
    func toggleManualSetup() {
        guard !busy, isConnectionPage else { return }
        if showingManualSetup {
            showingManualSetup = false
            clearConnectionCredentials()
            return
        }
        guard canCreateConnection else { return }
        // Keep the setup surface visible after manual creation adds a saved grant.
        if page != .pairing { page = .pairing }
        showingManualSetup = true
        clearInvitation()
        loadCredentials(existingID: setupManualConnectionID)
    }
    func toggleAgent(_ agent: AgentConnection) {
        clearConnectionCredentials()
        expandedAgentID = expandedAgentID == agent.id ? nil : agent.id
    }
    func toggleCredentials(_ agent: AgentConnection) {
        if credentialsConnectionID == agent.id {
            clearConnectionCredentials()
            return
        }
        guard !busy, serviceAvailable, !stopping else { return }
        loadCredentials(existingID: agent.id)
    }
    private func loadCredentials(existingID: String?) {
        guard !busy, !stopping, serviceAvailable,
              existingID != nil || canCreateConnection else { return }
        clearConnectionCredentials()
        pairingNotice = nil
        error = nil
        credentialsConnectionID = existingID
        let generation = credentialsGeneration
        let setup = setupGeneration
        let name = existingID.flatMap { id in state.agents.first(where: { $0.id == id })?.name } ?? nextAgentName
        if isDemo {
            let id = existingID ?? "demo-agent-\(UUID().uuidString)"
            connectionCredentials = .init(id: id, name: name, url: "http://127.0.0.1:32191/mcp", accessToken: "PREVIEW-ONLY-NOT-A-REAL-ACCESS-TOKEN-0123456789")
            credentialsConnectionID = id
            if existingID == nil {
                state.agents.append(.init(id: id, name: name, client: "manual"))
                setupManualConnectionID = id
            }
            return
        }
        stateRevision += 1
        loadingCredentials = true
        busy = true
        Task {
            defer {
                stateRevision += 1
                busy = false
                if generation == credentialsGeneration { loadingCredentials = false }
                resumeQueuedWake()
            }
            do {
                let payload = existingID == nil ? try JSONSerialization.data(withJSONObject: ["name": name]) : nil
                let path = existingID.map { "/agents/\($0)/credentials" } ?? "/agents/manual"
                let data = try await runtime.control(method: existingID == nil ? "POST" : "GET", path: path, body: payload)
                let credentials = try JSONDecoder().decode(ConnectionCredentials.self, from: data)
                guard existingID == nil || credentials.id == existingID else {
                    throw NSError(domain: "LocalWhatsAppMCP", code: 1, userInfo: [NSLocalizedDescriptionKey: "The requested credentials could not be loaded."])
                }
                guard !stopping else { return }
                if existingID == nil && !state.agents.contains(where: { $0.id == credentials.id }) {
                    state.agents.append(.init(id: credentials.id, name: credentials.name, client: "manual"))
                }
                if existingID == nil, setup == setupGeneration {
                    setupManualConnectionID = credentials.id
                }
                // A late response can update the grant list but cannot reopen credentials.
                guard isConnectionPage, generation == credentialsGeneration else { return }
                connectionCredentials = credentials
                credentialsConnectionID = credentials.id
            } catch {
                if isConnectionPage, generation == credentialsGeneration { self.error = error.localizedDescription }
            }
        }
    }
    private func clearConnectionCredentials() {
        credentialsGeneration += 1
        connectionCredentials = nil
        credentialsConnectionID = nil
        loadingCredentials = false
        accessTokenCopied = false
        credentialsURLCopied = false
    }
    func copyCredentialsURL() {
        guard let credentials = connectionCredentials else { return }
        pasteboard.clearContents()
        pasteboard.setString(credentials.url, forType: .string)
        credentialsURLCopied = true
        accessTokenCopied = false
    }
    func copyAccessToken() {
        guard let credentials = connectionCredentials else { return }
        pasteboard.clearContents()
        pasteboard.setString(credentials.accessToken, forType: .string)
        accessTokenCopied = true
        credentialsURLCopied = false
    }
    func changeSending(_ agent: AgentConnection, enabled: Bool) {
        guard !busy, serviceAvailable else { return }
        if isDemo {
            if let index = state.agents.firstIndex(where: { $0.id == agent.id }) { state.agents[index].sendEnabled = enabled }
            return
        }
        mutate(method: "PATCH", path: "/agents/\(agent.id)", body: ["send_enabled": enabled])
    }
    func removeAgent(_ agent: AgentConnection) {
        guard !busy, serviceAvailable else { return }
        if credentialsConnectionID == agent.id { clearConnectionCredentials() }
        if expandedAgentID == agent.id { expandedAgentID = nil }
        if isDemo { state.agents.removeAll { $0.id == agent.id }; return }
        mutate(method: "DELETE", path: "/agents/\(agent.id)")
    }
    func stop() async {
        stopping = true
        polling?.cancel()
        clearInvitation()
        clearConnectionCredentials()
        if !isDemo { await runtime.stop() }
    }
    func setLogin(_ enabled: Bool) {
        if isDemo { loginEnabled = enabled; return }
        do {
            if enabled { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
            UserDefaults.standard.set(true, forKey: "loginConfigured")
            loginEnabled = SMAppService.mainApp.status == .enabled
            loginMessage = SMAppService.mainApp.status == .requiresApproval ? "Allow LocalWhatsAppMCP in System Settings → Login Items." : nil
        } catch { loginMessage = "Start at Log In: \(error.localizedDescription)" }
    }
    func openLoginSettings() { SMAppService.openSystemSettingsLoginItems() }
    private func configureLogin() {
        loginEnabled = SMAppService.mainApp.status == .enabled
        if !UserDefaults.standard.bool(forKey: "loginConfigured") { setLogin(true) }
        else if SMAppService.mainApp.status == .requiresApproval { loginMessage = "Allow LocalWhatsAppMCP in System Settings → Login Items." }
    }
    private func mutate(method: String, path: String, body: [String: Any]? = nil, finished: (() -> Void)? = nil, completion: (() -> Void)? = nil) {
        guard !busy else { return }
        // Polls that predate this action or its result cannot restore old state.
        stateRevision += 1
        if ["/whatsapp/connect", "/whatsapp/disconnect", "/lifecycle/wake", "/scope"].contains(path) {
            state.whatsapp.syncState = "unknown"
            if path == "/whatsapp/connect" || path == "/lifecycle/wake" { state.whatsapp.status = "connecting" }
        }
        busy = true
        error = nil
        Task {
            var refreshAfterFailure = false
            defer {
                stateRevision += 1
                busy = false
                finished?()
                resumeQueuedWake()
                if refreshAfterFailure { Task { await refresh() } }
            }
            do {
                try await runtime.start()
                let payload = try body.map { try JSONSerialization.data(withJSONObject: $0) }
                let data = try await runtime.control(method: method, path: path, body: payload)
                state = try JSONDecoder().decode(ServiceState.self, from: data)
                serviceAvailable = true
                resolvePanelOpening()
                completion?()
            } catch {
                self.error = error.localizedDescription
                refreshAfterFailure = true
            }
        }
    }
    private func loadDemo(page: String?) {
        serviceAvailable = true
        state = ServiceState(whatsapp: .init(status: "ready", linked: true, lastSyncedAt: ISO8601DateFormatter().string(from: Date().addingTimeInterval(-120)), syncState: "caught_up"), scope: Scope(mode: "selected", chatIDs: ["work", "family"]), mcp: .init(url: "http://127.0.0.1:32191", status: "ready"))
        selection = ChatSelection(chats: [Chat(id: "work", name: "Product team", kind: "group", lastActivity: "2026-10-23T07:30:00Z"), Chat(id: "family", name: "Family", kind: "group", lastActivity: "2026-10-23T07:20:00Z"), Chat(id: "renee", name: "Renée Santos", phone: "+1 (415) 555-0123", kind: "direct"), Chat(id: "long", name: "Weekend hiking plans and the very ambitious autumn road trip", kind: "group"), Chat(id: "phone", phone: "+44 20 5550 0123", kind: "direct")] + (1...100).map { Chat(id: "project-\($0)", name: "Project \($0)", kind: "group") }, selectedIDs: ["work", "family"])
        switch page {
        case "synced", "syncing":
            state.scope = Scope(mode: "all")
            selection.chats += (101...1287).map { Chat(id: "project-\($0)", name: "Project \($0)", kind: "group") }
            state.whatsapp.syncState = page == "synced" ? "caught_up" : "syncing"
            if page == "syncing" { state.whatsapp.lastSyncedAt = ISO8601DateFormatter().string(from: Date()) }
        case "settings-enabled": self.page = .settings; loginEnabled = true
        case "mcp", "mcp-agents", "mcp-expanded", "mcp-credentials", "mcp-long":
            self.page = .mcp
            if page != "mcp" {
                var first = AgentConnection(id: "demo-agent-1", name: page == "mcp-long" ? "My assistant with a very long connection name that wraps" : "Connection 1", client: "generic", connected: true)
                first.lastSeenAt = ISO8601DateFormatter().string(from: Date().addingTimeInterval(-120))
                state.agents = [first, .init(id: "demo-agent-2", name: "Connection 2", client: "generic", sendEnabled: true)]
                if page == "mcp-expanded" || page == "mcp-credentials" || page == "mcp-long" { expandedAgentID = first.id }
                if page == "mcp-credentials" || page == "mcp-long" { toggleCredentials(first) }
            }
        case "mcp-icons":
            self.page = .mcp
            state.agents = (1...5).map { number in
                .init(id: "demo-agent-\(number)", name: "Connection \(number)", client: "generic")
            }
        case "mcp-unavailable": self.page = .mcp; serviceAvailable = false
        case "mcp-setup": self.page = .mcp; state.scope = Scope(); state.mcp.status = "setup_required"
        case "pairing-ready": self.page = .pairing; copyInstructions()
        case "manual": self.page = .pairing
        case "manual-ready": self.page = .pairing; toggleManualSetup()
        case "disconnected": state = ServiceState(); selection = ChatSelection()
        case "initial": state.scope = Scope(); state.agents = []; state.mcp.status = "setup_required"
        case "qr", "qr-hover", "loading":
            state = ServiceState(whatsapp: .init(status: "linking", qr: page == "loading" ? nil : "LOCALWHATSAPPMCP-DEMO-NOT-A-PAIRING-CODE"))
            self.page = .qr
        case "qr-error":
            state.whatsapp = .init(status: "attention", detail: "The connection was interrupted. Try again for a new QR code.", qr: "LOCALWHATSAPPMCP-DEMO-NOT-A-PAIRING-CODE")
            self.page = .qr
        case "reconnect":
            state.whatsapp = .init(status: "attention", detail: "WhatsApp sync stopped. Reconnect to resume syncing.", linked: true)
        case "disconnecting":
            state.whatsapp = .init(status: "disconnecting", linked: true, disconnectPending: true)
        case "disconnect-error":
            state.whatsapp = .init(status: "attention", detail: "WhatsApp sign-out is unfinished. Retry Disconnect.", linked: true, disconnectPending: true)
        case "catalog-loading": self.page = .chats; catalogPending = true
        case "linked": self.page = .qr; resolvePanelOpening()
        case "initial-linked": state.scope = Scope(); state.mcp.status = "setup_required"; self.page = .qr; resolvePanelOpening()
        case "reopening-linked": state.whatsapp = .init(status: "connecting", linked: true); self.page = .qr; pendingPanelOpen = true; resolvePanelOpening()
        case "error": state = ServiceState(); error = "The app could not access its saved files. Check permissions and free space, then retry."
        default: if let page, let target = Page(rawValue: page) { self.page = target }
        }
    }
}
