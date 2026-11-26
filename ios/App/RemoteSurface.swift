import LinagerKit
import SwiftUI
import UIKit

enum RemoteZoom: String { case fit, width, actual }
enum RemoteTouchMode: String { case direct, pad }

/// The picture of the computer and everything done to it by a finger: clicks, drags, the wheel, the zoom, the keyboard.
///
/// One finger is followed by the touches themselves (not by gesture recognizers, which wait for a threshold before they begin and fight one
/// another): the pointer moves from the first point of the move. With the direct touch the pointer is where the finger is, with the touchpad
/// it moves by the finger and a tap clicks where the pointer is. A second tap that is held (a tap and at once a press) holds the left button
/// while the finger moves: a window is moved, a text is selected; lifted at once it is the second click of a double click. A long press is the
/// right button, and so is a tap of two fingers. Two fingers scroll the page (the wheel), or move a picture that is larger than the screen.
/// A pinch zooms.
final class RemoteSurfaceView: UIView, UIKeyInput, UIGestureRecognizerDelegate {
    weak var model: RemoteModel?

    var zoom: RemoteZoom = .fit { didSet { manualOrigin = nil; userScale = nil; layoutPicture(animated: true) } }
    /// The selection mode: a finger holds the left button from the moment it touches, so that moving it selects a text or drags a window.
    var selectMode = false
    var touchMode: RemoteTouchMode = .direct {
        didSet {
            guard touchMode != oldValue else { return }
            // the touchpad has its pointer on the screen all the time, the direct touch only shows where the finger was
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            pointerLayer.opacity = touchMode == .pad && pictureSize.width > 0 ? 1 : 0
            CATransaction.commit()
        }
    }

    private let picture = CALayer()
    private let pointerLayer = CAShapeLayer()
    private var pictureSize = CGSize.zero
    private var pointer = CGPoint.zero // in pixels of the picture
    private var userScale: CGFloat?
    private var manualOrigin: CGPoint?
    private var bottomInset: CGFloat = 0
    private var edgeLink: CADisplayLink?
    private var wheelRest = CGPoint.zero
    private var pointerHideTask: Task<Void, Never>?
    private let haptic = UIImpactFeedbackGenerator(style: .light)
    private var accessory: UIView?
    private var pinchStart: (scale: CGFloat, anchor: CGPoint)?
    private var lastMoveSent = CGPoint(x: -1, y: -1)
    private var pendingMove: CGPoint?
    private var lastMoveTime: TimeInterval = 0
    private var moveTask: Task<Void, Never>?

    /// The finger that is down now (a sequence of one finger; a second finger ends it).
    private struct Track {
        var start: CGPoint
        var last: CGPoint
        var began: TimeInterval
        var moved = false
        var buttonDown = false
        /// A second tap (or the selection mode): the finger is going to hold the left button, but the press waits for the first move, so that a plain
        /// second tap stays a double click and a drag is not taken by Windows for the second click of one.
        var armed = false
        var rightClicked = false
        var hold: Task<Void, Never>?
    }

    private var track: Track?
    private var lastTap: (time: TimeInterval, location: CGPoint)?
    /// The last left click sent, in pixels of the picture.
    private var lastClick: (time: TimeInterval, point: CGPoint)?
    /// More than one finger has been on the screen in this sequence of touches.
    private var multi = false
    /// The fingers that are down now, counted here: `allTouches` of an event keeps touches that have ended.
    private var down = Set<ObjectIdentifier>()

    override init(frame: CGRect) {
        super.init(frame: frame)
        backgroundColor = .black
        clipsToBounds = true
        picture.magnificationFilter = .linear
        picture.minificationFilter = .trilinear
        picture.backgroundColor = UIColor.black.cgColor
        layer.addSublayer(picture)

        let arrow = UIBezierPath()
        arrow.move(to: CGPoint(x: 0, y: 0))
        arrow.addLine(to: CGPoint(x: 0, y: 16))
        arrow.addLine(to: CGPoint(x: 4, y: 12.5))
        arrow.addLine(to: CGPoint(x: 6.8, y: 18.5))
        arrow.addLine(to: CGPoint(x: 9, y: 17.5))
        arrow.addLine(to: CGPoint(x: 6.2, y: 11.8))
        arrow.addLine(to: CGPoint(x: 11, y: 11.5))
        arrow.close()
        pointerLayer.path = arrow.cgPath
        pointerLayer.fillColor = UIColor.white.cgColor
        pointerLayer.strokeColor = UIColor.black.cgColor
        pointerLayer.lineWidth = 1
        pointerLayer.shadowOpacity = 0.5
        pointerLayer.shadowRadius = 1.5
        pointerLayer.shadowOffset = .zero
        pointerLayer.opacity = 0
        layer.addSublayer(pointerLayer)

        let twoTap = UITapGestureRecognizer(target: self, action: #selector(twoFingerTap(_:)))
        twoTap.numberOfTouchesRequired = 2
        addGesture(twoTap)
        let twoPan = UIPanGestureRecognizer(target: self, action: #selector(twoFingerPan(_:)))
        twoPan.minimumNumberOfTouches = 2
        twoPan.maximumNumberOfTouches = 2
        addGesture(twoPan)
        addGesture(UIPinchGestureRecognizer(target: self, action: #selector(pinch(_:))))

        NotificationCenter.default.addObserver(self, selector: #selector(keyboardChanged(_:)), name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
        NotificationCenter.default.addObserver(self, selector: #selector(keyboardChanged(_:)), name: UIResponder.keyboardWillHideNotification, object: nil)
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    deinit { NotificationCenter.default.removeObserver(self) }

    private func addGesture(_ g: UIGestureRecognizer) {
        g.delegate = self
        g.cancelsTouchesInView = false // the touches go on to the view: it follows the finger itself
        // and at once: by default UIKit holds back the lifting of a finger until the recognizers have failed, so that a second tap could begin while the
        // first was still counted as down (and be taken for a second finger)
        g.delaysTouchesEnded = false
        g.delaysTouchesBegan = false
        addGestureRecognizer(g)
    }

    func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
        // a pinch moves the picture too
        (g is UIPinchGestureRecognizer && other is UIPanGestureRecognizer) || (g is UIPanGestureRecognizer && other is UIPinchGestureRecognizer)
    }

    // MARK: picture

    func setImage(_ image: CGImage?, size: CGSize) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        if size != pictureSize {
            pictureSize = size
            pointer = CGPoint(x: size.width / 2, y: size.height / 2)
            manualOrigin = nil
            userScale = nil
            picture.contents = nil
            layoutPicture(animated: false)
        }
        if let image { picture.contents = image }
        CATransaction.commit()
    }

    /// The part of the view the picture may take: inside the corners of the phone, and above the keyboard.
    private var area: CGRect {
        let i = safeAreaInsets
        return CGRect(x: i.left, y: 0, width: bounds.width - i.left - i.right, height: max(1, bounds.height - max(i.bottom, bottomInset)))
    }

    private var baseScale: CGFloat {
        guard pictureSize.width > 0, pictureSize.height > 0 else { return 1 }
        let a = area
        switch zoom {
        case .fit:
            // with the keyboard up the picture is not shrunk into what is left (it would not be readable): it fits the width, and the pointer scrolls it at the edges
            return bottomInset > 0 ? a.width / pictureSize.width : min(a.width / pictureSize.width, a.height / pictureSize.height)
        case .width: return a.width / pictureSize.width
        case .actual: return 1
        }
    }

    /// The least the picture may be: the whole of it in view, touching two opposite edges of the part of the screen it has. Smaller, it would hang
    /// in the black with all four sides free.
    private var minScale: CGFloat {
        guard pictureSize.width > 0, pictureSize.height > 0 else { return 0.05 }
        let a = area
        return min(a.width / pictureSize.width, a.height / pictureSize.height)
    }

    private var scale: CGFloat { max(minScale, min(8, userScale ?? baseScale)) }

    /// The top left corner of the picture in the view.
    private func origin(for scale: CGFloat) -> CGPoint {
        let a = area
        let w = pictureSize.width * scale, h = pictureSize.height * scale
        var x = a.minX + (a.width - w) / 2, y = a.minY + (a.height - h) / 2
        if w > a.width {
            x = a.minX + (manualOrigin.map { $0.x - a.minX } ?? 0)
            x = max(a.maxX - w, min(a.minX, x))
        }
        if h > a.height {
            y = a.minY + (manualOrigin.map { $0.y - a.minY } ?? 0)
            y = max(a.maxY - h, min(a.minY, y))
        }
        return CGPoint(x: x, y: y)
    }

    func layoutPicture(animated: Bool) {
        guard pictureSize.width > 0 else { return }
        let s = scale, o = origin(for: s)
        let apply = {
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            self.picture.frame = CGRect(origin: o, size: CGSize(width: self.pictureSize.width * s, height: self.pictureSize.height * s))
            self.placePointer()
            CATransaction.commit()
        }
        if animated { UIView.animate(withDuration: 0.25) { apply() } } else { apply() }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        layoutPicture(animated: false)
    }

    override func safeAreaInsetsDidChange() {
        super.safeAreaInsetsDidChange()
        layoutPicture(animated: false)
    }

    private func placePointer() {
        // no implicit animation: it would make the pointer glide to the finger a quarter of a second late
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        pointerLayer.position = CGPoint(x: picture.frame.minX + pointer.x * scale, y: picture.frame.minY + pointer.y * scale)
        pointerLayer.opacity = 1
        CATransaction.commit()
    }

    /// With the direct touch the pointer is only a mark of where the finger was, it goes away; with the touchpad it stays: it is what a tap clicks.
    private func scheduleHidingPointer() {
        pointerHideTask?.cancel()
        guard touchMode == .direct else { return }
        pointerHideTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(1500))
            if !Task.isCancelled {
                CATransaction.begin()
                CATransaction.setDisableActions(true)
                self?.pointerLayer.opacity = 0
                CATransaction.commit()
            }
        }
    }

    // MARK: coordinates

    private func toPicture(_ p: CGPoint) -> CGPoint {
        let s = scale
        let x = (p.x - picture.frame.minX) / s, y = (p.y - picture.frame.minY) / s
        return CGPoint(x: max(0, min(pictureSize.width - 1, x.rounded())), y: max(0, min(pictureSize.height - 1, y.rounded())))
    }

    /// The pointer is drawn at once; the move is sent to the computer at most about sixty times a second, the last one always.
    private func movePointer(to p: CGPoint) {
        pointer = p
        placePointer()
        pointerHideTask?.cancel()
        guard p != lastMoveSent else { return }
        let now = CACurrentMediaTime()
        if now - lastMoveTime >= 0.016 {
            sendMove(p)
        } else {
            pendingMove = p
            if moveTask == nil {
                moveTask = Task { [weak self] in
                    try? await Task.sleep(for: .milliseconds(16))
                    guard let self else { return }
                    moveTask = nil
                    if let q = pendingMove { sendMove(q) }
                }
            }
        }
    }

    private func sendMove(_ p: CGPoint) {
        pendingMove = nil
        lastMoveSent = p
        lastMoveTime = CACurrentMediaTime()
        model?.send(.move(x: Int(p.x), y: Int(p.y)))
    }

    private func flushMove() {
        if let q = pendingMove { sendMove(q) }
    }

    private func at(_ p: CGPoint) -> CGPoint { touchMode == .direct ? toPicture(p) : pointer }

    private func click(_ button: Int, at p: CGPoint) {
        movePointer(to: p)
        flushMove()
        if button == 0 { lastClick = (now(), p) }
        model?.send(.button(button, down: true, x: Int(p.x), y: Int(p.y)))
        model?.send(.button(button, down: false, x: Int(p.x), y: Int(p.y)))
        haptic.impactOccurred(intensity: button == 0 ? 0.5 : 0.9)
        ripple(at: picture.frame.origin.applying(CGAffineTransform(translationX: p.x * scale, y: p.y * scale)), right: button == 2)
    }

    private func ripple(at p: CGPoint, right: Bool, tint: UIColor? = nil) {
        let ring = CAShapeLayer()
        ring.path = UIBezierPath(ovalIn: CGRect(x: -9, y: -9, width: 18, height: 18)).cgPath
        ring.position = p
        let color = tint ?? (right ? UIColor.systemOrange : UIColor.white)
        ring.fillColor = (right || tint != nil ? color.withAlphaComponent(0.3) : UIColor.clear).cgColor
        ring.strokeColor = color.cgColor
        ring.lineWidth = 2
        layer.addSublayer(ring)
        let grow = CABasicAnimation(keyPath: "transform.scale")
        grow.fromValue = 0.5
        grow.toValue = 2.6
        let fade = CABasicAnimation(keyPath: "opacity")
        fade.fromValue = 1
        fade.toValue = 0
        let group = CAAnimationGroup()
        group.animations = [grow, fade]
        group.duration = 0.5
        group.isRemovedOnCompletion = true
        CATransaction.begin()
        CATransaction.setCompletionBlock { ring.removeFromSuperlayer() }
        ring.add(group, forKey: "ripple")
        ring.opacity = 0
        CATransaction.commit()
    }

    // MARK: gestures

    @objc private func twoFingerTap(_ g: UITapGestureRecognizer) {
        guard g.state == .ended else { return }
        click(2, at: at(g.location(in: self)))
    }

    // MARK: one finger, by the touches

    private func now() -> TimeInterval { CACurrentMediaTime() }

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesBegan(touches, with: event)
        for t in touches { down.insert(ObjectIdentifier(t)) }
        if down.count > 1 {
            // a second finger: this is a scroll, a pinch or a tap of two; the one finger is over
            multi = true
            if var t = track {
                t.hold?.cancel()
                if t.buttonDown { releaseButton() }
                track = nil
            }
            return
        }
        guard let touch = touches.first else { return }
        multi = false
        let loc = touch.location(in: self)
        var t = Track(start: loc, last: loc, began: now())
        // a second tap of a double tap (UIKit counts it, with its own time and distance) or a tap of our own count, or the selection mode
        let second = touch.tapCount >= 2 || (lastTap.map { now() - $0.time < 0.5 && hypot(loc.x - $0.location.x, loc.y - $0.location.y) < 60 } ?? false)
        if second || selectMode {
            // a tap and at once a press, or the selection mode: the left button will be held, from the first move of the finger. The press is not
            // sent yet: a second tap that is lifted without a move is a double click, and a drag has to start with a press Windows does not
            // take for the second click of one.
            lastTap = nil
            t.armed = true
            haptic.impactOccurred(intensity: 0.7)
            // a mark that the finger is armed: a blue ring where the pointer is (under the finger with the direct touch)
            let p = at(loc)
            ripple(at: CGPoint(x: picture.frame.minX + p.x * scale, y: picture.frame.minY + p.y * scale), right: false, tint: .systemBlue)
        } else {
            // a long press is the right button
            t.hold = Task { [weak self] in
                try? await Task.sleep(for: .milliseconds(500))
                guard !Task.isCancelled, let self, var cur = track, !cur.moved, !cur.buttonDown else { return }
                cur.rightClicked = true
                track = cur
                click(2, at: at(cur.start))
            }
        }
        track = t
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesMoved(touches, with: event)
        guard !multi, var t = track, let touch = touches.first else { return }
        // the finger of the sequence, at the finest points the screen gives
        let loc = touch.location(in: self)
        if !t.moved {
            guard hypot(loc.x - t.start.x, loc.y - t.start.y) > 4 else { return } // a tap does not move the pointer
            t.moved = true
            t.hold?.cancel()
            if t.armed && !t.buttonDown { beginDrag(&t) }
        }
        let dx = loc.x - t.last.x, dy = loc.y - t.last.y
        t.last = loc
        track = t
        if touchMode == .direct {
            movePointer(to: toPicture(loc))
        } else {
            // the touchpad: the pointer moves by the finger at an even rate: a little faster than it, and exactly as far as it while the button is
            // held, so that a selection can be led by a character
            let s = scale
            let gain: CGFloat = t.buttonDown ? 1.0 : 1.5
            movePointer(to: CGPoint(x: max(0, min(pictureSize.width - 1, pointer.x + dx * gain / s)), y: max(0, min(pictureSize.height - 1, pointer.y + dy * gain / s))))
        }
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesEnded(touches, with: event)
        for t in touches { down.remove(ObjectIdentifier(t)) }
        if down.isEmpty { multi = false }
        guard var t = track else { return }
        t.hold?.cancel()
        track = nil
        flushMove()
        if t.buttonDown {
            releaseButton()
        } else if t.armed && !t.moved {
            // the second tap lifted without a move: the second click of a double click, in the same point
            click(0, at: at(t.start))
        } else if !t.moved && !t.rightClicked && now() - t.began < 0.45 {
            let p = at(t.start)
            click(0, at: p)
            lastTap = (now(), t.start)
        }
        scheduleHidingPointer()
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        super.touchesCancelled(touches, with: event)
        for t in touches { down.remove(ObjectIdentifier(t)) }
        if down.isEmpty { multi = false }
        guard let t = track else { return }
        t.hold?.cancel()
        track = nil
        if t.buttonDown { releaseButton() }
    }

    /// The press that starts a drag: the left button goes down at the place the finger began (the pointer, with the touchpad).
    private func beginDrag(_ t: inout Track) {
        var p = at(t.start)
        // Windows takes a press near the last click, soon after it, for the second click of a double click, and what follows a double click is not
        // a drag (nothing is selected, no frame is drawn on the desktop): the press is moved four pixels aside, where it is a press of its own
        if let lc = lastClick, now() - lc.time < 1.0, abs(p.x - lc.point.x) <= 4, abs(p.y - lc.point.y) <= 4 {
            p.y += p.y + 5 < pictureSize.height ? 4 : -4
        }
        movePointer(to: p)
        flushMove()
        lastClick = (now(), p)
        model?.send(.button(0, down: true, x: Int(p.x), y: Int(p.y)))
        t.buttonDown = true
    }

    private func releaseButton() {
        flushMove()
        model?.send(.button(0, down: false, x: Int(pointer.x), y: Int(pointer.y)))
    }

    @objc private func twoFingerPan(_ g: UIPanGestureRecognizer) {
        switch g.state {
        case .began:
            wheelRest = .zero
        case .changed:
            let t = g.translation(in: self)
            g.setTranslation(.zero, in: self)
            let a = area, s = scale
            let overflowX = pictureSize.width * s > a.width + 1, overflowY = pictureSize.height * s > a.height + 1
            // what does not fit moves the picture, what does fit is the wheel of the mouse
            var wheelDX: CGFloat = 0, wheelDY: CGFloat = 0
            var o = manualOrigin ?? origin(for: s)
            if overflowX { o.x += t.x } else { wheelDX = -t.x }
            if overflowY { o.y += t.y } else { wheelDY = -t.y }
            if overflowX || overflowY {
                manualOrigin = o
                layoutPicture(animated: false)
            }
            wheelRest.x += wheelDX / 28
            wheelRest.y += wheelDY / 28
            let nx = Int(wheelRest.x.rounded(.towardZero)), ny = Int(wheelRest.y.rounded(.towardZero))
            if nx != 0 || ny != 0 {
                wheelRest.x -= CGFloat(nx)
                wheelRest.y -= CGFloat(ny)
                model?.send(.wheel(dx: nx, dy: ny))
            }
        default: break
        }
    }

    @objc private func pinch(_ g: UIPinchGestureRecognizer) {
        let c = g.location(in: self)
        switch g.state {
        case .began:
            let s = scale
            pinchStart = (s, CGPoint(x: (c.x - picture.frame.minX) / s, y: (c.y - picture.frame.minY) / s))
        case .changed:
            guard let st = pinchStart else { return }
            let s = max(minScale, min(8, st.scale * g.scale))
            userScale = s
            manualOrigin = CGPoint(x: c.x - st.anchor.x * s, y: c.y - st.anchor.y * s)
            layoutPicture(animated: false)
        case .ended, .cancelled:
            pinchStart = nil
            // a pinch back to the whole picture is the "fit" again
            if let s = userScale, abs(s - baseScale) < 0.02 { userScale = nil; manualOrigin = nil; layoutPicture(animated: true) }
        default: break
        }
    }

    // MARK: keyboard

    override var canBecomeFirstResponder: Bool { true }
    override var inputAccessoryView: UIView? { accessory }

    func setAccessory(_ view: UIView?) { accessory = view }

    var hasText: Bool { true }

    func insertText(_ text: String) { model?.typeText(text) }

    func deleteBackward() { model?.press(ScanCodes.backspace) }

    // UITextInputTraits: what is typed goes to another computer as it is
    var autocorrectionType: UITextAutocorrectionType { get { .no } set {} }
    var autocapitalizationType: UITextAutocapitalizationType { get { .none } set {} }
    var spellCheckingType: UITextSpellCheckingType { get { .no } set {} }
    var smartQuotesType: UITextSmartQuotesType { get { .no } set {} }
    var smartDashesType: UITextSmartDashesType { get { .no } set {} }
    var smartInsertDeleteType: UITextSmartInsertDeleteType { get { .no } set {} }
    var keyboardType: UIKeyboardType { get { .default } set {} }

    func toggleKeyboard() {
        if isFirstResponder { resignFirstResponder() } else { becomeFirstResponder() }
    }

    @objc private func keyboardChanged(_ n: Notification) {
        guard window != nil else { return }
        if n.name == UIResponder.keyboardWillHideNotification {
            bottomInset = 0
        } else if let end = (n.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? NSValue)?.cgRectValue {
            let local = convert(end, from: nil)
            bottomInset = max(0, bounds.maxY - local.minY)
        }
        model?.keyboardChanged(shown: bottomInset > 0)
        if bottomInset > 0 {
            // the keyboard has just come up: the picture is moved once so that the pointer is in the middle of what is left above it, then it
            // stays where it is and the pointer scrolls it at the edges
            if edgeLink == nil {
                let a = area, s = scale
                manualOrigin = CGPoint(x: a.midX - pointer.x * s, y: a.midY - pointer.y * s)
                startEdgeScroll()
            }
        } else {
            manualOrigin = nil
            stopEdgeScroll()
        }
        layoutPicture(animated: true)
    }

    // MARK: scrolling at the edges (with the keyboard up the picture goes under it: the pointer at an edge moves the picture)

    @MainActor private final class EdgeTarget: NSObject {
        weak var view: RemoteSurfaceView?
        init(_ view: RemoteSurfaceView) { self.view = view }
        @objc func tick(_ link: CADisplayLink) { view?.edgeTick(link) }
    }

    private func startEdgeScroll() {
        guard edgeLink == nil else { return }
        let link = CADisplayLink(target: EdgeTarget(self), selector: #selector(EdgeTarget.tick(_:)))
        link.add(to: .main, forMode: .common)
        edgeLink = link
    }

    private func stopEdgeScroll() {
        edgeLink?.invalidate()
        edgeLink = nil
    }

    override func willMove(toWindow newWindow: UIWindow?) {
        super.willMove(toWindow: newWindow)
        if newWindow == nil { stopEdgeScroll() }
    }

    private func edgeTick(_ link: CADisplayLink) {
        // the touchpad keeps its pointer, so a pointer left at an edge goes on scrolling; the direct touch has no pointer to leave, so it scrolls
        // only while the finger is down
        guard bottomInset > 0, pictureSize.width > 0, touchMode == .pad || track != nil, pinchStart == nil else { return }
        let a = area, s = scale, f = picture.frame
        // a pointer under the keyboard is brought to the edge of what is seen
        let p = CGPoint(x: min(a.maxX, max(a.minX, f.minX + pointer.x * s)), y: min(a.maxY, max(a.minY, f.minY + pointer.y * s)))
        let k = max(0.5, min(3, (link.targetTimestamp - link.timestamp) * 60)) // a frame of other length than 1/60 s
        let vx = CGFloat(EdgeScroll.speed(pos: Double(p.x - a.minX), size: Double(a.width))) * k
        let vy = CGFloat(EdgeScroll.speed(pos: Double(p.y - a.minY), size: Double(a.height))) * k
        let outside = p != CGPoint(x: f.minX + pointer.x * s, y: f.minY + pointer.y * s)
        guard vx != 0 || vy != 0 || outside else { return }
        let w = pictureSize.width * s, h = pictureSize.height * s
        var o = CGPoint(x: f.minX - vx, y: f.minY - vy)
        o.x = w > a.width ? max(a.maxX - w, min(a.minX, o.x)) : f.minX
        o.y = h > a.height ? max(a.maxY - h, min(a.minY, o.y)) : f.minY
        guard o != f.origin || outside else { return }
        manualOrigin = o
        layoutPicture(animated: false)
        // the picture moved under the pointer that stands still on the screen: the computer is told where its pointer is now
        let np = CGPoint(x: max(0, min(pictureSize.width - 1, ((p.x - o.x) / s).rounded())), y: max(0, min(pictureSize.height - 1, ((p.y - o.y) / s).rounded())))
        movePointer(to: np)
    }
}

struct RemoteSurface: UIViewRepresentable {
    let model: RemoteModel
    let zoom: RemoteZoom
    let touch: RemoteTouchMode
    /// The selection mode of the mouse.
    let select: Bool
    /// Becomes true to open the keyboard and false to close it.
    let keyboard: Bool
    let keyBar: @MainActor () -> UIView

    func makeUIView(context: Context) -> RemoteSurfaceView {
        let v = RemoteSurfaceView(frame: .zero)
        v.model = model
        v.setAccessory(keyBar())
        model.onFrame = { [weak v] image, size in v?.setImage(image, size: size) }
        return v
    }

    func updateUIView(_ v: RemoteSurfaceView, context: Context) {
        if v.zoom != zoom { v.zoom = zoom }
        v.touchMode = touch
        v.selectMode = select
        if keyboard != v.isFirstResponder {
            if keyboard { _ = v.becomeFirstResponder() } else { _ = v.resignFirstResponder() }
        }
    }
}
