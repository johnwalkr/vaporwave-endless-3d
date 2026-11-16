import MetalKit
import QuartzCore

/// Draws the frames the Go core prepares, with the same passes as the
/// desktop OpenGL renderer: sky/floor, geometry, bloom, composite, blit.
final class Renderer: NSObject, MTKViewDelegate {
    let device: MTLDevice
    private let queue: MTLCommandQueue
    private let bgPipe, geoPipe, downPipe, blurPipe, compPipe, blitPipe: MTLRenderPipelineState
    private let depthAlways, depthTest, depthWrite: MTLDepthStencilState
    private var scene, depth, bloom0, bloom1, comp: MTLTexture?
    private var texW = 0, texH = 0

    private var params = [Float](repeating: 0, count: Int(ve3d_param_count()))
    private var ui = [UInt32](repeating: 0, count: 4)
    private var runs = [Int32](repeating: 0, count: 3 * 16384)
    private var geoBuffers: [MTLBuffer?] = [nil, nil, nil]
    private var bufIndex = 0
    private let inFlight = DispatchSemaphore(value: 3)

    private var last: CFTimeInterval = 0
    private var fpsFrames = 0, fpsStart: CFTimeInterval = 0
    private var hudUpdate: CFTimeInterval = 0
    weak var core: AppCore?

    private struct CompP { var p0, p1, p2: SIMD4<Float>; var f: SIMD4<UInt32> }

    init?(view: MTKView) {
        guard let dev = MTLCreateSystemDefaultDevice(), let q = dev.makeCommandQueue(),
              let lib = dev.makeDefaultLibrary() else { return nil }
        device = dev
        queue = q
        view.device = dev
        view.colorPixelFormat = .bgra8Unorm
        view.depthStencilPixelFormat = .invalid
        view.framebufferOnly = true
        view.preferredFramesPerSecond = 60

        func pipe(_ vs: String, _ fs: String, _ color: MTLPixelFormat, depth: Bool = false,
                  blend: Bool = false, vertexDesc: MTLVertexDescriptor? = nil) -> MTLRenderPipelineState? {
            let d = MTLRenderPipelineDescriptor()
            d.vertexFunction = lib.makeFunction(name: vs)
            d.fragmentFunction = lib.makeFunction(name: fs)
            d.colorAttachments[0].pixelFormat = color
            if depth { d.depthAttachmentPixelFormat = .depth32Float }
            if blend {
                let c = d.colorAttachments[0]!
                c.isBlendingEnabled = true
                c.sourceRGBBlendFactor = .one
                c.sourceAlphaBlendFactor = .one
                c.destinationRGBBlendFactor = .oneMinusSourceAlpha
                c.destinationAlphaBlendFactor = .oneMinusSourceAlpha
            }
            d.vertexDescriptor = vertexDesc
            do { return try dev.makeRenderPipelineState(descriptor: d) } catch {
                print("pipeline \(fs): \(error)")
                return nil
            }
        }
        // 9 floats per vertex: x y invz | r g b a | edge distance, half width
        let vd = MTLVertexDescriptor()
        vd.attributes[0].format = .float3; vd.attributes[0].offset = 0; vd.attributes[0].bufferIndex = 0
        vd.attributes[1].format = .float4; vd.attributes[1].offset = 12; vd.attributes[1].bufferIndex = 0
        vd.attributes[2].format = .float2; vd.attributes[2].offset = 28; vd.attributes[2].bufferIndex = 0
        vd.layouts[0].stride = 36

        guard let bg = pipe("fsVS", "bgFS", .rgba16Float, depth: true),
              let geo = pipe("geoVS", "geoFS", .rgba16Float, depth: true, blend: true, vertexDesc: vd),
              let down = pipe("fsVS", "downFS", .rgba16Float),
              let blur = pipe("fsVS", "blurFS", .rgba16Float),
              let cmp = pipe("fsVS", "compFS", .rgba8Unorm),
              let blit = pipe("fsVS", "blitFS", view.colorPixelFormat) else { return nil }
        bgPipe = bg; geoPipe = geo; downPipe = down; blurPipe = blur; compPipe = cmp; blitPipe = blit

        func ds(_ f: MTLCompareFunction, _ write: Bool) -> MTLDepthStencilState {
            let d = MTLDepthStencilDescriptor()
            d.depthCompareFunction = f
            d.isDepthWriteEnabled = write
            return dev.makeDepthStencilState(descriptor: d)!
        }
        depthAlways = ds(.always, false)
        depthTest = ds(.lessEqual, false)
        depthWrite = ds(.lessEqual, true)
        super.init()
    }

    private func makeTex(_ w: Int, _ h: Int, _ f: MTLPixelFormat) -> MTLTexture? {
        let d = MTLTextureDescriptor.texture2DDescriptor(pixelFormat: f, width: w, height: h, mipmapped: false)
        d.usage = f == .depth32Float ? [.renderTarget] : [.renderTarget, .shaderRead]
        d.storageMode = .private
        return device.makeTexture(descriptor: d)
    }

    private func ensureTextures(_ w: Int, _ h: Int) {
        if w == texW && h == texH && scene != nil { return }
        texW = w; texH = h
        let bw = (w + 3) / 4, bh = (h + 3) / 4
        scene = makeTex(w, h, .rgba16Float)
        depth = makeTex(w, h, .depth32Float)
        bloom0 = makeTex(bw, bh, .rgba16Float)
        bloom1 = makeTex(bw, bh, .rgba16Float)
        comp = makeTex(w, h, .rgba8Unorm)
    }

    func mtkView(_ view: MTKView, drawableSizeWillChange size: CGSize) {
        ve3d_resize(Int32(size.width), Int32(size.height))
    }

    private func pass(_ cb: MTLCommandBuffer, _ tex: MTLTexture, depth: MTLTexture? = nil) -> MTLRenderCommandEncoder? {
        let rp = MTLRenderPassDescriptor()
        rp.colorAttachments[0].texture = tex
        rp.colorAttachments[0].loadAction = .dontCare
        rp.colorAttachments[0].storeAction = .store
        if let depth {
            rp.depthAttachment.texture = depth
            rp.depthAttachment.loadAction = .clear
            rp.depthAttachment.clearDepth = 1
            rp.depthAttachment.storeAction = .dontCare
        }
        return cb.makeRenderCommandEncoder(descriptor: rp)
    }

    func draw(in view: MTKView) {
        let now = CACurrentMediaTime()
        let dt = last == 0 ? 1.0 / 60 : now - last
        last = now
        ve3d_frame(dt, core?.audio.latency ?? 0.05)

        var iw: Int32 = 0, ih: Int32 = 0
        ve3d_internal_size(&iw, &ih)
        let w = Int(iw), h = Int(ih)
        ensureTextures(w, h)
        params.withUnsafeMutableBufferPointer { p in
            ui.withUnsafeMutableBufferPointer { u in _ = ve3d_params(p.baseAddress, Int32(p.count), u.baseAddress) }
        }
        var cpf = [Float](repeating: 0, count: 12), cpu = [UInt32](repeating: 0, count: 4)
        ve3d_comp(&cpf, &cpu)
        var compP = CompP(p0: SIMD4(cpf[0], cpf[1], cpf[2], cpf[3]), p1: SIMD4(cpf[4], cpf[5], cpf[6], cpf[7]),
                          p2: SIMD4(cpf[8], cpf[9], cpf[10], cpf[11]), f: SIMD4(cpu[0], cpu[1], cpu[2], cpu[3]))

        inFlight.wait()
        bufIndex = (bufIndex + 1) % geoBuffers.count
        let gf = Int(ve3d_geometry_floats())
        var geo = geoBuffers[bufIndex]
        if gf > 0 {
            if geo == nil || geo!.length < gf * 4 {
                geo = device.makeBuffer(length: max(gf * 4 * 3 / 2, 1 << 18), options: .storageModeShared)
                geoBuffers[bufIndex] = geo
            }
            if let geo { _ = ve3d_geometry(geo.contents().assumingMemoryBound(to: Float.self), Int32(gf)) }
        }
        let nr = runs.withUnsafeMutableBufferPointer { Int(ve3d_runs($0.baseAddress, Int32($0.count))) }

        guard let scene, let depth, let bloom0, let bloom1, let comp,
              let drawable = view.currentDrawable, let cb = queue.makeCommandBuffer() else {
            inFlight.signal()
            return
        }
        cb.addCompletedHandler { [inFlight] _ in inFlight.signal() }

        // 1 + 2: sky/floor, then the geometry over it
        if let e = pass(cb, scene, depth: depth) {
            e.setRenderPipelineState(bgPipe)
            e.setDepthStencilState(depthAlways)
            e.setFragmentBytes(params, length: params.count * 4, index: 0)
            e.setFragmentBytes(ui, length: 16, index: 1)
            e.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
            if gf > 0, let geo, nr > 0 {
                e.setRenderPipelineState(geoPipe)
                e.setVertexBuffer(geo, offset: 0, index: 0)
                var vp = SIMD2<Float>(Float(w), Float(h))
                e.setVertexBytes(&vp, length: 8, index: 1)
                let nv = gf / 9
                for i in 0..<nr {
                    let first = Int(runs[3 * i]), count = Int(runs[3 * i + 1])
                    if count <= 0 || first + count > nv { continue }
                    e.setDepthStencilState(runs[3 * i + 2] != 0 ? depthWrite : depthTest)
                    e.drawPrimitives(type: .triangle, vertexStart: first, vertexCount: count)
                }
            }
            e.endEncoding()
        }
        // 3: bloom
        if let e = pass(cb, bloom0) {
            e.setRenderPipelineState(downPipe)
            e.setFragmentTexture(scene, index: 0)
            e.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
            e.endEncoding()
        }
        for (src, dst, dir, rad) in [(bloom0, bloom1, 0, 2), (bloom1, bloom0, 1, 2), (bloom0, bloom1, 0, 4), (bloom1, bloom0, 1, 4)] {
            guard let e = pass(cb, dst) else { continue }
            e.setRenderPipelineState(blurPipe)
            e.setFragmentTexture(src, index: 0)
            var dr = SIMD2<Int32>(Int32(dir), Int32(rad))
            e.setFragmentBytes(&dr, length: 8, index: 0)
            e.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
            e.endEncoding()
        }
        // 4: composite
        if let e = pass(cb, comp) {
            e.setRenderPipelineState(compPipe)
            e.setFragmentTexture(scene, index: 0)
            e.setFragmentTexture(bloom0, index: 1)
            e.setFragmentBytes(&compP, length: MemoryLayout<CompP>.stride, index: 0)
            e.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
            e.endEncoding()
        }
        // 5: scale to the screen
        if let e = pass(cb, drawable.texture) {
            e.setRenderPipelineState(blitPipe)
            e.setFragmentTexture(comp, index: 0)
            var p = SIMD4<Float>(Float(drawable.texture.width), Float(drawable.texture.height), 0, 0)
            e.setFragmentBytes(&p, length: 16, index: 0)
            e.drawPrimitives(type: .triangle, vertexStart: 0, vertexCount: 3)
            e.endEncoding()
        }
        cb.present(drawable)
        cb.commit()

        fpsFrames += 1
        if fpsStart == 0 { fpsStart = now }
        if now - fpsStart >= 0.5 {
            core?.fps = Int((Double(fpsFrames) / (now - fpsStart)).rounded())
            fpsFrames = 0
            fpsStart = now
        }
        if now - hudUpdate >= 0.1 {
            hudUpdate = now
            core?.refreshHUD(internal: (w, h))
        }
    }
}
