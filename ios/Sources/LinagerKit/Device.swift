import Foundation
import Security

/// What the application tells the server about itself (the header `X-Linager-Client`): the platform, the versions of the application and
/// of the system, and the model. The server shows it in the list of the devices.
public struct ClientInfo: Equatable, Sendable {
    public var platform: String
    public var app: String
    public var os: String
    public var model: String

    public init(platform: String, app: String, os: String, model: String) {
        self.platform = platform
        self.app = app
        self.os = os
        self.model = model
    }

    /// This installation: the version of the bundle, the version of the system, the identifier of the model ("iPhone18,1").
    public static var current: ClientInfo {
        let v = ProcessInfo.processInfo.operatingSystemVersion
        let info = Bundle.main.infoDictionary
        let short = info?["CFBundleShortVersionString"] as? String ?? ""
        let build = info?["CFBundleVersion"] as? String ?? ""
        #if os(iOS)
        let platform = "ios"
        #else
        let platform = "macos"
        #endif
        return ClientInfo(platform: platform, app: build.isEmpty ? short : "\(short) (\(build))", os: "\(v.majorVersion).\(v.minorVersion)" + (v.patchVersion > 0 ? ".\(v.patchVersion)" : ""), model: Self.modelIdentifier())
    }

    /// The header value, a query string: `platform=ios&app=0.1.0%20(55)&os=27.1&model=iPhone18%2C1`.
    public var header: String {
        var c = URLComponents()
        c.queryItems = [("platform", platform), ("app", app), ("os", os), ("model", model)].filter { !$0.1.isEmpty }.map { URLQueryItem(name: $0.0, value: $0.1) }
        // URLComponents leaves "," and "+" as they are in a query; the server reads a query string, so the signs that mean something are escaped
        return (c.percentEncodedQuery ?? "").replacingOccurrences(of: "+", with: "%2B").replacingOccurrences(of: ",", with: "%2C")
    }

    private static func modelIdentifier() -> String {
        var size = 0
        sysctlbyname("hw.machine", nil, &size, nil, 0)
        guard size > 0 else { return "" }
        var buf = [CChar](repeating: 0, count: size)
        sysctlbyname("hw.machine", &buf, &size, nil, 0)
        return String(cString: buf)
    }
}

/// The identity of this installation to one server: a secret token only this phone has, and what it says about itself. Every request carries
/// them. The server keeps only a hash of the token; with the confirmation of devices on, a device it does not know waits for an administrator.
public struct DeviceIdentity: Equatable, Sendable {
    public var token: String
    public var info: ClientInfo

    public init(token: String, info: ClientInfo = .current) {
        self.token = token
        self.info = info
    }

    public static let tokenHeader = "X-Linager-Device"
    public static let clientHeader = "X-Linager-Client"

    /// Puts the headers of the device on a request.
    public func apply(to request: inout URLRequest) {
        request.setValue(token, forHTTPHeaderField: Self.tokenHeader)
        request.setValue(info.header, forHTTPHeaderField: Self.clientHeader)
    }

    /// A new random token: 32 bytes, base64url without padding (43 characters).
    public static func makeToken() -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        let status = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        if status != errSecSuccess { bytes = (0..<32).map { _ in UInt8.random(in: 0...255) } }
        return Data(bytes).base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }
}

/// Keeps the token of the device for each server (the same phone is a device of its own on each server).
public protocol DeviceTokenStore: Sendable {
    /// The token for a server; made and kept at the first call.
    func token(for endpoint: Endpoint) -> String
    func forget(_ endpoint: Endpoint)
}

/// The tokens in the Keychain: they never leave the phone (not in backups, not to other devices) and are readable in the background after the first unlock.
public struct KeychainDeviceTokenStore: DeviceTokenStore {
    private let service: String

    public init(service: String = "app.linager.ios.device") { self.service = service }

    private func query(_ endpoint: Endpoint) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: endpoint.display]
    }

    public func token(for endpoint: Endpoint) -> String {
        var q = query(endpoint)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        if SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data, let s = String(data: data, encoding: .utf8), s.count >= 32 { return s }
        let fresh = DeviceIdentity.makeToken()
        var add = query(endpoint)
        add[kSecValueData as String] = Data(fresh.utf8)
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemDelete(query(endpoint) as CFDictionary)
        SecItemAdd(add as CFDictionary, nil)
        return fresh
    }

    public func forget(_ endpoint: Endpoint) { SecItemDelete(query(endpoint) as CFDictionary) }
}

/// For the tests.
public final class MemoryDeviceTokenStore: DeviceTokenStore, @unchecked Sendable {
    private let lock = NSLock()
    private var tokens: [String: String]

    public init(_ tokens: [String: String] = [:]) { self.tokens = tokens }

    public func token(for endpoint: Endpoint) -> String {
        lock.withLock {
            if let t = tokens[endpoint.display] { return t }
            let t = DeviceIdentity.makeToken()
            tokens[endpoint.display] = t
            return t
        }
    }

    public func forget(_ endpoint: Endpoint) { lock.withLock { _ = tokens.removeValue(forKey: endpoint.display) } }
}
