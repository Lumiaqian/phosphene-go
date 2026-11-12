import Foundation

/// JSON v1. All durations and positions are seconds; dates are Unix seconds.
struct MediaEntry: Codable, Equatable, Sendable {
    let id: String
    var name: String
    let filename: String
    let width: Int
    let height: Int
    let durationSeconds: Double
    let frameRate: Double
    let addedAtUnix: Double
}
struct PlaylistSpec: Codable, Equatable, Sendable {
    var videoIDs: [String] = []
    var advanceMode = "end"
    var intervalSeconds = 60
    var keepAwakeWhenLockedOnAC = true
}
struct LibraryConfig: Codable, Equatable, Sendable {
    var schemaVersion = 1
    var revision: Int64 = 0
    var media: [MediaEntry] = []
    var playlist = PlaylistSpec()

    func validated() throws -> Self {
        guard schemaVersion == 1, ["end", "duration"].contains(playlist.advanceMode),
              playlist.intervalSeconds > 0,
              Set(media.map(\.id)).count == media.count,
              Set(playlist.videoIDs).count == playlist.videoIDs.count,
              Set(playlist.videoIDs).isSubset(of: Set(media.map(\.id))),
              media.allSatisfy({ UUID(uuidString: $0.id) != nil &&
                      !$0.filename.isEmpty && $0.filename != "." && $0.filename != ".." &&
                      !$0.filename.contains("/") && !$0.filename.contains("\\") &&
                      $0.durationSeconds.isFinite && $0.durationSeconds > 0 &&
                      $0.frameRate.isFinite && $0.frameRate > 0 && $0.width > 0 && $0.height > 0 })
        else { throw CocoaError(.fileReadCorruptFile) }
        return self
    }
}
struct DisplayProgress: Codable, Equatable, Sendable {
    var displayKey: String
    var videoID: String
    var positionSeconds: Double = 0
    var itemElapsedSeconds: Double = 0
}
struct RuntimeState: Codable, Sendable {
    var schemaVersion = 1
    var appliedRevision: Int64 = 0
    var ruleSignature: String?
    var updatedAtUnix: Double = 0
    var displays: [DisplayProgress] = []
    var activeDisplayKeys: [String] = []
    var openVideoIDs: [String] = []
    var keepAwakeActive = false
    var errors: [String] = []
}

/// Pure timeline driven ONLY by deltas from a renderer's presentation timebase.
/// Decoder EOF is intentionally absent: it can precede display by many frames.
enum PlaylistTimeline {
    static func cycleDuration(_ items: [MediaEntry], _ rule: PlaylistSpec) -> Double {
        rule.advanceMode == "duration" ? Double(rule.intervalSeconds) * Double(items.count) : items.reduce(0) { $0 + $1.durationSeconds }
    }

    static func initial(key: String, index: Int, displayCount: Int, items: [MediaEntry], rule: PlaylistSpec) -> DisplayProgress? {
        guard let first = items.first else { return nil }
        var progress = DisplayProgress(displayKey: key, videoID: first.id)
        advance(&progress, seconds: cycleDuration(items, rule) * Double(index) / Double(max(1, displayCount)), items: items, rule: rule)
        return progress
    }

    @discardableResult
    static func advance(_ progress: inout DisplayProgress, seconds: Double, items: [MediaEntry], rule: PlaylistSpec) -> Bool {
        guard !items.isEmpty, seconds.isFinite, seconds >= 0 else { return false }
        var crossedBoundary = false
        var index = items.firstIndex { $0.id == progress.videoID } ?? 0
        if items[index].id != progress.videoID {
            progress.videoID = items[index].id; progress.positionSeconds = 0; progress.itemElapsedSeconds = 0
        }
        var delta = seconds
        // A complete cycle lands on the same position, avoiding an unbounded loop.
        let cycle = cycleDuration(items, rule)
        if delta > cycle, cycle > 0 {
            delta.formTruncatingRemainder(dividingBy: cycle); crossedBoundary = true
        }
        while true {
            let item = items[index]
            progress.positionSeconds = max(0, progress.positionSeconds).truncatingRemainder(dividingBy: item.durationSeconds)
            let remaining = rule.advanceMode == "duration"
                ? max(0, Double(rule.intervalSeconds) - progress.itemElapsedSeconds)
                : item.durationSeconds - progress.positionSeconds
            if delta + 0.000001 < remaining {
                progress.positionSeconds = (progress.positionSeconds + delta).truncatingRemainder(dividingBy: item.durationSeconds)
                progress.itemElapsedSeconds += delta
                break
            }
            crossedBoundary = true
            delta = max(0, delta - remaining)
            index = (index + 1) % items.count
            progress.videoID = items[index].id; progress.positionSeconds = 0; progress.itemElapsedSeconds = 0
            if delta == 0 {
                break
            }
        }
        return crossedBoundary
    }

    static func reconcile(_ progress: inout DisplayProgress, oldOrder: [String], items: [MediaEntry], resetElapsed: Bool) {
        guard !items.isEmpty else { return } // hold the last frame and saved progress
        if let item = items.first(where: { $0.id == progress.videoID }) {
            progress.positionSeconds = max(0, progress.positionSeconds).truncatingRemainder(dividingBy: item.durationSeconds)
            if resetElapsed {
                progress.itemElapsedSeconds = 0
            }
            return
        }
        let oldIndex = oldOrder.firstIndex(of: progress.videoID) ?? -1
        let candidates = Array(oldOrder.dropFirst(oldIndex + 1)) + Array(oldOrder.prefix(oldIndex + 1))
        progress.videoID = candidates.first(where: { id in items.contains { $0.id == id } }) ?? items[0].id
        progress.positionSeconds = 0; progress.itemElapsedSeconds = 0
    }
}

/// Samples from different renderers or decoder epochs have unrelated time origins.
/// Resetting the baseline prevents seeking or switching surfaces from counting as play time.
struct PresentationClock {
    private var rendererID: Int?
    private var epoch: Int?
    private var seconds: Double = 0

    mutating func delta(rendererID: Int, epoch: Int, seconds: Double) -> Double {
        guard seconds.isFinite else { return 0 }
        defer { self.rendererID = rendererID; self.epoch = epoch; self.seconds = seconds }
        guard self.rendererID == rendererID, self.epoch == epoch else { return 0 }
        return max(0, seconds - self.seconds)
    }
}
