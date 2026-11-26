import CLinagerRdp
import Foundation
import LinagerKit

/// What the callbacks of the RDP core reach: they come from its own thread, with this object as the `user` pointer.
private final class EngineContext {
    let send: @Sendable (Data) -> Void
    let event: @Sendable (RdpEngineEvent) -> Void

    init(send: @escaping @Sendable (Data) -> Void, event: @escaping @Sendable (RdpEngineEvent) -> Void) {
        self.send = send
        self.event = event
    }
}

private func engineSend(_ user: UnsafeMutableRawPointer?, _ data: UnsafePointer<UInt8>?, _ len: Int) {
    guard let user, let data else { return }
    Unmanaged<EngineContext>.fromOpaque(user).takeUnretainedValue().send(Data(bytes: data, count: len))
}

private func engineEvent(_ user: UnsafeMutableRawPointer?, _ event: UnsafePointer<lrdp_event>?) {
    guard let user, let e = event?.pointee else { return }
    let ctx = Unmanaged<EngineContext>.fromOpaque(user).takeUnretainedValue()
    switch e.kind {
    case Int32(LRDP_CONNECTED):
        ctx.event(.connected(width: Int(e.w), height: Int(e.h)))
    case Int32(LRDP_FRAME):
        guard let data = e.data else { return }
        ctx.event(.frame(x: Int(e.x), y: Int(e.y), w: Int(e.w), h: Int(e.h), rgba: Data(bytes: data, count: e.len)))
    case Int32(LRDP_ENDED):
        ctx.event(.ended(e.text.map { String(cString: $0) } ?? ""))
    default:
        break
    }
}

/// The RDP core written in Rust (IronRDP, `ios/rdp`) behind `RdpEngine`.
final class RustRdpEngine: RdpEngine, @unchecked Sendable {
    private let lock = NSLock()
    private var session: OpaquePointer?
    private var context: Unmanaged<EngineContext>?

    func start(_ params: RdpParams, send: @escaping @Sendable (Data) -> Void, event: @escaping @Sendable (RdpEngineEvent) -> Void) -> Bool {
        let ctx = Unmanaged.passRetained(EngineContext(send: send, event: event))
        let width = UInt16(clamping: params.width), height = UInt16(clamping: params.height)
        let created: OpaquePointer? = params.username.withCString { user in
            params.password.withCString { pass in
                params.domain.withCString { domain in
                    params.destination.withCString { dest in
                        "linager".withCString { auth in
                            var p = lrdp_params(username: user, password: pass, domain: domain, destination: dest, proxy_auth: auth, width: width, height: height)
                            return lrdp_start(&p, engineSend, engineEvent, ctx.toOpaque())
                        }
                    }
                }
            }
        }
        guard let created else {
            ctx.release()
            return false
        }
        lock.withLock {
            session = created
            context = ctx
        }
        return true
    }

    func feed(_ data: Data) {
        guard let s = lock.withLock({ session }) else { return }
        data.withUnsafeBytes { raw in
            if let base = raw.bindMemory(to: UInt8.self).baseAddress { lrdp_feed(s, base, data.count) }
        }
    }

    func feedEnd() {
        guard let s = lock.withLock({ session }) else { return }
        lrdp_feed_end(s)
    }

    func input(_ input: AssistInput) {
        guard let s = lock.withLock({ session }) else { return }
        switch input {
        case .move(let x, let y): lrdp_mouse_move(s, UInt16(clamping: x), UInt16(clamping: y))
        case .button(let b, let down, let x, let y):
            // the pointer is where the finger is before the button goes
            lrdp_mouse_move(s, UInt16(clamping: x), UInt16(clamping: y))
            lrdp_mouse_button(s, Int32(b), down ? 1 : 0)
        case .wheel(let dx, let dy): lrdp_wheel(s, Int32(dx), Int32(dy))
        case .key(let sc, let ext, let down): lrdp_key(s, UInt8(clamping: sc), ext ? 1 : 0, down ? 1 : 0)
        case .unicode(let scalar, let down): lrdp_unicode(s, scalar, down ? 1 : 0)
        case .ctrlAltDel: break // not a key of the remote desktop (see RemoteModel.ctrlAltDel)
        }
    }

    func stop() {
        let (s, ctx) = lock.withLock { () -> (OpaquePointer?, Unmanaged<EngineContext>?) in
            defer { session = nil; context = nil }
            return (session, context)
        }
        guard let s else { return }
        lrdp_stop(s) // the thread of the session ends here; its last callbacks come before this returns
        ctx?.release()
    }
}
