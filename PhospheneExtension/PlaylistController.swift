import AppKit
import CoreGraphics
import Foundation

/// One logical session per stable display UUID. Remote layers stay per-surface.
/// The serial queue owns sessions, clock observations and the progress file.
final class PlaylistController: @unchecked Sendable {
    static let shared = PlaylistController()
    private let queue = DispatchQueue(label: "local.lumiaqian.phosphenego.playlist")
    private var config = LibraryConfig()
    private var sessions: [String: DisplayProgress] = [:]
    private var displayKeys: [UInt32: String] = [:]
    private var rejected = Set<String>()
    private var clocks: [String: PresentationClock] = [:]
    private var leaders: [String: Int] = [:]
    private var persistedRule: String?
    private var applied: [Int: String] = [:]
    private var timer: (any DispatchSourceTimer)?
    private var lastSave = Date.distantPast
    private var activeKeys = Set<String>()
    private var lastOpenIDs = Set<String>()
    private var errors: [String] = []
    private let awake = LockDisplayAssertion()

    private init() {
        if let data = try? Data(contentsOf: Self.runtimeURL),
           let saved = try? JSONDecoder().decode(RuntimeState.self, from: data), saved.schemaVersion == 1 {
            persistedRule = saved.ruleSignature
            for progress in saved.displays where progress.positionSeconds.isFinite && progress.itemElapsedSeconds.isFinite {
                sessions[progress.displayKey] = progress
            }
        }
    }
    private static var runtimeURL: URL {
        VideoLibrary.documents.appendingPathComponent("runtime.json")
    }
    private var items: [MediaEntry] {
        config.playlist.videoIDs.compactMap { id in
            guard !rejected.contains(id), VideoLibrary.shared.videoURL(for: id) != nil else { return nil }
            return config.media.first { $0.id == id }
        }
    }

    func start() {
        queue.async { [self] in
            reloadOnQueue()
            let timer = DispatchSource.makeTimerSource(queue: queue)
            timer.schedule(deadline: .now(), repeating: .milliseconds(100))
            timer.setEventHandler { [weak self] in self?.tick() }
            self.timer = timer; timer.resume()
        }
    }
    func reload() {
        queue.async { [self] in reloadOnQueue(); tick(); save() }
    }
    func lifecycleChanged() {
        queue.async { [self] in tick(); save() }
    }
    func checkpoint() {
        queue.sync { tick(); save() }
    }
    func mediaFailed(_ id: String?, message: String) {
        guard let id else { return }
        queue.async { [self] in reject(id, message: message); tick(); save() }
    }
    private func reject(_ id: String, message: String) {
        guard rejected.insert(id).inserted else { return }
        errors.append("无法解码：\(config.media.first { $0.id == id }?.name ?? id)（\(message)）")
        extensionLog("[Playlist] Rejected media \(id): \(message)")
    }

    /// Previews use the first item and never create or advance a display session.
    func selection(displayID: UInt32, preview: Bool) -> DisplayProgress? {
        queue.sync {
            let playable = items
            guard !preview else { return playable.first.map { DisplayProgress(displayKey: "preview", videoID: $0.id) } }
            refreshDisplays()
            guard let key = displayKeys[displayID] else { return nil }
            ensureSession(key: key, items: playable)
            return sessions[key]
        }
    }
    private func reloadOnQueue() {
        let old = config
        config = VideoLibrary.shared.config
        rejected.removeAll(); errors.removeAll()
        let signature = "\(config.playlist.advanceMode):\(config.playlist.intervalSeconds)"
        let ruleChanged = persistedRule.map { $0 != signature } ?? (old.playlist.advanceMode != config.playlist.advanceMode || old.playlist.intervalSeconds != config.playlist.intervalSeconds)
        persistedRule = signature
        let playable = items
        for key in sessions.keys {
            PlaylistTimeline.reconcile(&sessions[key]!, oldOrder: old.playlist.videoIDs, items: playable, resetElapsed: ruleChanged)
        }
        if ruleChanged {
            clocks.removeAll()
        }
    }
    private func refreshDisplays() {
        var count: UInt32 = 0
        CGGetActiveDisplayList(0, nil, &count)
        var ids = [CGDirectDisplayID](repeating: 0, count: Int(count))
        CGGetActiveDisplayList(count, &ids, &count)
        for id in ids {
            if let uuid = CGDisplayCreateUUIDFromDisplayID(id)?.takeRetainedValue() {
                displayKeys[id] = CFUUIDCreateString(nil, uuid) as String
            }
        }
    }
    private func ensureSession(key: String, items: [MediaEntry]) {
        guard sessions[key] == nil else { return }
        // Order comes from the system's current active display list, not names.
        var count: UInt32 = 0
        CGGetActiveDisplayList(0, nil, &count)
        var ids = [CGDirectDisplayID](repeating: 0, count: Int(count))
        CGGetActiveDisplayList(count, &ids, &count)
        let keys = ids.compactMap { displayKeys[$0] }
        sessions[key] = PlaylistTimeline.initial(
            key: key,
            index: keys.firstIndex(of: key) ?? 0,
            displayCount: keys.count,
            items: items,
            rule: config.playlist,
        )
    }

    private func tick() {
        refreshDisplays()
        let state = PlaybackStore.snapshot
        let registry = SurfaceRegistry.shared
        var playable = items
        var active = Set<String>()
        var open = Set<String>()
        var seenRenderers = Set<Int>()
        let grouped = Dictionary(grouping: state.surfaces.filter { $0.value.role == .desktop }, by: { displayKeys[$0.key.displayID] ?? "" })
        for (key, surfaces) in grouped where !key.isEmpty {
            ensureSession(key: key, items: playable)
            guard var progress = sessions[key] else { continue }
            PlaylistTimeline.reconcile(&progress, oldOrder: config.playlist.videoIDs, items: playable, resetElapsed: false)
            var crossedBoundary = false
            // Prefer an actually presented surface; preview clocks are never leaders.
            let visible = surfaces.filter { CGDisplayIsActive($0.key.displayID) != 0 && PlaybackReducer.target(for: $0.value, in: state).policy == .full }
            let leader = visible.sorted { $0.value.generation > $1.value.generation }
                .compactMap { registry.surface(for: $0.key)?.renderer }.first
            if !visible.isEmpty {
                active.insert(key)
            }
            if let leader, !playable.isEmpty {
                let sample = leader.clockSample()
                if sample.failed {
                    reject(progress.videoID, message: "原生解码失败")
                    playable = items
                    PlaylistTimeline.reconcile(&progress, oldOrder: config.playlist.videoIDs, items: playable, resetElapsed: true)
                    clocks[key] = nil
                } else if sample.ready, sample.url == VideoLibrary.shared.videoURL(for: progress.videoID) {
                    let mediaDuration = config.media.first { $0.id == progress.videoID }?.durationSeconds ?? 1
                    let actual = sample.seconds.truncatingRemainder(dividingBy: mediaDuration)
                    let distance = abs(actual - progress.positionSeconds)
                    if leaders[key] != leader.debugID, min(distance, mediaDuration - distance) > 0.25,
                       let url = VideoLibrary.shared.videoURL(for: progress.videoID) {
                        leader.play(url, at: progress.positionSeconds, force: true)
                        clocks[key] = nil
                    } else {
                        var clock = clocks[key] ?? PresentationClock()
                        let delta = clock.delta(rendererID: leader.debugID, epoch: sample.epoch, seconds: sample.seconds)
                        clocks[key] = clock
                        crossedBoundary = PlaylistTimeline.advance(&progress, seconds: delta, items: playable, rule: config.playlist)
                        if crossedBoundary {
                            lastSave = .distantPast
                        }
                    }
                    leaders[key] = leader.debugID
                } else {
                    clocks[key] = nil
                }
            } else {
                clocks[key] = nil; leaders[key] = nil
            }

            sessions[key] = progress
            for (surfaceKey, surface) in surfaces {
                guard let renderer = registry.surface(for: surfaceKey)?.renderer else {
                    attachRenderer(to: surfaceKey, progress: sessions[key])
                    continue
                }
                seenRenderers.insert(renderer.debugID)
                let sample = renderer.clockSample()
                // A removed media file stays on disk until every native reader lets go.
                for url in sample.openURLs {
                    open.insert(url.deletingLastPathComponent().lastPathComponent)
                }
                if playable.isEmpty {
                    renderer.apply(.paused)
                    continue
                }
                PlaylistTimeline.reconcile(&sessions[key]!, oldOrder: config.playlist.videoIDs, items: playable, resetElapsed: false)
                guard let current = sessions[key], let url = VideoLibrary.shared.videoURL(for: current.videoID) else { continue }
                if crossedBoundary || applied[renderer.debugID] != current.videoID {
                    if crossedBoundary || sample.url != url {
                        renderer.play(url, at: current.positionSeconds, force: crossedBoundary)
                    }
                    applied[renderer.debugID] = current.videoID
                }
                let target = CGDisplayIsActive(surfaceKey.displayID) == 0 ? .paused : PlaybackReducer.target(for: surface, in: state)
                renderer.apply(target)
            }
        }
        for surfaceKey in registry.surfaceKeys {
            guard let surface = registry.surface(for: surfaceKey), surface.isPreview, let renderer = surface.renderer else { continue }
            for url in renderer.clockSample().openURLs {
                open.insert(url.deletingLastPathComponent().lastPathComponent)
            }
            // Keep previews current after edits, without creating any persistent session.
            if let first = playable.first, let url = VideoLibrary.shared.videoURL(for: first.id) {
                renderer.play(url, at: 0)
            } else {
                renderer.apply(.paused)
            }
        }
        applied = applied.filter { seenRenderers.contains($0.key) }
        if active != activeKeys || open != lastOpenIDs {
            lastSave = .distantPast
        }
        activeKeys = active; lastOpenIDs = open
        awake.update(
            enabled: config.playlist.keepAwakeWhenLockedOnAC,
            locked: state.screenLocked,
            active: !active.isEmpty && !playable.isEmpty,
            asleep: state.displaysAsleep,
        )
        if Date().timeIntervalSince(lastSave) >= 5 {
            save()
        }
    }
    /// Reattach after an empty initial list becomes playable or a corrupt first
    /// asset was rejected. The registry claim prevents racing the XPC acquire task.
    private func attachRenderer(to key: SurfaceKey, progress: DisplayProgress?) {
        guard let progress, let url = VideoLibrary.shared.videoURL(for: progress.videoID),
              !items.isEmpty, let surface = SurfaceRegistry.shared.surface(for: key),
              SurfaceRegistry.shared.claimRendererCreate(for: key) else { return }
        let root = SendableBox(value: surface.rootLayer)
        Task {
            do {
                let renderer = try await VideoRenderer.create(rootLayer: root.value, videoURL: url, position: progress.positionSeconds)
                guard SurfaceRegistry.shared.surface(for: key) != nil else { renderer.stop(); return }
                let old = SurfaceRegistry.shared.setRenderer(renderer, videoID: playlistChoiceID, for: key)
                old?.stop()
                await PlaybackStore.shared.follow(renderer, surface: key)
                await renderer.start()
            } catch {
                SurfaceRegistry.shared.clearRendererPending(for: key)
                self.mediaFailed(progress.videoID, message: error.localizedDescription)
            }
        }
    }

    private func save() {
        let runtime = RuntimeState(
            appliedRevision: config.revision,
            ruleSignature: persistedRule,
            updatedAtUnix: Date().timeIntervalSince1970,
            displays: sessions.values.sorted { $0.displayKey < $1.displayKey },
            activeDisplayKeys: activeKeys.sorted(),
            openVideoIDs: lastOpenIDs.sorted(),
            keepAwakeActive: awake.isActive,
            errors: items.isEmpty ? errors + ["播放列表为空或没有可播放的视频，保留末帧"] : errors,
        )
        do {
            try JSONEncoder().encode(runtime).write(to: Self.runtimeURL, options: .atomic)
            lastSave = Date()
        } catch { extensionLog("[Playlist] Cannot save progress: \(error)") }
    }
}
