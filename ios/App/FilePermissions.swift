import LinagerKit
import SwiftUI

/// Permissions and owner of one entry: the nine bits as a grid (who × read, write, execute), the special bits, the octal number, the owner and
/// the group. A system folder (`protected`) is shown but not changed. A change that sets a special bit is asked about first.
struct FilePermissionsSheet: View {
    let card: ServerCard
    let path: String
    let done: () -> Void
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var stat: FileStat?
    @State private var failure: LinagerError?
    @State private var mode = FileMode()
    @State private var owner = ""
    @State private var group = ""
    @State private var recursive = false
    @State private var octal = ""
    @State private var saving = false
    @State private var confirming = false
    @State private var note: String?

    private var name: String { (path as NSString).lastPathComponent }
    private var changedMode: Bool { stat.flatMap { FileMode(octal: $0.mode) }.map { $0 != mode } ?? false }
    private var changedOwner: Bool { stat.map { $0.owner != owner || $0.group != group } ?? false }
    private var canApply: Bool { stat?.protected == false && (changedMode || changedOwner || (recursive && stat?.isFolder == true)) && !saving && !owner.isEmpty && !group.isEmpty }

    var body: some View {
        NavigationStack {
            Group {
                if let stat { form(stat) } else if let failure {
                    ContentUnavailableView {
                        Label("srv.files.failed", systemImage: "exclamationmark.triangle")
                    } description: { Text(failure.text()) } actions: { Button("common.retry") { Task { await load() } } }
                } else { ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity) }
            }
            .navigationTitle(L("srv.perm.title"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("common.close") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("srv.perm.apply") { if mode.isDangerous && changedMode { confirming = true } else { Task { await apply() } } }
                        .disabled(!canApply)
                }
            }
        }
        .presentationDetents([.large])
        .task { await load() }
        .confirmationDialog("srv.perm.danger.title", isPresented: $confirming, titleVisibility: .visible) {
            Button("srv.perm.apply", role: .destructive) { Task { await apply() } }
            Button("common.cancel", role: .cancel) {}
        } message: { Text(L("srv.perm.danger.text", mode.octal)) }
    }

    private func form(_ s: FileStat) -> some View {
        Form {
            Section {
                LabeledContent("srv.perm.name") { Text(name).lineLimit(1).truncationMode(.middle) }
                LabeledContent("srv.perm.current") { Text(s.modeStr).font(.system(.body, design: .monospaced)) }
                if s.protected { Label("srv.perm.protected", systemImage: "lock.fill").foregroundStyle(.secondary) }
            }
            Section {
                Grid(horizontalSpacing: 0, verticalSpacing: 6) {
                    GridRow {
                        Color.clear.frame(height: 1)
                        ForEach(FileMode.What.allCases, id: \.rawValue) { w in
                            Text(whatTitle(w)).font(.caption.weight(.semibold)).foregroundStyle(.secondary).frame(maxWidth: .infinity)
                        }
                    }
                    ForEach(FileMode.Who.allCases, id: \.rawValue) { who in
                        GridRow {
                            Text(whoTitle(who)).frame(maxWidth: .infinity, alignment: .leading)
                            ForEach(FileMode.What.allCases, id: \.rawValue) { w in
                                Toggle("", isOn: bit(w, who)).labelsHidden().frame(maxWidth: .infinity)
                            }
                        }
                    }
                }
                .disabled(s.protected)
            } header: { Text("srv.perm.access") }
            Section {
                Toggle("srv.perm.setuid", isOn: special(.setuid))
                Toggle("srv.perm.setgid", isOn: special(.setgid))
                Toggle("srv.perm.sticky", isOn: special(.sticky))
            } header: { Text("srv.perm.special") } footer: { Text("srv.perm.specialHint") }
                .disabled(s.protected)
            Section {
                HStack {
                    Text("srv.perm.octal")
                    Spacer()
                    TextField("0644", text: $octal)
                        .font(.system(.body, design: .monospaced))
                        .multilineTextAlignment(.trailing)
                        .keyboardType(.numberPad)
                        .frame(width: 90)
                        .onChange(of: octal) { _, v in if let m = FileMode(octal: v), m != mode, v.count >= 3 { mode = m } }
                }
                LabeledContent("srv.perm.symbolic") { Text(mode.symbolic).font(.system(.body, design: .monospaced)) }
            }
            .disabled(s.protected)
            Section {
                HStack { Text("srv.perm.owner"); Spacer(); TextField("root", text: $owner).multilineTextAlignment(.trailing).textInputAutocapitalization(.never).autocorrectionDisabled() }
                HStack { Text("srv.perm.group"); Spacer(); TextField("root", text: $group).multilineTextAlignment(.trailing).textInputAutocapitalization(.never).autocorrectionDisabled() }
            } header: { Text("srv.perm.ownership") }
                .disabled(s.protected)
            if s.isFolder && !s.protected {
                Section {
                    Toggle("srv.perm.recursive", isOn: $recursive)
                } footer: { Text("srv.perm.recursiveHint") }
            }
            if let note { Section { Text(note).foregroundStyle(.red) } }
        }
        .onChange(of: mode) { _, m in if FileMode(octal: octal) != m { octal = m.octal } }
    }

    private func bit(_ w: FileMode.What, _ who: FileMode.Who) -> Binding<Bool> {
        Binding(get: { mode.has(w, for: who) }, set: { mode.set(w, for: who, $0) })
    }

    private func special(_ s: FileMode.Special) -> Binding<Bool> {
        Binding(get: { mode.has(s) }, set: { mode.set(s, $0) })
    }

    private func whoTitle(_ w: FileMode.Who) -> String {
        switch w {
        case .owner: L("srv.perm.owner")
        case .group: L("srv.perm.group")
        case .others: L("srv.perm.others")
        }
    }

    private func whatTitle(_ w: FileMode.What) -> String {
        switch w {
        case .read: L("srv.perm.read")
        case .write: L("srv.perm.write")
        case .execute: L("srv.perm.exec")
        }
    }

    private func load() async {
        failure = nil
        do {
            let s = try await model.stat(serverID: card.id, path: path)
            stat = s
            mode = FileMode(octal: s.mode) ?? FileMode()
            octal = mode.octal
            owner = s.owner
            group = s.group
        } catch let e as LinagerError {
            failure = e
        } catch {
            failure = .unreachable(error.localizedDescription)
        }
    }

    private func apply() async {
        guard let s = stat, !saving else { return }
        saving = true
        note = nil
        defer { saving = false }
        do {
            let all = recursive && s.isFolder
            if changedMode || all { try await model.setPermissions(serverID: card.id, path: path, mode: mode.octal, recursive: all) }
            if changedOwner || all { try await model.setOwner(serverID: card.id, path: path, owner: owner, group: group, recursive: all) }
            done()
            dismiss()
        } catch let e as LinagerError {
            note = e.text()
        } catch {
            note = error.localizedDescription
        }
    }
}
