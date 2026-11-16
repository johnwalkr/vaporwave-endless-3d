import AVFoundation

/// Plays the Go synth through AVAudioEngine.
final class AudioOutput {
    private let engine = AVAudioEngine()
    private var node: AVAudioSourceNode?
    /// Seconds between rendering a sample and hearing it; the visuals use it to stay on the beat.
    private(set) var latency: Double = 0.05

    func start() {
        let session = AVAudioSession.sharedInstance()
        try? session.setCategory(.playback, mode: .default, options: [])
        try? session.setPreferredIOBufferDuration(0.01)
        try? session.setActive(true)
        if node == nil {
            let fmt = AVAudioFormat(standardFormatWithSampleRate: 44100, channels: 2)!
            let n = AVAudioSourceNode(format: fmt) { _, _, frames, abl -> OSStatus in
                let list = UnsafeMutableAudioBufferListPointer(abl)
                guard list.count >= 2,
                      let l = list[0].mData?.assumingMemoryBound(to: Float.self),
                      let r = list[1].mData?.assumingMemoryBound(to: Float.self) else { return noErr }
                ve3d_audio(l, r, Int32(frames))
                return noErr
            }
            engine.attach(n)
            engine.connect(n, to: engine.mainMixerNode, format: fmt)
            node = n
            NotificationCenter.default.addObserver(forName: AVAudioSession.interruptionNotification, object: nil,
                                                   queue: .main) { [weak self] note in
                let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt
                if raw == AVAudioSession.InterruptionType.ended.rawValue { self?.start() }
            }
            NotificationCenter.default.addObserver(forName: AVAudioSession.routeChangeNotification, object: nil,
                                                   queue: .main) { [weak self] _ in self?.updateLatency() }
        }
        if !engine.isRunning { try? engine.start() }
        updateLatency()
    }

    func stop() { engine.pause() }

    private func updateLatency() {
        let s = AVAudioSession.sharedInstance()
        latency = s.outputLatency + s.ioBufferDuration
    }
}
