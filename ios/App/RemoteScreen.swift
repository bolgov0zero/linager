import LinagerKit
import SwiftUI

/// The screen of a computer in the Assistant: the picture in landscape, the panel of buttons at the top that folds into a button that can be
/// moved, the keyboard.
struct RemoteScreen: View {
    let card: ServerCard
    /// The remote desktop (RDP) instead of the Assistant: how to sign in to the computer. The screen is the same.
    var rdp: RdpParams?
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var phase
    @Environment(VaultModel.self) private var vault
    @Environment(LockModel.self) private var lock
    @State private var remote = RemoteModel()
    @AppStorage("remote.zoom") private var zoom: RemoteZoom = .fit
    @AppStorage("remote.touch") private var touch: RemoteTouchMode = .direct
    @AppStorage("remote.stats") private var showStats = true
    @AppStorage("remote.debug") private var debugInput = false
    @State private var folded = false
    @State private var autoFolded = false
    @State private var keyboardWanted = false
    @State private var fab: CGPoint?
    @State private var dragStart: CGPoint?
    @State private var showSignIn = false
    @State private var selectMode = false
    /// The phone has turned to landscape. The connection starts and the picture shows only then, so the turn is not a jerk of a live picture.
    @State private var turned = false
    @State private var openMenu: PanelMenu?
    @State private var capsule: CGRect = .zero
    @State private var buttonFrames: [PanelMenu: CGRect] = [:]
    @State private var menuHeight: CGFloat = 0
    /// The menu is drawn open (it grows from its button when this turns on and shrinks back when it turns off).
    @State private var menuOn = false

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .topLeading) {
                Color.black.ignoresSafeArea()
                RemoteSurface(model: remote, zoom: zoom, touch: touch, select: selectMode, keyboard: keyboardWanted) {
                    UIKitAccessory.make(for: remote) { keyboardWanted = false }
                }
                .ignoresSafeArea()
                // the picture comes out of a soft blur and a little smaller; the black "connecting" cover fades at the same time
                .opacity(revealed ? 1 : 0)
                .scaleEffect(revealed ? 1 : 0.94)
                .blur(radius: revealed ? 0 : 14)

                overlays(geo)
                panel(geo)
                dropdown(geo)
                foldedButton(geo)
            }
            .coordinateSpace(name: "remote")
            .animation(.smooth(duration: 0.6), value: revealed)
            .onChange(of: geo.size.width > geo.size.height) { _, landscape in
                if landscape { markTurned(after: .milliseconds(450)) }
            }
        }
        .statusBarHidden(true)
        .persistentSystemOverlays(.hidden)
        .onAppear { Orientation.landscape() }
        .onDisappear {
            remote.close()
            Orientation.portrait()
        }
        // if the phone does not report the turn (it is already in landscape, or it refuses), the connection starts anyway
        .task { markTurned(after: .seconds(1.5)) }
        .task(id: turned) {
            guard turned else { return }
            let id = card.id
            if let rdp {
                remote.configureRdp(RdpBackend(open: { try await model.openDesktop(serverID: id, params: rdp, engine: RustRdpEngine()) }))
                return
            }
            remote.configure(AssistBackend(
                open: { try await model.openAssistant(serverID: id, session: "console") },
                settings: { try await model.assistantSettings(serverID: id) },
                setSettings: { try await model.setAssistantSettings(serverID: id, $0) }))
        }
        .onChange(of: phase) { _, new in
            switch new {
            case .background: remote.setPaused(true)
            case .active: remote.setPaused(false)
            default: break
            }
        }
        .onChange(of: folded) { _, now in if now { openMenu = nil; menuOn = false } }
        .onChange(of: remote.keyboardShown) { _, shown in
            keyboardWanted = shown
            // the keyboard takes half of the screen: the panel steps aside into its button, and comes back when the keyboard goes
            if shown, !folded {
                autoFolded = true
                withAnimation { folded = true }
            } else if !shown, autoFolded {
                autoFolded = false
                withAnimation { folded = false }
            }
        }
        .alert("remote.failed.title", isPresented: Binding(get: { failureText != nil }, set: { if !$0 { dismiss() } })) {
            Button("common.ok") { dismiss() }
        } message: {
            Text(failureText ?? "")
        }
        .sheet(isPresented: $showSignIn) { SignInKeysSheet(remote: remote, card: card) }
    }

    /// The key button: with the vault open the list of the keys; with it locked and Face ID on, the face is asked for at once, with no sheet in
    /// between, and the list follows. If the face does not open it, the sheet with the passphrase is there.
    private func openKeys() {
        guard vault.state == .locked, vault.faceIDOn else {
            showSignIn = true
            return
        }
        Task {
            await vault.unlockWithFaceID(context: lock.authenticated)
            showSignIn = true
        }
    }

    private var revealed: Bool { turned && remote.hasPicture }

    private func markTurned(after delay: Duration) {
        guard !turned else { return }
        Task {
            try? await Task.sleep(for: delay)
            turned = true
        }
    }

    private var failureText: String? {
        switch remote.state {
        case .failed(let e): e.text()
        case .busy: L("remote.busy")
        default: nil
        }
    }

    // MARK: the overlays of the picture

    @ViewBuilder private func overlays(_ geo: GeometryProxy) -> some View {
        let i = geo.safeAreaInsets
        if !turned || remote.state == .connecting || (!remote.hasPicture && remote.state == .open) {
            VStack(spacing: 14) {
                ProgressView().controlSize(.large).tint(.white)
                Text(card.name).font(.headline).foregroundStyle(.white)
                Text("remote.connecting").font(.subheadline).foregroundStyle(.white.opacity(0.6))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.black)
            .allowsHitTesting(false)
            .transition(.opacity)
        }
        if remote.reconnecting {
            Text("remote.reconnecting").font(.footnote).padding(.horizontal, 12).padding(.vertical, 6)
                .glassEffect(.regular, in: .capsule)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).padding(.top, 62)
                .allowsHitTesting(false)
        }
        if remote.state == .waking {
            Text("remote.waking").font(.footnote).padding(.horizontal, 12).padding(.vertical, 6)
                .glassEffect(.regular, in: .capsule)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).padding(.top, 62)
                .allowsHitTesting(false)
        }
        if showStats, remote.hello != nil, remote.state == .open, remote.hasPicture {
            Text(L("remote.stats", Int(remote.stats.fps.rounded()), String(format: "%.1f", Double(remote.stats.kbps) / 1000)))
                .font(.system(.caption2, design: .monospaced)).monospacedDigit()
                .padding(.horizontal, 10).padding(.vertical, 4)
                .glassEffect(.regular, in: .capsule)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading)
                .padding(.leading, i.leading + 8).padding(.bottom, max(i.bottom, 8) + 4)
                .allowsHitTesting(false)
        }
        if debugInput {
            Text("↓\(remote.sentDowns) ↑\(remote.sentUps) m\(remote.sentMoves)  \(remote.leftHeld ? "HELD" : "free")  \(Int(remote.lastPoint.x)),\(Int(remote.lastPoint.y))")
                .font(.system(.caption2, design: .monospaced)).monospacedDigit()
                .padding(.horizontal, 10).padding(.vertical, 4)
                .glassEffect(.regular, in: .capsule)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
                .padding(.trailing, i.trailing + 8).padding(.bottom, max(i.bottom, 8) + 4)
                .allowsHitTesting(false)
        }
        if let notice = remote.notice {
            Text(notice).font(.subheadline.weight(.medium)).padding(.horizontal, 16).padding(.vertical, 8)
                .glassEffect(.regular, in: .capsule)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .offset(y: 40)
                .allowsHitTesting(false)
                .transition(.opacity)
        }
        if case .closed(let reason) = remote.state {
            VStack(spacing: 6) {
                Text("remote.closed").font(.headline)
                Text(reason).font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                HStack(spacing: 8) {
                    Button { remote.reconnect() } label: { Text("remote.reconnect").frame(maxWidth: .infinity) }.buttonStyle(.borderedProminent)
                    Button { dismiss() } label: { Text("remote.close").frame(maxWidth: .infinity) }.buttonStyle(.bordered)
                }
                .controlSize(.large)
                .padding(.top, 6)
            }
            .padding(18)
            .frame(width: 360)
            .glassEffect(.regular, in: .rect(cornerRadius: 26))
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    // MARK: the panel

    @ViewBuilder private func panel(_ geo: GeometryProxy) -> some View {
        let i = geo.safeAreaInsets
        GlassEffectContainer(spacing: 8) {
            HStack(spacing: 8) {
                Button { dismiss() } label: {
                    Image(systemName: "xmark").font(.system(size: 16, weight: .bold)).foregroundStyle(.red).frame(width: 44, height: 44).contentShape(Circle())
                }
                .buttonStyle(.plain)
                .glassEffect(.regular.interactive(), in: .circle)
                .accessibilityLabel(Text("remote.close"))

                HStack(spacing: 0) {
                    barButton("keyboard", "remote.keyboard", active: keyboardWanted) { keyboardWanted.toggle() }
                    menuButton(.keys, "command", "remote.keys")
                    menuButton(.mouse, selectMode ? "text.cursor" : (touch == .direct ? "hand.point.up.left" : "rectangle.and.hand.point.up.left"), "remote.mouse")
                    menuButton(.zoom, "arrow.up.left.and.arrow.down.right", "remote.zoom")
                    // the clipboard, the monitors and the quality are the Assistant's; the remote desktop has none of them yet
                    if !remote.isRdp { menuButton(.clipboard, "doc.on.clipboard", "remote.clipboard") }
                    if let h = remote.hello, h.monitors.count > 1 { menuButton(.monitor, "display.2", "remote.monitor") }
                    if !remote.isRdp { menuButton(.quality, "gauge.with.dots.needle.33percent", "remote.quality") }
                    if remote.stats.isSignInDesktop {
                        barButton("key", "remote.signin", active: false, badge: true) { openKeys() }
                    }
                }
                .padding(.horizontal, 5)
                .frame(height: 44)
                // where the menus open: right under the capsule, between the close and the fold buttons
                .onGeometryChange(for: CGRect.self) { $0.frame(in: .named("remote")) } action: { capsule = $0 }
                .glassEffect(.regular, in: .capsule)

                Button { withAnimation { folded = true; autoFolded = false } } label: {
                    Image(systemName: "chevron.up").font(.system(size: 16, weight: .bold)).foregroundStyle(.primary).frame(width: 44, height: 44).contentShape(Circle())
                }
                .buttonStyle(.plain)
                .glassEffect(.regular.interactive(), in: .circle)
                .accessibilityLabel(Text("remote.fold"))
            }
        }
        .padding(.top, 8)
        .padding(.horizontal, max(i.leading, i.trailing) + 8)
        .frame(maxWidth: .infinity, alignment: .center)
        .opacity(folded ? 0 : 1)
        .offset(y: folded ? -16 : 0)
        .allowsHitTesting(!folded)
    }

    private func barButton(_ symbol: String, _ label: LocalizedStringKey, active: Bool, badge: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 17, weight: .regular))
                .foregroundStyle(active ? Color.white : Color.primary)
                .frame(minWidth: 38, minHeight: 36)
                .background(Capsule().fill(active ? Color.accentColor : Color.clear))
                .overlay(alignment: .topTrailing) { if badge { Circle().fill(Color.orange).frame(width: 8, height: 8).offset(x: -4, y: 4) } }
                .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(label))
    }

    // MARK: the menus of the panel
    //
    // A system Menu opens where the system puts it, over or beside its button. These open in one place: a glass sheet under the capsule,
    // exactly as wide as the capsule, between the close and the fold buttons. Every row has a symbol.

    private func menuButton(_ menu: PanelMenu, _ symbol: String, _ label: LocalizedStringKey) -> some View {
        let on = openMenu == menu
        return Button {
            if on {
                closeMenu()
            } else {
                menuHeight = 0
                openMenu = menu
            }
        } label: {
            Image(systemName: symbol)
                .font(.system(size: 17, weight: .regular))
                .foregroundStyle(Color.primary)
                .frame(minWidth: 38, minHeight: 36)
                .background(Capsule().fill(on ? Color.primary.opacity(0.14) : Color.clear))
                .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .onGeometryChange(for: CGRect.self) { $0.frame(in: .named("remote")) } action: { buttonFrames[menu] = $0 }
        .accessibilityLabel(Text(label))
        .accessibilityAddTraits(on ? .isSelected : [])
    }

    private static let menuSpring = Animation.spring(response: 0.36, dampingFraction: 0.8)

    private func closeMenu() {
        withAnimation(Self.menuSpring) { menuOn = false } completion: { if !menuOn { openMenu = nil } }
    }

    @ViewBuilder private func dropdown(_ geo: GeometryProxy) -> some View {
        if let menu = openMenu, !folded, capsule.width > 0 {
            let room = max(140, geo.size.height - capsule.maxY - 12 - geo.safeAreaInsets.bottom)
            let rows = menuRows(menu)
            // the menu grows out of its button: it starts small at the button and opens down, as a system menu does
            let from = buttonFrames[menu].map { UnitPoint(x: min(1, max(0, ($0.midX - capsule.minX) / capsule.width)), y: 0) } ?? .top
            // the height of the rows (an estimate until they are measured), so the sheet has no empty room
            let estimate = CGFloat(rows.joined().count) * 42 + CGFloat(max(0, rows.count - 1)) * 9 + 16
            ZStack(alignment: .topLeading) {
                // a tap beside the menu closes it and does not reach the picture
                Color.clear.contentShape(Rectangle()).onTapGesture { closeMenu() }.ignoresSafeArea()
                ScrollView {
                    menuList(rows)
                        .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { menuHeight = $0 }
                }
                .scrollBounceBehavior(.basedOnSize)
                .frame(width: capsule.width, height: min(menuHeight > 0 ? menuHeight : estimate, room))
                .glassEffect(.regular, in: .rect(cornerRadius: 26))
                // scaled about its own corner above the button, then moved into place (a scale after the move would pivot about the wrong point)
                .scaleEffect(menuOn ? 1 : 0.25, anchor: from)
                .opacity(menuOn ? 1 : 0)
                .offset(x: capsule.minX, y: capsule.maxY + 8)
                .onAppear { withAnimation(Self.menuSpring) { menuOn = true } }
            }
        }
    }

    private func menuList(_ sections: [[PanelItem]]) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(sections.enumerated()), id: \.offset) { si, items in
                if si > 0 { Divider().padding(.horizontal, 16) }
                ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                    Button {
                        closeMenu()
                        item.action()
                    } label: {
                        HStack(spacing: 14) {
                            Image(systemName: item.symbol).font(.system(size: 17)).frame(width: 24)
                            Text(item.title).font(.body)
                            Spacer(minLength: 8)
                            if item.checked { Image(systemName: "checkmark").font(.system(size: 15, weight: .semibold)) }
                        }
                        .foregroundStyle(Color.primary)
                        .padding(.horizontal, 18)
                        .frame(maxWidth: .infinity, minHeight: 42, alignment: .leading)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .disabled(item.disabled)
                    .accessibilityAddTraits(item.checked ? .isSelected : [])
                }
            }
        }
        .padding(.vertical, 8)
    }

    /// The rows of a menu, in groups (a line between the groups).
    private func menuRows(_ menu: PanelMenu) -> [[PanelItem]] {
        let linux = remote.hello?.isLinux == true
        switch menu {
        case .keys:
            var first: [PanelItem] = []
            if !linux { first.append(PanelItem("lock.open", remote.isRdp ? "remote.keys.cadrdp" : "remote.keys.cad") { remote.ctrlAltDel() }) }
            first.append(PanelItem("square.grid.2x2", "remote.keys.win") { remote.shortcut(ScanCodes.win, with: [], label: "Win") })
            first.append(PanelItem("rectangle.on.rectangle", "remote.keys.alttab") { remote.shortcut(ScanCodes.tab, with: [.alt], label: "Alt+Tab") })
            if !linux { first.append(PanelItem("waveform.path.ecg.rectangle", "remote.keys.taskmgr") { remote.shortcut(ScanCodes.esc, with: [.ctrl, .shift], label: "Ctrl+Shift+Esc") }) }
            first.append(PanelItem("escape", "remote.keys.esc") { remote.shortcut(ScanCodes.esc, with: [], label: "Esc") })
            return [first, [
                PanelItem("doc.on.doc", "remote.keys.copy") { remote.shortcut(KeyStroke(sc: 0x2e), with: [.ctrl], label: "Ctrl+C") },
                PanelItem("doc.on.clipboard", "remote.keys.paste") { remote.shortcut(KeyStroke(sc: 0x2f), with: [.ctrl], label: "Ctrl+V") },
            ]]
        case .mouse:
            return [[
                PanelItem("hand.point.up.left", "remote.mouse.direct", checked: touch == .direct) { touch = .direct },
                PanelItem("rectangle.and.hand.point.up.left", "remote.mouse.pad", checked: touch == .pad) { touch = .pad },
            ], [
                PanelItem("text.cursor", "remote.mouse.select", checked: selectMode) {
                    selectMode.toggle()
                    remote.show(L(selectMode ? "remote.mouse.select.on" : "remote.mouse.select.off"))
                },
                PanelItem("ladybug", "remote.debug", checked: debugInput) { debugInput.toggle() },
            ]]
        case .zoom:
            return [[
                PanelItem("arrow.up.left.and.arrow.down.right", "remote.zoom.fit", checked: zoom == .fit) { zoom = .fit },
                PanelItem("arrow.left.and.right", "remote.zoom.width", checked: zoom == .width) { zoom = .width },
                PanelItem("1.magnifyingglass", "remote.zoom.actual", checked: zoom == .actual) { zoom = .actual },
            ]]
        case .clipboard:
            return [[
                PanelItem("arrow.up.doc", "remote.clip.send") { remote.sendClipboard() },
                PanelItem("arrow.down.doc", "remote.clip.get") { remote.fetchClipboard() },
            ]]
        case .monitor:
            guard let h = remote.hello else { return [] }
            var items = [PanelItem("display.2", "remote.monitor.all", checked: h.monitor == -1) { remote.selectMonitor(-1) }]
            for i in h.monitors.indices {
                items.append(PanelItem("display", LocalizedStringKey(L("remote.monitor.n", i + 1)), checked: h.monitor == i) { remote.selectMonitor(i) })
            }
            return [items]
        case .quality:
            let preset = remote.settings?.preset
            var first = [
                PanelItem("leaf", "remote.quality.eco", checked: preset == .eco) { remote.applyPreset(.eco) },
                PanelItem("gauge.with.dots.needle.50percent", "remote.quality.normal", checked: preset == .normal) { remote.applyPreset(.normal) },
                PanelItem("gauge.with.dots.needle.100percent", "remote.quality.hq", checked: preset == .hq) { remote.applyPreset(.hq) },
            ]
            if remote.settings != nil, preset == nil {
                first.append(PanelItem("slider.horizontal.3", "remote.quality.custom", checked: true) {})
            }
            return [first, [
                PanelItem("chart.xyaxis.line", "remote.stats.show", checked: showStats) { showStats.toggle() },
                PanelItem("cursorarrow", "remote.cursor.show", checked: remote.settings?.showCursor ?? false) { remote.setShowCursor(!(remote.settings?.showCursor ?? false)) },
            ]]
        }
    }

    // MARK: the button the panel folds into

    @ViewBuilder private func foldedButton(_ geo: GeometryProxy) -> some View {
        let i = geo.safeAreaInsets
        let size: CGFloat = 48
        let minX = i.leading + 6, maxX = geo.size.width - i.trailing - size - 6
        let home = CGPoint(x: maxX, y: 12)
        let pos = fab ?? home
        if folded {
            Image(systemName: "ellipsis").font(.system(size: 20, weight: .semibold)).foregroundStyle(.primary)
                .frame(width: size, height: size)
                .glassEffect(.regular.interactive(), in: .circle)
                .contentShape(Circle())
                .scaleEffect(dragStart == nil ? 1 : 1.08)
                .position(x: pos.x + size / 2, y: pos.y + size / 2)
                .gesture(
                    DragGesture(minimumDistance: 0)
                        .onChanged { v in
                            if dragStart == nil { dragStart = pos }
                            guard let s = dragStart else { return }
                            fab = CGPoint(x: max(minX, min(maxX, s.x + v.translation.width)), y: max(6, min(geo.size.height - size - 24, s.y + v.translation.height)))
                        }
                        .onEnded { v in
                            let moved = hypot(v.translation.width, v.translation.height) > 6
                            dragStart = nil
                            if moved, let f = fab {
                                // it sticks to the nearer side
                                withAnimation(.snappy) { fab = CGPoint(x: f.x + size / 2 < geo.size.width / 2 ? minX : maxX, y: f.y) }
                            } else {
                                withAnimation { folded = false; autoFolded = false }
                            }
                        }
                )
                .accessibilityLabel(Text("remote.panel"))
                .accessibilityAddTraits(.isButton)
                .transition(.scale.combined(with: .opacity))
        }
    }
}

/// The menus of the panel.
private enum PanelMenu: Hashable { case keys, mouse, zoom, clipboard, monitor, quality }

/// A row of a menu of the panel.
private struct PanelItem {
    let symbol: String
    let title: LocalizedStringKey
    var checked = false
    var disabled = false
    let action: () -> Void

    init(_ symbol: String, _ title: LocalizedStringKey, checked: Bool = false, disabled: Bool = false, action: @escaping () -> Void) {
        self.symbol = symbol
        self.title = title
        self.checked = checked
        self.disabled = disabled
        self.action = action
    }
}

/// The glass bar over the keyboard, made for a remote screen.
@MainActor
enum UIKitAccessory {
    static func make(for model: RemoteModel, onHide: @escaping () -> Void) -> UIView {
        GlassAccessoryView(content: RemoteKeyBar(model: model, onHide: onHide))
    }
}

/// A login or a password for the sign-in or UAC screen of the computer, typed as it is (not kept anywhere).
struct SignInTypingSheet: View {
    let remote: RemoteModel
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var after = ""

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    SecureField("remote.signin.field", text: $text)
                        .textContentType(.password)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    Picker("remote.signin.after", selection: $after) {
                        Text("remote.signin.none").tag("")
                        Text("remote.signin.tab").tag("tab")
                        Text("remote.signin.enter").tag("enter")
                    }
                } footer: {
                    Text("remote.signin.hint")
                }
            }
            .navigationTitle("remote.signin.title")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("common.cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("remote.signin.type") {
                        _ = remote.typeOnSignIn(text, after: after)
                        text = ""
                        dismiss()
                    }
                    .disabled(text.isEmpty)
                }
            }
        }
        .presentationSizing(.form)
    }
}
