import Foundation
import LinagerKit

/// A string of the catalog with values put in (the keys with %@ and %lld in Localizable.xcstrings).
func L(_ key: String, _ args: CVarArg...) -> String {
    String(format: NSLocalizedString(key, comment: ""), arguments: args)
}

/// Numbers and times for the screen, in the language and the units of the phone.
enum Fmt {
    /// "40d 4h" / "40 д. 4 ч": at most two units, at least a minute.
    static func duration(_ seconds: TimeInterval) -> String {
        let f = DateComponentsFormatter()
        f.unitsStyle = .abbreviated
        f.allowedUnits = [.day, .hour, .minute]
        f.maximumUnitCount = 2
        return f.string(from: max(60, seconds)) ?? ""
    }

    /// One unit: "12 min".
    static func ago(_ date: Date, now: Date = .now) -> String {
        let f = DateComponentsFormatter()
        f.unitsStyle = .abbreviated
        f.allowedUnits = [.day, .hour, .minute]
        f.maximumUnitCount = 1
        return f.string(from: max(60, now.timeIntervalSince(date))) ?? ""
    }

    static func rate(_ bytesPerSecond: Double) -> String {
        L("unit.perSecond", ByteCountFormatter.string(fromByteCount: Int64(max(0, bytesPerSecond)), countStyle: .binary))
    }

    static func time(_ date: Date) -> String {
        date.formatted(date: .omitted, time: .shortened)
    }
}

extension Level {
    var title: String {
        switch self {
        case .ok: L("st.ok")
        case .warn: L("st.warn")
        case .crit: L("st.crit")
        case .off: L("st.off")
        }
    }
}

extension LinagerError {
    /// What the screen says about a failure. `codeStep` is the second step of the sign-in, where a refused code is not "wrong password".
    func text(codeStep: Bool = false) -> String {
        switch self {
        case .unreachable(let detail): L("err.unreachable") + "\n" + detail
        case .pinMismatch: L("err.pinMismatch")
        case .accessDenied: L("err.accessDenied")
        case .invalidCredentials: L(codeStep ? "err.badCode" : "err.badLogin")
        case .totpRequired: L("err.badCode")
        case .twoFactorSetupRequired: L("err.setup2fa")
        case .rateLimited: L("err.rateLimited")
        case .unauthorized: L("err.sessionEnded")
        case .server(let status, _): L("err.server", status)
        case .badResponse: L("err.badResponse")
        }
    }
}
