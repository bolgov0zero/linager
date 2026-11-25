import Foundation

/// Everything that can go wrong between the phone and the server, in the terms the screens speak.
public enum LinagerError: Error, Equatable, Sendable {
    /// The server cannot be reached (no network, refused, timeout). The text is for the log, not for the screen.
    case unreachable(String)
    /// The server showed another certificate than the pinned one. `seen` is the fingerprint it showed.
    case pinMismatch(seen: String?)
    /// The access restriction of the application port refused this address (a plain 403).
    case accessDenied
    case invalidCredentials
    /// The password was right and the account has a second step: ask for the code and repeat.
    case totpRequired
    /// The server requires two-step verification and this account has not turned it on (it is turned on in the desktop application).
    case twoFactorSetupRequired
    case rateLimited(String)
    /// There is no session (or it has ended).
    case unauthorized
    case server(status: Int, message: String)
    case badResponse
}

extension LinagerError {
    /// The error of an API answer: the status and the text of `{"error": "..."}` (or the plain text of the access guard).
    static func fromStatus(_ status: Int, message: String) -> LinagerError {
        switch (status, message) {
        case (401, "totp_required"): return .totpRequired
        case (401, "invalid credentials"): return .invalidCredentials
        case (401, _): return .unauthorized
        case (403, "2fa_required"): return .twoFactorSetupRequired
        case (403, "bad csrf token"): return .unauthorized
        case (403, _): return .accessDenied
        case (429, _): return .rateLimited(message)
        default: return .server(status: status, message: message)
        }
    }
}
