import SwiftUI

/// Owns the Go core, the audio output and the HUD state.
final class AppCore: ObservableObject {
    let audio = AudioOutput()
    var renderer: Renderer?

    @Published var title = ""
    @Published var info = ""
    @Published var toast = ""
    @Published var titleOpacity = 1.0
    @Published var fps = 0
    @Published var showFPS = false
    @Published var paused = false
    @Published var internalSize = (0, 0)
    private let started = Date()

    init() {
        let dir = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].path
        dir.withCString { ve3d_init(UnsafeMutablePointer(mutating: $0)) }
        audio.start()
    }

    static func text(_ which: Int32) -> String {
        var buf = [CChar](repeating: 0, count: 512)
        _ = ve3d_text(which, &buf, Int32(buf.count))
        return String(cString: buf)
    }

    func refreshHUD(internal size: (Int, Int)) {
        let t = AppCore.text(0), i = AppCore.text(1)
        var toastText = AppCore.text(2)
        if toastText.isEmpty && Date().timeIntervalSince(started) < 12 {
            toastText = "Drag to look · tap for a pulse · double-tap for a new scene · hold for new colours · two fingers: slide & tape speed · pinch: altitude"
        }
        let age = ve3d_scene_age()
        let op = age < 6 ? 1.0 : max(0, 1 - (age - 6) / 2)
        if t != title { title = t }
        if i != info { info = i }
        if toastText != toast { toast = toastText }
        if abs(op - titleOpacity) > 0.01 { titleOpacity = op }
        let sf = ve3d_show_fps() != 0
        if sf != showFPS { showFPS = sf }
        if size != internalSize { internalSize = size }
    }

    func setActive(_ active: Bool) {
        paused = !active
        if active { audio.start() } else {
            ve3d_save()
            audio.stop()
        }
    }
}

@main
struct VaporwaveApp: App {
    @StateObject private var core = AppCore()
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            ContentView(core: core)
                .onChange(of: phase) { p in core.setActive(p == .active) }
        }
    }
}

struct ContentView: View {
    @ObservedObject var core: AppCore
    @State private var showSettings = false

    var body: some View {
        ZStack {
            MetalView(core: core).ignoresSafeArea()
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .top) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(core.title)
                            .font(.system(size: 24, weight: .heavy, design: .monospaced))
                            .opacity(core.titleOpacity)
                        Text(core.info).font(.system(size: 11, design: .monospaced)).opacity(0.85)
                    }
                    Spacer()
                    if core.showFPS {
                        Text("\(core.fps) FPS · METAL GPU · \(core.internalSize.0)x\(core.internalSize.1)")
                            .font(.system(size: 11, design: .monospaced))
                    }
                }
                Spacer()
                HStack(alignment: .bottom) {
                    Text(core.toast)
                        .font(.system(size: 12, design: .monospaced))
                        .frame(maxWidth: 520, alignment: .leading)
                    Spacer()
                    button("shuffle") { ve3d_new_scene() }
                    button("sparkles") { ve3d_toggle_space() }
                    button("slider.horizontal.3") { showSettings = true }
                }
            }
            .foregroundColor(.white)
            .shadow(color: .black.opacity(0.8), radius: 2, x: 1, y: 1)
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
        }
        .background(Color.black)
        .statusBarHidden(true)
        .persistentSystemOverlays(.hidden)
        .sheet(isPresented: $showSettings, onDismiss: { ve3d_save() }) {
            SettingsView()
        }
    }

    private func button(_ icon: String, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: 20, weight: .semibold))
                .frame(width: 46, height: 46)
                .background(.ultraThinMaterial, in: Circle())
        }
        .buttonStyle(.plain)
    }
}

/// The settings menu; its rows come from the Go core (Mobile.buildMenu).
struct SettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var rows: [Row] = []

    struct Row: Identifiable { let id: Int; let label: String; let value: String; let header: Bool }
    struct Section_: Identifiable { let id: Int; let title: String; let rows: [Row] }

    private var sections: [Section_] {
        var out: [Section_] = []
        for r in rows {
            if r.header { out.append(Section_(id: r.id, title: r.label, rows: [])) }
            else if let last = out.popLast() { out.append(Section_(id: last.id, title: last.title, rows: last.rows + [r])) }
        }
        return out
    }

    private func reload() {
        var l = [CChar](repeating: 0, count: 256), v = [CChar](repeating: 0, count: 256)
        rows = (0..<Int(ve3d_menu_count())).map { i in
            let h = ve3d_menu_item(Int32(i), &l, Int32(l.count), &v, Int32(v.count)) != 0
            return Row(id: i, label: String(cString: l), value: String(cString: v), header: h)
        }
    }

    private func adjust(_ i: Int, _ d: Int) {
        ve3d_menu_adjust(Int32(i), Int32(d))
        reload()
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(sections) { s in
                    Section(s.title) {
                        ForEach(s.rows) { r in
                            HStack {
                                Text(r.label)
                                Spacer()
                                if r.value == "Go" || r.value == "Reset" {
                                    Button(r.value) { adjust(r.id, 1) }
                                } else {
                                    Button { adjust(r.id, -1) } label: { Image(systemName: "minus.circle.fill") }
                                    Text(r.value).monospacedDigit().frame(minWidth: 120)
                                    Button { adjust(r.id, 1) } label: { Image(systemName: "plus.circle.fill") }
                                }
                            }
                            .buttonStyle(.borderless)
                            .font(.system(size: 15))
                        }
                    }
                }
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } } }
        }
        .onAppear(perform: reload)
    }
}
