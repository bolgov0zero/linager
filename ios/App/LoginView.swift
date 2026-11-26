import LinagerKit
import SwiftUI

struct LoginView: View {
    @Environment(AppModel.self) private var model
    @State private var login = ""
    @State private var password = ""
    @State private var code = ""
    @State private var backup = false
    @State private var confirmForget = false
    @FocusState private var focus: Field?

    private enum Field { case login, password, backup }

    var body: some View {
        NavigationStack {
            Group {
                if model.needsCode { codeStep } else { passwordStep }
            }
            .navigationTitle(LocalizedStringKey(model.needsCode ? "totp.title" : "login.title"))
            .navigationSubtitle(model.needsCode ? L("totp.subtitle") : (model.endpoint?.display ?? ""))
            .navigationBarTitleDisplayMode(.large)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: back) {
                        Label(LocalizedStringKey(model.needsCode ? "login.title" : "login.back"), systemImage: "chevron.backward").labelStyle(.titleAndIcon)
                    }
                }
            }
            .confirmationDialog("acc.forget.title", isPresented: $confirmForget, titleVisibility: .visible) {
                Button("acc.forget", role: .destructive) { Task { await model.forgetServer() } }
                Button("common.cancel", role: .cancel) {}
            } message: {
                Text("acc.forget.text")
            }
        }
        .task { if login.isEmpty { login = model.rememberedLogin ?? "" } }
        .onChange(of: model.loginError) { _, e in
            // a refused code is typed again from scratch
            if e != nil, model.needsCode { code = "" }
        }
    }

    // MARK: steps

    private var passwordStep: some View {
        Form {
            Section {
                TextField("login.user", text: $login)
                    .textContentType(.username)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.next)
                    .focused($focus, equals: .login)
                    .onSubmit { focus = .password }
                SecureField("login.password", text: $password)
                    .textContentType(.password)
                    .submitLabel(.go)
                    .focused($focus, equals: .password)
                    .onSubmit(submit)
            } footer: {
                HStack(spacing: 6) {
                    Image(systemName: "checkmark.shield.fill").foregroundStyle(.green)
                    Text("login.pinned")
                }
            }
            errorSection
            Section {
                signInButton("login.button", disabled: login.trimmingCharacters(in: .whitespaces).isEmpty || password.isEmpty)
            }
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private var codeStep: some View {
        Form {
            Section {
                if backup {
                    TextField("totp.backupPlaceholder", text: $code)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .submitLabel(.go)
                        .focused($focus, equals: .backup)
                        .onSubmit(submit)
                } else {
                    CodeField(text: $code, onComplete: submit).frame(maxWidth: .infinity).padding(.vertical, 8)
                }
            }
            .listRowBackground(backup ? nil : Color.clear)
            errorSection
            if backup {
                Section {
                    signInButton("login.button", disabled: code.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }
            Section {
                Button(LocalizedStringKey(backup ? "totp.useApp" : "totp.backup")) {
                    backup.toggle()
                    code = ""
                    focus = backup ? .backup : nil
                }
                .frame(maxWidth: .infinity)
            }
            .listRowBackground(Color.clear)
        }
    }

    @ViewBuilder private var errorSection: some View {
        if let e = model.loginError {
            Section {
                Label(e.text(codeStep: model.needsCode), systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).font(.subheadline).textSelection(.enabled)
            }
        }
    }

    private func signInButton(_ title: LocalizedStringKey, disabled: Bool) -> some View {
        Button(action: submit) {
            HStack(spacing: 8) {
                if model.signingIn { ProgressView() }
                Text(title)
            }
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        .controlSize(.large)
        .disabled(disabled || model.signingIn)
    }

    // MARK: actions

    private func submit() {
        guard !model.signingIn else { return }
        focus = nil
        Task { await model.signIn(login: login, password: password, code: model.needsCode ? code : nil) }
    }

    private func back() {
        if model.needsCode {
            model.restartLogin()
            code = ""
            backup = false
        } else if model.rememberedLogin != nil {
            confirmForget = true // the server was used before: forgetting it erases the pin
        } else {
            Task { await model.forgetServer() }
        }
    }
}

/// Six boxes for the digits of a one-time code. A text field that cannot be seen takes the typing (and the code the phone
/// offers from its messages or from Passwords); the boxes show it.
struct CodeField: View {
    @Binding var text: String
    var onComplete: () -> Void
    @FocusState private var focused: Bool

    var body: some View {
        ZStack {
            TextField("", text: $text)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .focused($focused)
                .frame(width: 1, height: 1)
                .opacity(0.01)
                .accessibilityLabel(Text("totp.title"))
            HStack(spacing: 8) {
                ForEach(0..<6, id: \.self) { i in
                    let chars = Array(text)
                    Text(i < chars.count ? String(chars[i]) : "")
                        .font(.system(.title, design: .rounded).weight(.semibold))
                        .frame(width: 46, height: 56)
                        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: boxRadius, style: .continuous))
                        .overlay {
                            if focused && i == min(chars.count, 5) {
                                RoundedRectangle(cornerRadius: boxRadius, style: .continuous).strokeBorder(Color.accentColor, lineWidth: 2)
                            }
                        }
                }
            }
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
        .contentShape(Rectangle())
        .onTapGesture { focused = true }
        .onAppear { focused = true }
        .onChange(of: text) { _, new in
            let digits = String(new.filter(\.isNumber).prefix(6))
            if digits != new { text = digits }
            if digits.count == 6 { onComplete() }
        }
    }
}
