import MetalKit
import SwiftUI

/// The Metal view with the touch controls:
/// drag = look, tap = shockwave + chime, double-tap = new scene, hold = new colours,
/// two-finger drag = slide sideways (left/right) and tape speed (up/down), pinch = altitude.
final class GameMTKView: MTKView, UIGestureRecognizerDelegate {
    private var lastY: CGFloat = 0

    func setupGestures() {
        isMultipleTouchEnabled = true
        let look = UIPanGestureRecognizer(target: self, action: #selector(onLook))
        look.maximumNumberOfTouches = 1
        let two = UIPanGestureRecognizer(target: self, action: #selector(onTwo))
        two.minimumNumberOfTouches = 2
        let pinch = UIPinchGestureRecognizer(target: self, action: #selector(onPinch))
        let tap = UITapGestureRecognizer(target: self, action: #selector(onTap))
        let dbl = UITapGestureRecognizer(target: self, action: #selector(onDouble))
        dbl.numberOfTapsRequired = 2
        tap.require(toFail: dbl)
        let hold = UILongPressGestureRecognizer(target: self, action: #selector(onHold))
        hold.minimumPressDuration = 0.6
        for g in [look, two, pinch, tap, dbl, hold] as [UIGestureRecognizer] {
            g.delegate = self
            addGestureRecognizer(g)
        }
    }

    func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith o: UIGestureRecognizer) -> Bool {
        (g is UIPinchGestureRecognizer && o is UIPanGestureRecognizer) || (o is UIPinchGestureRecognizer && g is UIPanGestureRecognizer)
    }

    @objc private func onLook(_ g: UIPanGestureRecognizer) {
        let t = g.translation(in: self)
        ve3d_look(Float(t.x / bounds.width), Float(t.y / bounds.height))
        g.setTranslation(.zero, in: self)
    }

    @objc private func onTwo(_ g: UIPanGestureRecognizer) {
        let t = g.translation(in: self)
        switch g.state {
        case .began:
            lastY = 0
        case .changed:
            ve3d_strafe(Float(max(-1, min(1, t.x / (bounds.width * 0.18)))))
            ve3d_speed(Float(-(t.y - lastY) / bounds.height) * 1.2)
            lastY = t.y
        default:
            ve3d_strafe(0)
        }
    }

    @objc private func onPinch(_ g: UIPinchGestureRecognizer) {
        if g.scale > 0 { ve3d_altitude(Float(1 / g.scale)) }
        g.scale = 1
    }

    @objc private func onTap(_ g: UITapGestureRecognizer) {
        let p = g.location(in: self)
        ve3d_tap(Float(p.x / bounds.width), Float(p.y / bounds.height))
    }

    @objc private func onDouble(_ g: UITapGestureRecognizer) { ve3d_new_scene() }

    @objc private func onHold(_ g: UILongPressGestureRecognizer) {
        if g.state == .began { ve3d_recolour() }
    }
}

struct MetalView: UIViewRepresentable {
    let core: AppCore

    func makeUIView(context: Context) -> GameMTKView {
        let v = GameMTKView(frame: .zero)
        if let r = Renderer(view: v) {
            r.core = core
            core.renderer = r
            v.delegate = r
        }
        v.setupGestures()
        return v
    }

    func updateUIView(_ v: GameMTKView, context: Context) {
        v.isPaused = core.paused
    }
}
