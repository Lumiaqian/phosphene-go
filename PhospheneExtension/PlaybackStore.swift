import Foundation
import Observation
import os

/// One surface's current target, observed by that surface's renderer alone. Keeping
/// each target in its own object means a change on one surface wakes only the
/// renderer following it.
@MainActor @Observable
final class SurfaceTargetCell {
    var target: SurfaceTarget

    init(_ target: SurfaceTarget) {
        self.target = target
    }
}

/// The single owner of playback state. Every input arrives as a `PlaybackEvent`
/// through `send`, which runs the pure reducer and republishes the derived targets.
/// Renderers follow their surface's target with `Observations`, so a renderer that
/// attaches late starts from the current target and never misses an earlier event.
@MainActor @Observable
final class PlaybackStore {
    static let shared = PlaybackStore()

    private(set) var state: PlaybackState

    @ObservationIgnored private var cells: [SurfaceKey: SurfaceTargetCell] = [:]

    /// A copy of `state` readable from any thread, for code on the XPC and renderer
    /// queues that needs to peek without hopping to the main actor.
    private nonisolated static let mirror = OSAllocatedUnfairLock(initialState: PlaybackState())

    private init() {
        state = PlaybackState()
    }

    // MARK: - Events

    /// Deliver an event from any thread. Delivery is synchronous on the main thread
    /// and FIFO from every other thread, so events from one source keep their order.
    nonisolated static func post(_ event: PlaybackEvent) {
        if Thread.isMainThread {
            MainActor.assumeIsolated { shared.send(event) }
        } else {
            DispatchQueue.main.async { MainActor.assumeIsolated { shared.send(event) } }
        }
    }

    func send(_ event: PlaybackEvent) {
        let next = PlaybackReducer.reduce(state, event)
        traceLog("[Store] \(event)")
        guard next != state else { return }
        state = next
        Self.mirror.withLock { $0 = next }
        republishTargets()
        PlaylistController.shared.lifecycleChanged()
    }

    private func republishTargets() {
        let targets = PlaybackReducer.targets(state)
        for (key, target) in targets {
            if let cell = cells[key] {
                if cell.target != target {
                    cell.target = target
                }
            } else {
                cells[key] = SurfaceTargetCell(target)
            }
        }
        for key in cells.keys where targets[key] == nil {
            cells[key] = nil
        }
    }

    // MARK: - Renderers

    /// Make `renderer` follow `key`'s target for as long as it runs. The current target
    /// is applied before this returns, so a `start()` issued afterwards already plays
    /// or holds according to it.
    func follow(_ renderer: VideoRenderer, surface key: SurfaceKey) {
        guard let cell = cells[key] else {
            extensionLog("[Store] follow: no surface \(key) — renderer #\(renderer.debugID) held paused")
            renderer.apply(.paused)
            return
        }
        renderer.apply(cell.target)
        renderer.setFollowTask(Task(name: "Follow \(key)") {
            for await target in Observations({ cell.target }) {
                renderer.apply(target)
            }
        })
    }

    // MARK: - Snapshot reads

    nonisolated static var snapshot: PlaybackState {
        mirror.withLock { $0 }
    }
}
