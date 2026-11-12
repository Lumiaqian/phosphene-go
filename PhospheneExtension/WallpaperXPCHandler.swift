import AppKit
import AVFoundation
import CoreMedia
import os
import QuartzCore

/// Carries a non-Sendable value (e.g. a `CALayer`) into a `Task` without tainting the
/// closure's isolation region. `nonisolated(unsafe)` on a local isn't enough under Swift 6.2
/// region-based isolation — capturing the raw layer merges other (Sendable) captures like
/// `videoURL` into a non-Sendable region, which then trips the `sending` checker across the
/// sibling BMP-snapshot Task. Boxing makes the capture genuinely Sendable.
struct SendableBox<T>: @unchecked Sendable { let value: T }

/// Process-wide serialization for wallpaper lifecycle XPC. Every connection gets its
/// own `WallpaperXPCHandler`, but the Agent multiplexes desktop + Settings-preview +
/// thumbnail connections, so lifecycle callbacks (acquire/update/invalidate/choice
/// change) can otherwise interleave across connections. We funnel them all through ONE
/// serial queue — mirroring Apple's single `Controller`-actor `AsyncQueue` — so an
/// invalidate can't slip between the halves of an acquire.
enum Lifecycle {
    static let queue = DispatchQueue(label: "local.lumiaqian.phosphenego.lifecycle")

    /// Pending per-surface teardown timers. Touched only on `queue`.
    nonisolated(unsafe) static var teardownTimers: [SurfaceKey: DispatchWorkItem] = [:]

    /// Grace between an invalidate of a display's LIVE wallpaper and actually tearing it
    /// down. A re-acquire (display woke / switched) cancels it; only a display that stays
    /// gone (asleep/removed) lets it fire. Short enough to save power promptly, long enough
    /// to ride out a brief sleep/wake flicker.
    static let teardownGrace: TimeInterval = 15.0
}

/// Arm (or re-arm) the teardown timer for a surface that was invalidated.
/// Must be called on `Lifecycle.queue`.
private func scheduleTeardown(for key: SurfaceKey) {
    Lifecycle.teardownTimers[key]?.cancel()
    let item = DispatchWorkItem {
        Lifecycle.teardownTimers[key] = nil
        let torn = SurfaceRegistry.shared.tearDown(key)
        PlaybackStore.post(.surfaceRemoved(key))
        extensionLog("  [teardown] grace fired for \(key) → \(torn ? "stopped renderer + invalidated CAContext" : "nothing to tear down")")
    }
    Lifecycle.teardownTimers[key] = item
    Lifecycle.queue.asyncAfter(deadline: .now() + Lifecycle.teardownGrace, execute: item)
}

/// Cancel a surface's pending teardown because it was re-acquired.
/// Must be called on `Lifecycle.queue`.
private func cancelTeardown(for key: SurfaceKey) {
    if let item = Lifecycle.teardownTimers.removeValue(forKey: key) {
        item.cancel()
        extensionLog("  [teardown] cancelled pending teardown for \(key) (re-acquired)")
    }
}

/// Only the system presentation destination crosses this private API boundary.
private struct AcquisitionDestination {
    var size = CGSize(width: 2_560, height: 1_440)
    var scale: CGFloat = 2
    var displayID: UInt32?
    var isPreview = false
    var cacheDirectory: URL?

    init(_ request: Any?) {
        guard let request = request as? NSObject else { return }
        for child in Mirror(reflecting: request).children {
            for property in Mirror(reflecting: child.value).children {
                switch property.label {
                case "destination":
                    for value in Mirror(reflecting: property.value).children {
                        switch value.label {
                        case "size": if let value = value.value as? CGSize {
                                size = value
                            }
                        case "scaleFactor": if let value = value.value as? CGFloat {
                                scale = value
                            }
                        case "directDisplayID": displayID = value.value as? UInt32
                        default: break
                        }
                    }
                case "isPreview": isPreview = property.value as? Bool ?? false
                case "cacheDirectory": cacheDirectory = property.value as? URL
                default: break
                }
            }
        }
    }
}

final class WallpaperXPCHandler: NSObject, WallpaperExtensionXPCProtocol {
    /// Proxy to call methods on WallpaperAgent (ping, invalidateSnapshots, etc.).
    /// Lock-backed: it's assigned in `accept(connection:)` and nilled from the
    /// invalidation queue, while XPC callbacks read it from the message queue.
    /// The proxy existential isn't Sendable, so this uses a plain `NSLock` around
    /// `nonisolated(unsafe)` storage rather than `OSAllocatedUnfairLock` (whose
    /// `withLock` body is `@Sendable` and would reject the non-Sendable value).
    private let agentProxyLock = NSLock()
    private nonisolated(unsafe) var _agentProxy: (any WallpaperExtensionProxyXPCProtocol)?
    var agentProxy: (any WallpaperExtensionProxyXPCProtocol)? {
        get { agentProxyLock.lock(); defer { agentProxyLock.unlock() }; return _agentProxy }
        set { agentProxyLock.lock(); defer { agentProxyLock.unlock() }; _agentProxy = newValue }
    }

    /// PID of the peer on this connection (WallpaperAgent vs. Settings preview vs.
    /// the thumbnail service), set in `accept(connection:)`. Logged so acquire and
    /// invalidation can be attributed to a specific connection.
    var connectionPID: Int32 = -1

    // Whether this connection's most recent acquire was a Settings *preview*
    // (`isPreview: true`). A preview connection reports its own presentation state
    // (which idles/toggles as the picker is interacted with), and since every
    // connection shares the one desktop renderer, letting a preview's `update()`
    // apply pause policy would freeze the visible desktop wallpaper. Preview
    // connections therefore don't drive playback; only the live desktop connection
    // (`isPreview: false`) does.

    /// Whether any exported method has been invoked on this connection. Read by the
    /// invalidationHandler: a connection that is accepted and invalidated without ever
    /// serving a method is "empty" — the WallpaperAgent spiral-of-death signal
    /// (see SpiralRecovery). Set synchronously at each method's entry, before any queue hop.
    private let servedMethod = OSAllocatedUnfairLock(initialState: false)
    var didServeMethod: Bool {
        servedMethod.withLock { $0 }
    }

    /// Mark this connection as healthy (a real method arrived) and clear any spiral run.
    /// Call first thing in every exported entry point.
    private func markServed() {
        servedMethod.withLock { $0 = true }
        SpiralRecovery.noteHealthyConnection()
    }

    // MARK: - Lifecycle

    /// Stable per-display surface UUID for the rare acquire that carries no WallpaperID — so
    /// such acquires still collapse to one context per display (old behavior) instead of
    /// minting a fresh context each time. Encodes the displayID into a fixed UUID layout.
    static func fallbackSurfaceUUID(forDisplay displayID: UInt32) -> UUID {
        UUID(uuidString: String(format: "00000000-0000-0000-0000-%012x", displayID))
            ?? UUID(uuid: (0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0))
    }

    func acquire(withId id: Any?, request: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        markServed()
        nonisolated(unsafe) let unsafeRequest = request
        nonisolated(unsafe) let unsafeID = id
        nonisolated(unsafe) let handler = self
        Lifecycle.queue.async { handler.acquireBody(id: unsafeID, request: unsafeRequest, reply: reply) }
    }

    private func acquireBody(id: Any?, request: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        traceLog("=== ACQUIRE ===")

        let destination = AcquisitionDestination(request)
        let destSize = destination.size
        let scaleFactor = destination.scale
        let displayID = destination.displayID
        let isPreview = destination.isPreview
        let choiceConfiguration = playlistChoiceID
        if let cache = destination.cacheDirectory {
            SurfaceRegistry.shared.cacheDirectoryURL = cache
        }

        let selection = PlaylistController.shared.selection(displayID: displayID ?? 0, preview: isPreview)
        let renderChoice = selection?.videoID
        let position = selection?.positionSeconds ?? 0

        // Each WallpaperID (a Space, the lock-screen surface, or a Settings preview) is its
        // own hosted surface with its own CAContext; see `SurfaceKey`. An id without a UUID
        // falls back to a per-display constant.
        let displayID0 = displayID ?? 0
        let surfaceUUID = extractWallpaperUUID(fromID: id) ?? Self.fallbackSurfaceUUID(forDisplay: displayID0)
        let key = SurfaceKey(displayID: displayID0, surfaceUUID: surfaceUUID)
        let registry = SurfaceRegistry.shared
        registry.register(wallpaperID: surfaceUUID, as: key)
        PlaybackStore.post(.surfaceAcquired(key, displayID: displayID, role: isPreview ? .preview : .desktop, choice: choiceConfiguration))
        // A re-acquire of this surface (display woke / preview refresh / switch) cancels
        // its pending teardown, so a brief invalidate→re-acquire flicker doesn't drop it.
        cancelTeardown(for: key)
        let videoURL = renderChoice.flatMap { VideoLibrary.shared.videoURL(for: $0) }
        let cachedStill = loadCachedSnapshotImage(forChoice: renderChoice)

        // ---- REUSE: this surface's context already exists ----
        if let existing = registry.surface(for: key) {
            reuseSurface(existing, key: key, destination: destination, selection: selection, reply: reply)
            return
        }

        // ---- CREATE: first acquire for this surface ----
        var contextOptions: [String: Any] = [:]
        if let did = displayID {
            contextOptions["displayId"] = did
        }
        let caContextRaw: Any? = contextOptions.isEmpty
            ? CAContext.remoteContext()
            : CAContext.perform(NSSelectorFromString("remoteContextWithOptions:"), with: contextOptions)?.takeUnretainedValue()
        guard let caContext = caContextRaw as? CAContext, caContext.contextId != 0 else {
            extensionLog("  ERROR: remote CAContext creation failed — failing acquire")
            reply(nil, NSError(domain: "PhospheneExtension", code: 4, userInfo: [NSLocalizedDescriptionKey: "Failed to create remote CAContext"]))
            return
        }
        let contextId = caContext.contextId

        let layerFrame = CGRect(origin: .zero, size: destSize)
        let rootLayer = CALayer()
        rootLayer.frame = layerFrame
        rootLayer.contentsScale = scaleFactor
        rootLayer.contentsGravity = .resizeAspectFill
        if let cachedStill {
            rootLayer.contents = cachedStill
        }
        caContext.layer = rootLayer
        CATransaction.flush()

        guard let replyObj = createRemoteContextXPC(contextId: contextId) else {
            reply(nil, NSError(domain: "PhospheneExtension", code: 3, userInfo: nil)); return
        }

        // Install the surface now (renderer added async) so a concurrent acquire for the
        // same surface reuses this context instead of creating another.
        registry.install(
            HostedSurface(caContext: caContext, contextId: contextId, rootLayer: rootLayer, renderer: nil, displayID: displayID, videoID: choiceConfiguration, isPreview: isPreview, destSize: destSize, scaleFactor: scaleFactor),
            for: key,
        )
        extensionLog("  Created context \(contextId) for \(key) at \(destSize) @\(scaleFactor)x")
        reconcileSurfaceGeometry(for: key, because: "acquire")

        // The XPC reply is deferred until the new context displays video. WallpaperAgent
        // hosts a context only after it receives the reply and keeps compositing the old
        // wallpaper's context until then; replying early made it swap to a context that
        // wasn't rendering yet (a blink / still-flash / zoom on every switch). Every
        // branch below replies exactly once, so the acquire can never hang.

        guard let videoURL else {
            // No video file: solid gradient fallback. Static, so it's ready at once.
            let gradientLayer = CAGradientLayer()
            gradientLayer.colors = [
                CGColor(red: 0.2, green: 0.0, blue: 0.5, alpha: 1.0),
                CGColor(red: 0.0, green: 0.3, blue: 0.7, alpha: 1.0),
                CGColor(red: 0.0, green: 0.6, blue: 0.4, alpha: 1.0),
            ]
            gradientLayer.startPoint = CGPoint(x: 0, y: 0)
            gradientLayer.endPoint = CGPoint(x: 1, y: 1)
            gradientLayer.frame = layerFrame
            gradientLayer.contentsScale = scaleFactor
            CATransaction.begin(); CATransaction.setDisableActions(true)
            rootLayer.addSublayer(gradientLayer)
            CATransaction.commit(); CATransaction.flush()
            reply(replyObj, nil)
            extensionLog("  No video file found — solid color fallback")
            return
        }

        // Cold start = no Phosphene surface in the same role (preview vs. live desktop) on
        // this display that the agent could keep compositing during the swap. On a switch
        // the outgoing context is ours and stays hosted (teardown grace) until we reply.
        // On a cold start the agent shows whatever our context holds the instant it hosts
        // it, and `rootLayer.contents` is black cross-process (only IOSurface-backed
        // AVSampleBufferDisplayLayer content composites remotely — see
        // Research/wallpaper-extension-issue13-and-rendering-findings.md). So a cold start
        // replies as soon as `VideoRenderer.create()` has seeded the IOSurface still, and a
        // switch waits for the first video frame.
        //
        // Filtering by role fixes the WallpaperAgent-restart ordering: a preview-first,
        // desktop-second boot must not make the desktop acquire look like a switch, since
        // the desktop CALayerHost has never hosted anything of ours.
        let coldStart = !registry.hasLiveRenderer(onDisplay: displayID0, isPreview: isPreview)

        // Claim this surface's single create slot; a racing acquire that lost skips.
        if registry.claimRendererCreate(for: key) {
            traceLog("  Setting up VideoRenderer with: \(videoURL.lastPathComponent) (coldStart=\(coldStart))")
            let boxedRoot = SendableBox(value: rootLayer)
            let boxedReply = SendableBox(value: replyObj)
            Task(name: "Create renderer \(key)") { [coldStart, boxedRoot, boxedReply, videoURL, cachedStill, position, key, choiceConfiguration] in
                let renderer: VideoRenderer
                do {
                    renderer = try await VideoRenderer.create(rootLayer: boxedRoot.value, videoURL: videoURL, stillImage: cachedStill, position: position)
                } catch {
                    extensionLog("  [Renderer] Failed to create: \(error)")
                    SurfaceRegistry.shared.clearRendererPending(for: key)
                    PlaylistController.shared.mediaFailed(renderChoice, message: error.localizedDescription)
                    reply(boxedReply.value, nil) // unblock the acquire regardless
                    return
                }
                // Cold start: the IOSurface still is seeded and flushed, so the agent can
                // host our context without a black gap while the video comes up over it.
                if coldStart && cachedStill != nil {
                    reply(boxedReply.value, nil)
                    traceLog("  [acquire] cold start → replied after still seeded for \(videoURL.lastPathComponent)")
                }
                guard SurfaceRegistry.shared.surface(for: key) != nil else {
                    renderer.stop(); if !coldStart || cachedStill == nil {
                        reply(boxedReply.value, nil)
                    }; return
                }
                let old = SurfaceRegistry.shared.setRenderer(renderer, videoID: choiceConfiguration, for: key)
                await PlaybackStore.shared.follow(renderer, surface: key)
                await renderer.start()
                // Switch: reply once the first frame is composited. Either way, stop the
                // old renderer once the agent has been told to swap off it.
                if !coldStart || cachedStill == nil {
                    reply(boxedReply.value, nil)
                }
                old?.stop()
            }
        } else {
            traceLog("  [acquire] renderer create already in flight for \(key) — skipping duplicate (create path)")
            reply(replyObj, nil)
        }
        let w = Int(destSize.width * scaleFactor), h = Int(destSize.height * scaleFactor)
        Task { await writeBMPSnapshot(videoURL: videoURL, videoID: renderChoice, displayPixelWidth: w, displayPixelHeight: h) }
    }

    private func reuseSurface(
        _ existing: HostedSurface,
        key: SurfaceKey,
        destination: AcquisitionDestination,
        selection: DisplayProgress?,
        reply: @escaping @Sendable (Any?, (any Error)?) -> Void,
    ) {
        let registry = SurfaceRegistry.shared
        let destSize = destination.size
        let scaleFactor = destination.scale
        let choiceConfiguration = playlistChoiceID
        let renderChoice = selection?.videoID
        let position = selection?.positionSeconds ?? 0
        let videoURL = renderChoice.flatMap { VideoLibrary.shared.videoURL(for: $0) }
        let cachedStill = loadCachedSnapshotImage(forChoice: renderChoice)
        traceLog("  [acquire] REUSE ctx=\(existing.contextId) \(key) storedVideoID=\(existing.videoID ?? "nil") newChoice=\(choiceConfiguration) renderer=\(existing.renderer.map { "#\($0.debugID)" } ?? "nil")")

        // Geometry may have changed since this surface was created: a bigger or
        // smaller display reconnected, or the display switched resolution. Re-frame
        // the root and renderer layers before re-hosting (issue #21).
        if let resized = registry.updateGeometryIfChanged(destSize: destSize, scaleFactor: scaleFactor, for: key) {
            layOutSurface(resized, destSize: destSize, scaleFactor: scaleFactor)
            extensionLog("  [acquire] REUSE geometry changed → resized \(key) to \(destSize) @\(scaleFactor)x")
        }
        reconcileSurfaceGeometry(for: key, because: "re-acquire")

        guard let replyObj = createRemoteContextXPC(contextId: existing.contextId) else {
            reply(nil, NSError(domain: "PhospheneExtension", code: 3, userInfo: nil)); return
        }
        reply(replyObj, nil)

        if existing.videoID == choiceConfiguration, existing.renderer != nil {
            traceLog("  [acquire] same choice (\(choiceConfiguration)) and renderer present → no swap")
            return
        }
        guard let videoURL else {
            traceLog("  [acquire] no video for new choice, keeping current")
            return
        }
        extensionLog("  [acquire] switching to \(videoURL.lastPathComponent) (renderer \(existing.renderer != nil ? "present → switchVideo" : "nil → create"))")
        if let renderer = existing.renderer {
            // Switch in place on the already-hosted display layer: a new
            // AVSampleBufferDisplayLayer added to a hosted context doesn't composite.
            renderer.play(videoURL, at: position)
            registry.updateVideoID(choiceConfiguration, for: key)
        } else if registry.claimRendererCreate(for: key) {
            let boxedRoot = SendableBox(value: existing.rootLayer)
            Task(name: "Attach renderer \(key)") { [boxedRoot, videoURL, cachedStill, position, key, choiceConfiguration] in
                let renderer: VideoRenderer
                do {
                    renderer = try await VideoRenderer.create(rootLayer: boxedRoot.value, videoURL: videoURL, stillImage: cachedStill, position: position)
                } catch {
                    extensionLog("  [Renderer] swap create failed: \(error)")
                    SurfaceRegistry.shared.clearRendererPending(for: key)
                    PlaylistController.shared.mediaFailed(renderChoice, message: error.localizedDescription)
                    return
                }
                guard SurfaceRegistry.shared.surface(for: key) != nil else { renderer.stop(); return }
                let old = SurfaceRegistry.shared.setRenderer(renderer, videoID: choiceConfiguration, for: key)
                old?.stop()
                await PlaybackStore.shared.follow(renderer, surface: key)
                await renderer.start()
            }
        } else {
            traceLog("  [acquire] renderer create already in flight for \(key) — skipping duplicate")
        }
        let w = Int(destSize.width * scaleFactor), h = Int(destSize.height * scaleFactor)
        Task { [videoURL, renderChoice, w, h] in await writeBMPSnapshot(videoURL: videoURL, videoID: renderChoice, displayPixelWidth: w, displayPixelHeight: h) }
    }

    func update(withId id: Any?, request: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        markServed()
        nonisolated(unsafe) let unsafeRequest = request
        nonisolated(unsafe) let unsafeID = id
        nonisolated(unsafe) let handler = self
        Lifecycle.queue.async { handler.updateBody(id: unsafeID, request: unsafeRequest, reply: reply) }
    }

    private func updateBody(id: Any?, request: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        // Read the enum cases through Mirror rather than a stringified description,
        // whose format changes silently. A field that can't be found reads as the
        // benign desktop-active value.
        var mode = PresentationMode.default
        var activity = ActivityState.active
        if let request {
            if let value = mirrorFindProperty("presentationMode", in: request) {
                mode = PresentationMode(caseName: enumCaseName(value))
            }
            if let value = mirrorFindProperty("activityState", in: request) {
                activity = ActivityState(caseName: enumCaseName(value))
            }
        }

        let key = extractWallpaperUUID(fromID: id).flatMap { SurfaceRegistry.shared.key(forWallpaperID: $0) }
        PlaybackStore.post(.agentUpdate(key, mode: mode, activity: activity))
        extensionLog("=== UPDATE (pid \(connectionPID)) === \(key.map(\.description) ?? "unknown surface → all") mode: \(mode), activity: \(activity)")
        reply(nil)
    }

    func invalidate(withId id: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        markServed()
        nonisolated(unsafe) let unsafeID = id
        nonisolated(unsafe) let handler = self
        Lifecycle.queue.async { handler.invalidateBody(id: unsafeID, reply: reply) }
    }

    private func invalidateBody(id: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        // Per-surface teardown after a short grace, resolved through the WallpaperID UUID
        // learned at acquire. A re-acquire of the same UUID (display sleep/wake, a quick
        // Space revisit) cancels it. Scoped to one surface, it can never black out another
        // Space, and a superseded UUID cleans up its own context instead of leaking it.
        let registry = SurfaceRegistry.shared
        guard let uuid = extractWallpaperUUID(fromID: id) else {
            extensionLog("=== INVALIDATE === no UUID in id → ignore (kept \(registry.count) surface(s))")
            reply(nil); return
        }
        guard let key = registry.key(forWallpaperID: uuid) else {
            extensionLog("=== INVALIDATE === UUID \(uuid) unknown (not ours / already forgotten) → ignore")
            reply(nil); return
        }
        PlaylistController.shared.checkpoint()
        PlaybackStore.post(.surfaceInvalidated(key))
        registry.forget(wallpaperID: uuid)
        scheduleTeardown(for: key)
        extensionLog("=== INVALIDATE === \(key) → tear down in \(Lifecycle.teardownGrace)s unless re-acquired")
        reply(nil)
    }

    func snapshot(withId id: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        markServed()
        let registry = SurfaceRegistry.shared
        let key = extractWallpaperUUID(fromID: id).flatMap { registry.key(forWallpaperID: $0) }
        let sample = key.flatMap { registry.surface(for: $0)?.renderer?.clockSample() }
        let fallback = VideoLibrary.shared.config.playlist.videoIDs.compactMap { VideoLibrary.shared.videoURL(for: $0) }.first
        guard let url = sample?.url ?? fallback else { reply(nil, nil); return }
        let time = sample.map { CMTime(seconds: $0.seconds, preferredTimescale: 60_000) }
        Task { await reply(createSnapshotViaRuntime(videoURL: url, currentTime: time), nil) }
    }

    // MARK: - Settings

    func provideSettingsViewModels(withContentTypes _: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        markServed()
        traceLog("=== PROVIDE SETTINGS VIEW MODELS ===")

        Task {
            if let result = await buildSettingsViewModelsXPC() {
                traceLog("  [Settings] Remapped to \(NSStringFromClass(type(of: result as AnyObject)))")
                reply(result, nil)
            } else {
                traceLog("  [Settings] Build failed, using empty fallback")
                reply(makeEmptyGroupsResponse(), nil)
            }
        }
    }

    // MARK: - Choices

    func addChoiceRequest(withChoiceRequest _: Any?, onBehalfOfProcess _: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        markServed()
        traceLog("=== ADD CHOICE REQUEST ===")
        reply(nil, nil)
    }

    /// WallpaperAgent routes a removal request by its case (image, folder, color,
    /// screensaver, Photos) to Apple's own providers, so none reaches a third-party
    /// extension. Video tiles are deleted through `removeDownload(for:)` instead.
    func removeChoiceRequest(withChoiceRequest _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        markServed()
        extensionLog("=== REMOVE CHOICE REQUEST === (not routed to third-party providers; ignored)")
        reply(nil)
    }

    func selectedChoicesDidChange(for id: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        markServed()
        nonisolated(unsafe) let unsafeID = id
        nonisolated(unsafe) let handler = self
        Lifecycle.queue.async { handler.selectedChoicesDidChangeBody(id: unsafeID, reply: reply) }
    }

    /// The agent sends this to Aerials but not to third-party providers on macOS 27;
    /// the acquire that follows a pick is what carries the choice. Refresh snapshots
    /// so the picker re-fetches the new video's still.
    private func selectedChoicesDidChangeBody(id _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        extensionLog("=== SELECTED CHOICES DID CHANGE ===")
        agentProxy?.invalidateSnapshots { error in
            if let error {
                extensionLog("  [Choice] invalidateSnapshots error: \(error)")
            }
        }
        reply(nil)
    }

    func invokeContextMenuAction(withMenuItemID menuItemID: Any?, groupItemID _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        let identifier = (menuItemID as? String) ?? String(describing: menuItemID ?? "nil")
        extensionLog("=== CONTEXT MENU ACTION === identifier: \(identifier)")

        let urlByAction = [
            "add-video": "phosphenego://add-video",
            "manage-library": "phosphenego://library",
        ]
        if let urlString = urlByAction[identifier], let url = URL(string: urlString) {
            extensionLog("  Launching companion app via NSWorkspace: \(urlString)")
            let opened = NSWorkspace.shared.open(url)
            traceLog("  NSWorkspace.open = \(opened)")
        }

        reply(nil)
    }

    // MARK: - Downloads

    func isChoiceDownloaded(with _: Any?, reply: @escaping @Sendable (NSNumber?, (any Error)?) -> Void) {
        markServed()
        traceLog("isChoiceDownloaded")
        reply(NSNumber(value: true), nil)
    }

    func download(withChoiceID _: Any?, reply: ((any Error)?) -> Void) -> Any? {
        traceLog("download")
        reply(nil)
        return nil
    }

    func pauseDownload(for _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        reply(nil)
    }

    func cancelDownload(for _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        reply(nil)
    }

    func resumeDownload(for _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        reply(nil)
    }

    func removeDownload(for _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        // This provider has one non-disposable playlist; only the Go app deletes media.
        reply(nil)
    }

    // MARK: - Migration

    func migrateSelectedChoice(for _: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        traceLog("migrateSelectedChoice")
        reply(nil, nil)
    }

    func migrate(from _: Any?, to _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        traceLog("migrate")
        reply(nil)
    }

    // MARK: - Shuffle

    func skipShuffledContent(withId _: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        markServed()
        reply(nil)
    }

    func canSkipShuffledContent(withId _: Any?, reply: @escaping @Sendable (Bool, (any Error)?) -> Void) {
        traceLog("canSkipShuffledContent")
        reply(false, nil)
    }

    // MARK: - Debug & Notifications

    func handleDebugRequest(for _: Any?, reply: @escaping @Sendable (Any?, (any Error)?) -> Void) {
        traceLog("handleDebugRequest")
        reply(nil, nil)
    }

    func handleNotification(withNamed name: Any?, reply: @escaping @Sendable ((any Error)?) -> Void) {
        traceLog("handleNotification(\(name ?? "nil"))")
        reply(nil)
    }
}

/// Recursively search a value's `Mirror` for a stored property with the given
/// label, to a shallow depth. Robust to the XPC wrapper nesting, unlike scanning
/// a stringified description.
private func mirrorFindProperty(_ label: String, in value: Any, depth: Int = 0) -> Any? {
    guard depth < 6 else { return nil }
    for child in Mirror(reflecting: value).children {
        if child.label == label {
            return child.value
        }
        if let found = mirrorFindProperty(label, in: child.value, depth: depth + 1) {
            return found
        }
    }
    return nil
}

/// Extract an enum case name from a value: `.idle` → `"idle"`,
/// `.suspended(reason)` → `"suspended"`. Falls back to `String(describing:)` for
/// non-enums or payload-less cases (whose description is already the case name).
private func enumCaseName(_ value: Any) -> String {
    let mirror = Mirror(reflecting: value)
    if mirror.displayStyle == .enum, let label = mirror.children.first?.label {
        return label
    }
    return String(describing: value)
}
