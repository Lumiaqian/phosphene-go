import Foundation

/// How WallpaperAgent is presenting one surface, from the `presentationMode` of an
/// `update` request.
nonisolated enum PresentationMode: Equatable, Sendable, CustomStringConvertible {
    /// The ordinary desktop.
    case `default`
    /// The lock screen.
    case locked
    /// The screensaver slot: ours when a Phosphene choice is the screensaver, a
    /// foreign screensaver covering us otherwise.
    case idle
    case other(String)

    /// Map the agent's enum case name (read via `Mirror`) to a mode.
    init(caseName: String) {
        switch caseName {
        case "default": self = .default
        case "locked": self = .locked
        case "idle": self = .idle
        default: self = .other(caseName)
        }
    }

    var description: String {
        switch self {
        case .default: "default"
        case .locked: "locked"
        case .idle: "idle"
        case let .other(name): name
        }
    }
}

/// Whether WallpaperAgent considers one surface active, from the `activityState` of
/// an `update` request.
nonisolated enum ActivityState: Equatable, Sendable, CustomStringConvertible {
    case active
    /// Every suspended variant: the surface is not being composited.
    case suspended
    case other(String)

    init(caseName: String) {
        if caseName == "active" {
            self = .active
        } else if caseName.contains("suspended") {
            self = .suspended
        } else {
            self = .other(caseName)
        }
    }

    var description: String {
        switch self {
        case .active: "active"
        case .suspended: "suspended"
        case let .other(name): name
        }
    }
}

/// Only system lifecycle may suspend a surface. No application pause preferences.
nonisolated enum PlaybackPolicy: Int, Sendable {
    case full
    case paused
}
