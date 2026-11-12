import Foundation

let playlistChoiceID = "managed-playlist"

/// Each remote CAContext must belong to one hosted surface. Logical progress belongs
/// to its stable display UUID and is shared across that display's surfaces.
nonisolated struct SurfaceKey: Hashable, Sendable, CustomStringConvertible {
    let displayID: UInt32
    let surfaceUUID: UUID
    var description: String {
        "display \(displayID)/\(surfaceUUID.uuidString.prefix(8))"
    }
}
nonisolated enum SurfaceRole: Equatable, Sendable { case desktop, preview }
nonisolated struct Surface: Equatable, Sendable {
    var displayID: UInt32?
    var role: SurfaceRole
    var choice: String?
    var mode: PresentationMode = .default
    var activity: ActivityState = .active
    var generation: Int
}
nonisolated struct PlaybackState: Equatable, Sendable {
    var surfaces: [SurfaceKey: Surface] = [:]
    var screenLocked = false
    var displaysAsleep = false
    var nextGeneration = 0
}
nonisolated enum PlaybackEvent: Sendable {
    case surfaceAcquired(SurfaceKey, displayID: UInt32?, role: SurfaceRole, choice: String?)
    case surfaceRemoved(SurfaceKey)
    case surfaceInvalidated(SurfaceKey)
    case agentUpdate(SurfaceKey?, mode: PresentationMode, activity: ActivityState)
    case screenLockChanged(Bool)
    case displaysAsleepChanged(Bool)
}
nonisolated struct SurfaceTarget: Equatable, Sendable {
    var policy: PlaybackPolicy
    static let paused = SurfaceTarget(policy: .paused)
}
nonisolated enum PlaybackReducer {
    static func reduce(_ state: PlaybackState, _ event: PlaybackEvent) -> PlaybackState {
        var next = state
        switch event {
        case let .surfaceAcquired(key, displayID, role, choice):
            var surface = next.surfaces[key] ?? Surface(displayID: displayID, role: role, choice: choice, generation: next.nextGeneration)
            surface.choice = choice; surface.role = role; surface.displayID = displayID
            surface.activity = .active
            surface.generation = next.nextGeneration
            next.nextGeneration += 1
            next.surfaces[key] = surface
        case let .surfaceRemoved(key): next.surfaces[key] = nil
        case let .surfaceInvalidated(key): next.surfaces[key]?.activity = .suspended
        case let .agentUpdate(key, mode, activity):
            for key in key.map({ [$0] }) ?? Array(next.surfaces.keys) {
                next.surfaces[key]?.mode = mode; next.surfaces[key]?.activity = activity
            }
        case let .screenLockChanged(locked): next.screenLocked = locked
        case let .displaysAsleepChanged(asleep): next.displaysAsleep = asleep
        }
        return next
    }
    static func target(for surface: Surface, in state: PlaybackState) -> SurfaceTarget {
        SurfaceTarget(policy: state.displaysAsleep || surface.activity == .suspended ? .paused : .full)
    }
    static func targets(_ state: PlaybackState) -> [SurfaceKey: SurfaceTarget] {
        state.surfaces.mapValues { target(for: $0, in: state) }
    }
}
