import SwiftUI
import UIKit

/// The application is portrait; only the remote screen turns the phone to landscape and back.
@MainActor
final class AppDelegate: NSObject, UIApplicationDelegate {
    /// What the system may rotate to now.
    static var orientations: UIInterfaceOrientationMask = .portrait

    func application(_ application: UIApplication, supportedInterfaceOrientationsFor window: UIWindow?) -> UIInterfaceOrientationMask {
        Self.orientations
    }
}

@MainActor
enum Orientation {
    /// The remote screen: landscape. The phone turns by itself, whatever the orientation lock of the phone says.
    static func landscape() { set(.landscape) }

    /// Back to the rest of the application.
    static func portrait() { set(.portrait) }

    private static func set(_ mask: UIInterfaceOrientationMask) {
        AppDelegate.orientations = mask
        guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else { return }
        scene.windows.first?.rootViewController?.setNeedsUpdateOfSupportedInterfaceOrientations()
        scene.requestGeometryUpdate(.iOS(interfaceOrientations: mask)) { _ in }
    }
}
