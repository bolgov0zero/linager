import Foundation
import Security

/// What the phone keeps about the one server it is connected to.
public struct Profile: Codable, Equatable, Sendable {
    public var endpoint: Endpoint
    /// The pinned certificate, "SHA256:<base64>".
    public var fingerprint: String
    public var login: String?
    /// The session cookie of the server; nil until the sign-in.
    public var token: String?
    public var csrf: String?

    public init(endpoint: Endpoint, fingerprint: String, login: String? = nil, token: String? = nil, csrf: String? = nil) {
        self.endpoint = endpoint
        self.fingerprint = fingerprint
        self.login = login
        self.token = token
        self.csrf = csrf
    }

    public var credentials: Credentials { Credentials(token: token, csrf: csrf) }
}

public protocol ProfileStore: Sendable {
    func load() -> Profile?
    func save(_ profile: Profile)
    func clear()
}

/// The profile in the Keychain: it holds the pin and the session, so it never leaves the phone (not in backups, not to other devices).
public struct KeychainProfileStore: ProfileStore {
    private let service: String
    private let account = "profile"

    public init(service: String = "app.linager.ios") { self.service = service }

    private var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
    }

    public func load() -> Profile? {
        var q = query
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data else { return nil }
        return try? JSONDecoder().decode(Profile.self, from: data)
    }

    public func save(_ profile: Profile) {
        guard let data = try? JSONEncoder().encode(profile) else { return }
        let update = [kSecValueData as String: data]
        if SecItemUpdate(query as CFDictionary, update as CFDictionary) == errSecItemNotFound {
            var add = query
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
            SecItemAdd(add as CFDictionary, nil)
        }
    }

    public func clear() { SecItemDelete(query as CFDictionary) }
}

/// For the tests.
public final class MemoryProfileStore: ProfileStore, @unchecked Sendable {
    private let lock = NSLock()
    private var profile: Profile?

    public init(_ profile: Profile? = nil) { self.profile = profile }

    public func load() -> Profile? { lock.withLock { profile } }
    public func save(_ p: Profile) { lock.withLock { profile = p } }
    public func clear() { lock.withLock { profile = nil } }
}
