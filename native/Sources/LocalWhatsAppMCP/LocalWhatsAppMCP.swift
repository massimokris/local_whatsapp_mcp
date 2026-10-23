import AppKit
import Combine
import SwiftUI

@main
struct LocalWhatsAppMCPApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    var body: some Scene {
        Settings { EmptyView() }
            .commands {
                CommandGroup(replacing: .appSettings) {
                    Button("Settings…") { delegate.model.page = .settings; delegate.showPanel() }.keyboardShortcut(",")
                }
                CommandGroup(replacing: .newItem) {
                    Button("Show LocalWhatsAppMCP") { delegate.showPanel() }.keyboardShortcut("1")
                }
            }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let model = AppModel()
    private var statusItem: NSStatusItem?
    private var panel: MenuPanel?
    private var subscriptions = Set<AnyCancellable>()
    private var outsideMonitor: Any?
    private var localMonitor: Any?
    private var wakeObserver: NSObjectProtocol?
    private var terminating = false
    private var servicesStopped = false
    private var captured = false

    func applicationWillFinishLaunching(_ notification: Notification) {
        NSAppleEventManager.shared().setEventHandler(self, andSelector: #selector(handleURL(_:withReplyEvent:)), forEventClass: AEEventClass(kInternetEventClass), andEventID: AEEventID(kAEGetURL))
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem = item
        item.button?.image = NSImage(systemSymbolName: "bubble.left.and.bubble.right", accessibilityDescription: "LocalWhatsAppMCP")
        item.button?.image?.isTemplate = true
        item.button?.target = self
        item.button?.action = #selector(togglePanel(_:))
        item.button?.sendAction(on: [.leftMouseUp, .rightMouseUp])
        item.button?.setAccessibilityLabel("LocalWhatsAppMCP")
        let panel = MenuPanel(contentRect: NSRect(x: 0, y: 0, width: 460, height: 664), styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        panel.title = "LocalWhatsAppMCP"
        panel.isReleasedWhenClosed = false
        panel.backgroundColor = .clear
        panel.isOpaque = false
        panel.hasShadow = true
        panel.level = .popUpMenu
        panel.hidesOnDeactivate = false
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.onCancel = { [weak self] in self?.closePanel() }
        self.panel = panel
        model.$appearance.sink { [weak self] _ in
            DispatchQueue.main.async { [weak self] in self?.panel?.appearance = self?.model.nativeAppearance }
        }.store(in: &subscriptions)
        model.objectWillChange.sink { [weak self] _ in
            DispatchQueue.main.async { [weak self] in
                guard let self else { return }
                self.statusItem?.button?.toolTip = "LocalWhatsAppMCP · \(self.model.statusLabel)"
                self.statusItem?.button?.setAccessibilityLabel("LocalWhatsAppMCP, WhatsApp \(self.model.statusLabel)")
            }
        }.store(in: &subscriptions)
        wakeObserver = NSWorkspace.shared.notificationCenter.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.model.wake() }
        }
        model.start()
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [weak self] in self?.showPanel() }
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool { showPanel(); return false }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if servicesStopped { return .terminateNow }
        guard !terminating else { return .terminateCancel }
        terminating = true
        // Cancel this synchronous AppKit request while shutdown runs. Retrying after
        // completion avoids AppKit's termination run loop starving the main actor.
        Task {
            await model.stop()
            servicesStopped = true
            DispatchQueue.main.async { sender.terminate(nil) }
        }
        return .terminateCancel
    }
    @objc private func handleURL(_ event: NSAppleEventDescriptor, withReplyEvent reply: NSAppleEventDescriptor) {
        guard let value = event.paramDescriptor(forKeyword: keyDirectObject)?.stringValue, let url = URL(string: value) else { return }
        model.authorize(url)
    }
    @objc private func togglePanel(_ sender: NSStatusBarButton) {
        if NSApp.currentEvent?.type == .rightMouseUp {
            closePanel()
            let menu = NSMenu()
            let open = menu.addItem(withTitle: "Open LocalWhatsAppMCP", action: #selector(openFromMenu), keyEquivalent: "")
            open.target = self
            menu.addItem(.separator())
            menu.addItem(withTitle: "Quit LocalWhatsAppMCP", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
            statusItem?.menu = menu; sender.performClick(nil); statusItem?.menu = nil
        } else if panel?.isVisible == true { closePanel() } else { showPanel() }
    }
    @objc private func openFromMenu() { showPanel() }
    func showPanel() {
        guard let panel else { return }
        let screen = statusItem?.button?.window?.screen?.visibleFrame ?? NSScreen.main?.visibleFrame ?? NSRect(x: 0, y: 0, width: 1000, height: 800)
        let anchor: NSRect
        if let button = statusItem?.button, let window = button.window { anchor = window.convertToScreen(button.convert(button.bounds, to: nil)) }
        else { anchor = NSRect(x: screen.maxX - 230, y: screen.maxY, width: 20, height: 20) }
        let width: CGFloat = 460
        let height = min(664, screen.height - 20)
        let x = min(max(anchor.midX - width / 2, screen.minX + 8), screen.maxX - width - 8)
        let pointer = min(max(anchor.midX - x, 16), width - 16)
        panel.contentViewController = NSHostingController(rootView:
            VStack(spacing: 0) {
                Triangle().fill(Theme.background).frame(width: 16, height: 8).offset(x: pointer - width / 2)
                PanelView(model: model, quit: { NSApp.terminate(nil) }, dismiss: { [weak self] in self?.closePanel() })
            }.frame(width: width, height: height))
        panel.appearance = model.nativeAppearance
        panel.setFrame(NSRect(x: x, y: anchor.minY - height, width: width, height: height), display: true)
        panel.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        installMonitors()
        captureIfRequested()
    }
    private func closePanel() {
        panel?.orderOut(nil)
        if let outsideMonitor { NSEvent.removeMonitor(outsideMonitor) }
        if let localMonitor { NSEvent.removeMonitor(localMonitor) }
        outsideMonitor = nil; localMonitor = nil
    }
    private func installMonitors() {
        if outsideMonitor == nil {
            outsideMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in Task { @MainActor in self?.closePanel() } }
        }
        if localMonitor == nil {
            localMonitor = NSEvent.addLocalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] event in
                guard let self else { return event }
                if event.window !== self.panel && event.window !== self.statusItem?.button?.window { self.closePanel() }
                return event
            }
        }
    }
    private func captureIfRequested() {
        guard model.isDemo, !captured, let path = ProcessInfo.processInfo.environment["LWMCP_CAPTURE_PATH"] else { return }
        captured = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.7) { [weak self] in
            guard let view = self?.panel?.contentView else { return }
            view.layoutSubtreeIfNeeded()
            guard let bitmap = view.bitmapImageRepForCachingDisplay(in: view.bounds) else { return }
            view.cacheDisplay(in: view.bounds, to: bitmap)
            guard let data = bitmap.representation(using: .png, properties: [:]) else { return }
            Task {
                _ = await Task.detached { try? data.write(to: URL(fileURLWithPath: path), options: .atomic) }.value
                if ProcessInfo.processInfo.environment["LWMCP_CAPTURE_EXIT"] == "1" { DispatchQueue.main.async { NSApp.terminate(nil) } }
            }
        }
    }
}
private final class MenuPanel: NSPanel {
    var onCancel: (() -> Void)?
    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
    override func cancelOperation(_ sender: Any?) { onCancel?() }
}
private struct Triangle: Shape {
    func path(in rect: CGRect) -> Path { Path { p in p.move(to: CGPoint(x: rect.midX, y: 0)); p.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY)); p.addLine(to: CGPoint(x: 0, y: rect.maxY)); p.closeSubpath() } }
}
