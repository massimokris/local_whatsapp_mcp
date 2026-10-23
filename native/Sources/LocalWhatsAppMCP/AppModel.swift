import AppKit
import Combine
import LocalWhatsAppCore
import ServiceManagement

@MainActor
final class AppModel: ObservableObject {
    enum Page: String { case home, chats, agents, pairing, cloud, settings }
    enum Appearance: String, CaseIterable { case system = "System", light = "Light", dark = "Dark" }
    @Published var state = ServiceState()
    @Published var page: Page = .home
    @Published var selection = ChatSelection()
    @Published var busy = false
    @Published var catalogPending = false
    @Published var error: String?
    @Published var instructions: String?
    @Published var invitationExpires: String?
    @Published var agentName = "My agent"
    @Published var agentClient = "codex"
    @Published var provider = "ngrok"
    @Published var appearance: Appearance = .system {
        didSet { if !isDemo { UserDefaults.standard.set(appearance.rawValue, forKey: "appearance") } }
    }
    @Published var loginEnabled = false
    @Published var loginMessage: String?
    let isDemo: Bool
    let runtime: HelperRuntime
    private var polling: Task<Void, Never>?
    private var invitationAgentCount: Int?
    private var refreshing = false
    private var stopping = false
    private var didLoad = false
    private var seedAllSelection = false

    init() {
        let environment = ProcessInfo.processInfo.environment
        isDemo = CommandLine.arguments.contains("--demo") || environment["LWMCP_DEMO"] == "1"
        let helpers = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers")
        let applicationSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        runtime = HelperRuntime(helper: helpers.appendingPathComponent("localwhatsappmcp"), wacli: helpers.appendingPathComponent("wacli"), dataDirectory: applicationSupport.appendingPathComponent("LocalWhatsAppMCP"))
        appearance = Appearance(rawValue: environment["LWMCP_APPEARANCE"] ?? (isDemo ? "System" : UserDefaults.standard.string(forKey: "appearance") ?? "System")) ?? .system
        if isDemo { loadDemo(page: environment["LWMCP_DEMO_PAGE"]) }
    }
    var whatsappReady: Bool {
        state.whatsapp.linked == true || ["connected", "ready", "syncing", "importing", "receiving_history"].contains(state.whatsapp.status ?? "")
    }
    var scopeLabel: String {
        switch state.scope.mode {
        case "all": return "All chats · including future chats"
        case "selected": return "\(state.scope.chatIDs.count) selected chats"
        default: return "Initial capture · choose your scope"
        }
    }
    var statusLabel: String { (state.whatsapp.status ?? "Starting").replacingOccurrences(of: "_", with: " ").capitalized }
    var nativeAppearance: NSAppearance? {
        switch appearance { case .system: return nil; case .light: return NSAppearance(named: .aqua); case .dark: return NSAppearance(named: .darkAqua) }
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
        guard !isDemo, !refreshing, !stopping else { return }
        refreshing = true
        defer { refreshing = false }
        do {
            try await runtime.start()
            let data = try await runtime.control(method: "GET", path: "/state")
            let latest = try JSONDecoder().decode(ServiceState.self, from: data)
            state = latest
            if !didLoad { selection.selectedIDs = Set(latest.scope.chatIDs); didLoad = true }
            if error?.hasPrefix("Connecting to the local service") == true { error = nil }
            if let count = invitationAgentCount, latest.agents.count > count { clearInvitation() }
            if let expiry = invitationExpires, let date = ISO8601DateFormatter().date(from: expiry), date < Date() { clearInvitation() }
            if page == .chats || (latest.scope.mode == "initial" && whatsappReady) {
                let catalog = try await runtime.control(method: "GET", path: "/catalog")
                selection.chats = try JSONDecoder().decode(Catalog.self, from: catalog).chats
                if seedAllSelection { selection.selectedIDs = Set(selection.chats.map(\.id)); seedAllSelection = false }
                catalogPending = false
            }
        } catch {
            if !busy { self.error = "Connecting to the local service. \(error.localizedDescription)" }
        }
    }
    func connectWhatsApp() {
        if isDemo { state.whatsapp = .init(status: "linking", detail: "Demo QR only. No WhatsApp connection will be made.", qr: "LOCALWHATSAPPMCP-DEMO-NOT-A-PAIRING-CODE"); return }
        mutate(method: "POST", path: "/whatsapp/connect", body: [:])
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
        if isDemo {
            state.scope = Scope(mode: all ? "all" : "selected", chatIDs: all ? [] : selection.selectedIDs.sorted())
            page = .agents; return
        }
        mutate(method: "PUT", path: "/scope", body: ["mode": all ? "all" : "selected", "chat_ids": all ? [] : selection.selectedIDs.sorted()]) { self.page = .agents }
    }
    func changeSending(_ agent: AgentConnection, enabled: Bool) {
        if isDemo { if let index = state.agents.firstIndex(where: { $0.id == agent.id }) { state.agents[index].sendEnabled = enabled }; return }
        mutate(method: "PATCH", path: "/agents/\(agent.id)", body: ["send_enabled": enabled])
    }
    func disconnect(_ agent: AgentConnection) {
        if isDemo { state.agents.removeAll { $0.id == agent.id }; return }
        mutate(method: "DELETE", path: "/agents/\(agent.id)")
    }
    func makeInstructions(cloud: Bool) {
        guard !busy else { return }
        clearInvitation()
        if isDemo { instructions = "Demo setup instructions. This preview does not create credentials, pair an agent, or change your configuration."; return }
        busy = true; error = nil
        Task {
            defer { busy = false }
            do {
                let body: [String: String] = cloud ? ["provider": provider, "name": agentName] : ["name": agentName, "client": agentClient]
                let data = try await runtime.control(method: "POST", path: cloud ? "/cloud/instructions" : "/pairing", body: JSONSerialization.data(withJSONObject: body))
                let result = try JSONDecoder().decode(SetupInstructions.self, from: data)
                instructions = result.instructions; invitationExpires = result.expiresAt
                if !cloud { invitationAgentCount = state.agents.count }
            } catch { self.error = error.localizedDescription }
        }
    }
    func disableCloud() {
        if isDemo { state.cloud = .init(); return }
        mutate(method: "DELETE", path: "/cloud")
    }
    func wake() {
        guard !isDemo, !stopping else { return }
        Task {
            do { try await runtime.start(); _ = try await runtime.control(method: "POST", path: "/lifecycle/wake", body: Data("{}".utf8)); await refresh() }
            catch { self.error = error.localizedDescription }
        }
    }
    func authorize(_ url: URL) {
        guard !isDemo, !stopping, let request = CloudAuthorizationCallback.request(from: url) else { return }
        Task {
            do { try await runtime.start(); try await runtime.authorizeCloud(request: request); await refresh() }
            catch { self.error = error.localizedDescription }
        }
    }
    func stop() async {
        stopping = true; polling?.cancel(); clearInvitation()
        if !isDemo { await runtime.stop() }
    }
    func clearInvitation() {
        if let instructions, NSPasteboard.general.string(forType: .string) == instructions { NSPasteboard.general.clearContents() }
        instructions = nil; invitationExpires = nil; invitationAgentCount = nil
    }
    func copyInstructions() {
        guard let instructions else { return }
        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(instructions, forType: .string)
    }
    func setLogin(_ enabled: Bool) {
        if isDemo { loginEnabled = enabled; return }
        do {
            if enabled { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
            UserDefaults.standard.set(true, forKey: "loginConfigured")
            loginEnabled = SMAppService.mainApp.status == .enabled
            loginMessage = SMAppService.mainApp.status == .requiresApproval ? "Allow LocalWhatsAppMCP in System Settings → Login Items." : nil
        } catch { loginMessage = "Open at login: \(error.localizedDescription)" }
    }
    func openLoginSettings() { SMAppService.openSystemSettingsLoginItems() }
    private func configureLogin() {
        loginEnabled = SMAppService.mainApp.status == .enabled
        if !UserDefaults.standard.bool(forKey: "loginConfigured") { setLogin(true) }
        else if SMAppService.mainApp.status == .requiresApproval { loginMessage = "Allow LocalWhatsAppMCP in System Settings → Login Items." }
    }
    private func mutate(method: String, path: String, body: [String: Any]? = nil, completion: (() -> Void)? = nil) {
        guard !busy else { return }; busy = true; error = nil
        Task {
            defer { busy = false }
            do {
                let payload = try body.map { try JSONSerialization.data(withJSONObject: $0) }
                let data = try await runtime.control(method: method, path: path, body: payload)
                state = try JSONDecoder().decode(ServiceState.self, from: data)
                completion?()
            } catch { self.error = error.localizedDescription }
        }
    }
    private func loadDemo(page: String?) {
        state = ServiceState(whatsapp: .init(status: "syncing", detail: "Import available history · recent events received. WhatsApp controls history coverage.", lastEventAt: "2026-10-23T07:30:00Z"), scope: Scope(mode: "selected", chatIDs: ["work", "family"]), agents: [.init(id: "demo-codex", name: "Codex · personal", client: "codex", connected: true), .init(id: "demo-claude", name: "Claude · research", client: "claude", sendEnabled: true)], mcp: .init(url: "http://127.0.0.1:8765/mcp", status: "ready"), cloud: .init(enabled: true, provider: "ngrok", hostname: "example.ngrok.app", status: "attention", detail: "Demo: provider login required. Local access is available."))
        selection = ChatSelection(chats: [Chat(id: "work", name: "Product team", kind: "group", lastActivity: "2026-10-23T07:30:00Z"), Chat(id: "family", name: "Family", kind: "group", lastActivity: "2026-10-23T07:20:00Z"), Chat(id: "renee", name: "Renée Santos", phone: "+1 (415) 555-0123", kind: "direct"), Chat(id: "long", name: "Weekend hiking plans and the very ambitious autumn road trip", kind: "group"), Chat(id: "phone", phone: "+44 20 5550 0123", kind: "direct")] + (1...100).map { Chat(id: "project-\($0)", name: "Project \($0)", kind: "group") }, selectedIDs: ["work", "family"])
        if page == "initial" { state.scope = Scope(); self.page = .home }
        else if page == "qr" { state.scope = Scope(); state.whatsapp = .init(status: "linking", detail: "Demo QR only. No WhatsApp connection will be made.", qr: "LOCALWHATSAPPMCP-DEMO-NOT-A-PAIRING-CODE"); self.page = .home }
        else if let page, let target = Page(rawValue: page) { self.page = target }
    }
}
