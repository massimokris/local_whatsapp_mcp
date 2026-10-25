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
                    Button("Settings…") { delegate.model.page = .settings; delegate.showPanel(prepareConnection: false) }.keyboardShortcut(",")
                }
                CommandGroup(replacing: .newItem) {
                    Button("Show LocalWhatsAppMCP") { delegate.showPanel() }.keyboardShortcut("1")
                }
            }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let model = AppModel()
    private var statusItem: NSStatusItem?
    private var panel: MenuPanel?
    private var subscriptions = Set<AnyCancellable>()
    private var wakeObserver: NSObjectProtocol?
    private var terminating = false
    private var servicesStopped = false
    private var captured = false

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
        let panel = MenuPanel(contentRect: NSRect(x: 0, y: 0, width: 420, height: 750), styleMask: [.titled, .closable, .miniaturizable, .fullSizeContentView], backing: .buffered, defer: false)
        panel.title = "LocalWhatsAppMCP"
        panel.titleVisibility = .hidden
        panel.titlebarAppearsTransparent = true
        panel.toolbarStyle = .unified
        panel.isMovableByWindowBackground = true
        panel.isReleasedWhenClosed = false
        panel.backgroundColor = NSColor(Theme.background)
        panel.isOpaque = true
        panel.hasShadow = true
        panel.isFloatingPanel = false
        panel.level = .normal
        panel.hidesOnDeactivate = false
        panel.collectionBehavior = [.managed]
        panel.appearance = NSAppearance(named: .darkAqua)
        panel.delegate = self
        self.panel = panel
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
    func windowShouldClose(_ sender: NSWindow) -> Bool { closePanel(); return false }
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
    @objc private func togglePanel(_ sender: NSStatusBarButton) {
        if NSApp.currentEvent?.type == .rightMouseUp {
            let menu = NSMenu()
            let open = menu.addItem(withTitle: "Open LocalWhatsAppMCP", action: #selector(openFromMenu), keyEquivalent: "")
            open.target = self
            menu.addItem(.separator())
            menu.addItem(withTitle: "Quit LocalWhatsAppMCP", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
            statusItem?.menu = menu; sender.performClick(nil); statusItem?.menu = nil
        } else { showPanel() }
    }
    @objc private func openFromMenu() { showPanel() }
    func showPanel(prepareConnection: Bool = true) {
        guard let panel else { return }
        if panel.isVisible && !panel.isMiniaturized {
            panel.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            return
        }
        if prepareConnection { model.panelOpened() }
        let screen = statusItem?.button?.window?.screen?.visibleFrame ?? NSScreen.main?.visibleFrame ?? NSRect(x: 0, y: 0, width: 1000, height: 800)
        let anchor: NSRect
        if let button = statusItem?.button, let window = button.window { anchor = window.convertToScreen(button.convert(button.bounds, to: nil)) }
        else { anchor = NSRect(x: screen.maxX - 210, y: screen.maxY, width: 20, height: 20) }
        let width: CGFloat = min(420, screen.width - 16)
        let height = min(750, screen.height - 16)
        let x = min(max(anchor.midX - width / 2, screen.minX + 8), screen.maxX - width - 8)
        let y = max(screen.minY + 8, min(anchor.minY - height - 4, screen.maxY - height - 8))
        let controller = NSHostingController(rootView:
            PanelView(model: model, quit: { NSApp.terminate(nil) })
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .ignoresSafeArea()
                .preferredColorScheme(.dark))
        // AppKit owns the outer frame. Propagating the hosting view's intrinsic
        // size adds titlebar height a second time with full-size content views.
        controller.sizingOptions = []
        panel.contentViewController = controller
        panel.setFrame(NSRect(x: x, y: y, width: width, height: height), display: true)
        if panel.isMiniaturized { panel.deminiaturize(nil) }
        panel.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        captureIfRequested()
    }
    private func closePanel() {
        model.panelClosed()
        panel?.orderOut(nil)
    }
    private func captureIfRequested() {
        guard model.isDemo, !captured, let path = ProcessInfo.processInfo.environment["LWMCP_CAPTURE_PATH"] else { return }
        captured = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.7) { [weak self] in
            guard let contentView = self?.panel?.contentView else { return }
            // The enclosing AppKit frame includes the real native titlebar controls.
            // Rendering that view keeps demo captures confined to this window.
            let view = contentView.superview ?? contentView
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
    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { true }
}
