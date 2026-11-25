import LinagerKit
import SwiftUI

struct ConnectView: View {
    @Environment(AppModel.self) private var model
    @State private var address = ""
    @State private var port = ""
    @FocusState private var focus: Field?

    private enum Field { case address, port }

    var body: some View {
        Form {
            Section {
                VStack(spacing: 6) {
                    Image("Logo").resizable().interpolation(.none).frame(width: 88, height: 88)
                        .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
                        .shadow(color: .black.opacity(0.18), radius: 8, y: 4)
                        .padding(.bottom, 8)
                        .accessibilityHidden(true)
                    Text("Linager").font(.largeTitle.weight(.bold))
                    Text("connect.subtitle").foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 20)
            }
            .listRowBackground(Color.clear)

            Section {
                LabeledContent("connect.address") {
                    TextField("connect.addressPlaceholder", text: $address)
                        .multilineTextAlignment(.trailing)
                        .keyboardType(.URL)
                        .textContentType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.next)
                        .focused($focus, equals: .address)
                        .onSubmit { focus = .port }
                }
                LabeledContent("connect.port") {
                    TextField("connect.portPlaceholder", text: $port)
                        .multilineTextAlignment(.trailing)
                        .keyboardType(.numberPad)
                        .focused($focus, equals: .port)
                }
            } footer: {
                Text("connect.portHint")
            }

            if let e = model.connectError {
                Section {
                    Label(e.text(), systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).font(.subheadline).textSelection(.enabled)
                }
            }

            Section {
                Button(action: submit) {
                    HStack(spacing: 8) {
                        if model.probing { ProgressView() }
                        Text(LocalizedStringKey(model.probing ? "connect.checking" : "connect.button"))
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(model.probing || address.trimmingCharacters(in: .whitespaces).isEmpty)
            }
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
        }
        .scrollDismissesKeyboard(.interactively)
        .sheet(item: Binding(get: { model.pending }, set: { if $0 == nil { model.cancelTrust() } })) { pending in
            TrustSheet(pending: pending)
        }
    }

    private func submit() {
        focus = nil
        Task { await model.connect(address: address, port: port) }
    }
}

/// The certificate of a server that is not known yet. Nothing is kept until the person has compared the fingerprint with the one
/// the server shows (the installer, Settings, Connection) and confirms.
struct TrustSheet: View {
    let pending: PendingTrust
    @Environment(AppModel.self) private var model
    @State private var copied = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text("trust.title").font(.title2.weight(.bold))
                Text(L("trust.text", pending.endpoint.display)).font(.subheadline).foregroundStyle(.secondary)
                FingerprintBox(fingerprint: pending.fingerprint)
                Button {
                    UIPasteboard.general.string = pending.fingerprint
                    copied = true
                } label: {
                    Label(LocalizedStringKey(copied ? "common.copied" : "common.copy"), systemImage: copied ? "checkmark" : "doc.on.doc").frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
                Button { model.trust() } label: { Text("trust.confirm").frame(maxWidth: .infinity) }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .padding(.top, 4)
                Button(role: .cancel) { model.cancelTrust() } label: { Text("common.cancel").frame(maxWidth: .infinity) }
                    .controlSize(.large)
            }
            .padding(24)
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }
}

/// A fingerprint in lines, to be compared by eye.
struct FingerprintBox: View {
    let fingerprint: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(Fingerprint.lines(fingerprint), id: \.self) { line in
                Text(line).font(.system(.callout, design: .monospaced))
            }
        }
        .textSelection(.enabled)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .accessibilityElement(children: .combine)
    }
}
