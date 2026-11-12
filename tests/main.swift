import Foundation

func require(_ condition: @autoclosure () -> Bool, _ message: String) {
    if !condition() {
        fatalError(message)
    }
}
func close(_ lhs: Double, _ rhs: Double) -> Bool {
    abs(lhs - rhs) < 0.00001
}
let config = try JSONDecoder().decode(LibraryConfig.self, from: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))).validated()
try JSONEncoder().encode(config).write(to: URL(fileURLWithPath: CommandLine.arguments[2]), options: .atomic)
let items = config.media
var rule = config.playlist
var a = PlaylistTimeline.initial(key: "display-A", index: 0, displayCount: 2, items: items, rule: rule)!
let b = PlaylistTimeline.initial(key: "display-B", index: 1, displayCount: 2, items: items, rule: rule)!
require(a.videoID == items[0].id && b.videoID == items[1].id && close(b.positionSeconds, 61.5), "even initial staggering")
PlaylistTimeline.advance(&a, seconds: 6.99, items: items, rule: rule)
require(a.videoID == items[0].id, "must not switch when decoder has read ahead")
PlaylistTimeline.advance(&a, seconds: 0.01, items: items, rule: rule)
require(a.videoID == items[1].id && close(a.positionSeconds, 0), "switch on presentation boundary")
require(close(b.positionSeconds, 61.5), "display-A must not modify display-B")
let saved = try JSONEncoder().encode(b)
let restored = try JSONDecoder().decode(DisplayProgress.self, from: saved)
require(restored == b, "stable display progress round trip")
rule.advanceMode = "duration"
a = DisplayProgress(displayKey: "display-A", videoID: items[0].id)
PlaylistTimeline.advance(&a, seconds: 59, items: items, rule: rule)
require(a.videoID == items[0].id && close(a.positionSeconds, 3) && close(a.itemElapsedSeconds, 59), "short video loops until dwell deadline")
PlaylistTimeline.advance(&a, seconds: 1, items: items, rule: rule)
require(a.videoID == items[1].id && close(a.positionSeconds, 0), "short video deadline")
PlaylistTimeline.advance(&a, seconds: 60, items: items, rule: rule)
require(a.videoID == items[2].id, "long video cut at deadline")
a = b
PlaylistTimeline.reconcile(&a, oldOrder: rule.videoIDs, items: items, resetElapsed: true)
require(close(a.positionSeconds, b.positionSeconds) && a.itemElapsedSeconds == 0, "rule edit preserves position and resets dwell")
PlaylistTimeline.reconcile(&a, oldOrder: rule.videoIDs, items: [items[0], items[2]], resetElapsed: false)
require(a.videoID == items[2].id, "removed or bad item skips to next ordered item")
let held = a
PlaylistTimeline.reconcile(&a, oldOrder: rule.videoIDs, items: [], resetElapsed: false)
PlaylistTimeline.advance(&a, seconds: 20, items: [], rule: rule)
require(a == held, "empty list holds final position")
a = DisplayProgress(displayKey: "one", videoID: items[0].id)
require(PlaylistTimeline.advance(&a, seconds: 60, items: [items[0]], rule: rule), "single video dwell boundary must restart its item")
require(a.positionSeconds == 0, "single video position")
rule.advanceMode = "end"
PlaylistTimeline.advance(&a, seconds: 100_000_000, items: items, rule: rule)
require(a.positionSeconds.isFinite, "large deltas are bounded")
var bad = config; bad.playlist.intervalSeconds = 0
require((try? bad.validated()) == nil, "invalid interval rejected")
let key = SurfaceKey(displayID: 2, surfaceUUID: UUID())
var state = PlaybackState()
state = PlaybackReducer.reduce(state, .surfaceAcquired(key, displayID: 2, role: .desktop, choice: playlistChoiceID))
require(PlaybackReducer.targets(state)[key]?.policy == .full, "desktop runs without application pause policies")
state = PlaybackReducer.reduce(state, .screenLockChanged(true))
require(PlaybackReducer.targets(state)[key]?.policy == .full, "lock screen runs")
state = PlaybackReducer.reduce(state, .displaysAsleepChanged(true))
require(PlaybackReducer.targets(state)[key]?.policy == .paused, "system sleep releases resources")
state = PlaybackReducer.reduce(state, .displaysAsleepChanged(false))
require(PlaybackReducer.targets(state)[key]?.policy == .full, "wake resumes")
state = PlaybackReducer.reduce(state, .agentUpdate(key, mode: .default, activity: .suspended))
require(PlaybackReducer.targets(state)[key]?.policy == .paused, "system inactive surface suspended")
print("PASS: Swift timeline, lifecycle and Go → Swift contract")
var clock = PresentationClock()
require(clock.delta(rendererID: 1, epoch: 0, seconds: 50) == 0, "first presentation sample is baseline")
require(close(clock.delta(rendererID: 1, epoch: 0, seconds: 51), 1), "presentation clock advances")
require(clock.delta(rendererID: 1, epoch: 1, seconds: 120) == 0, "seek must not count as played time")
require(clock.delta(rendererID: 2, epoch: 0, seconds: 300) == 0, "surface switch must not count as played time")
state = PlaybackReducer.reduce(state, .surfaceInvalidated(key))
require(PlaybackReducer.targets(state)[key]?.policy == .paused, "invalidated surface releases playback immediately")
state = PlaybackReducer.reduce(state, .surfaceAcquired(key, displayID: 2, role: .desktop, choice: playlistChoiceID))
require(PlaybackReducer.targets(state)[key]?.policy == .full, "reacquired surface resumes")
print("PASS: presentation-clock epochs and surface reactivation")

let fixtureDirectory = URL(fileURLWithPath: CommandLine.arguments[1]).deletingLastPathComponent()
let goRuntime = try JSONDecoder().decode(RuntimeState.self, from: Data(contentsOf: fixtureDirectory.appendingPathComponent("go-runtime.json")))
require(goRuntime.displays[0].displayKey == "display-A" && goRuntime.displays[0].positionSeconds == 61.5, "Go runtime is read in seconds")
var swiftRuntime = RuntimeState()
var savedDisplay = b
savedDisplay.displayKey = "display-B"
swiftRuntime.displays = [savedDisplay]
try JSONEncoder().encode(swiftRuntime).write(to: fixtureDirectory.appendingPathComponent("runtime.json"), options: .atomic)
let display = CGSize(width: 2_560, height: 1_440)
require(GeometryVerdict.decide(laidOutSize: display, laidOutScale: 2, displaySize: display, displayScale: 2) == .matches, "matching display geometry stays")
require(GeometryVerdict.decide(laidOutSize: CGSize(width: 5_120, height: 2_880), laidOutScale: 1, displaySize: display, displayScale: 2) == .reframe(size: display, scale: 2), "pixel-sized destination is reframed")
require(GeometryVerdict.decide(laidOutSize: CGSize(width: 1_280, height: 720), laidOutScale: 4, displaySize: display, displayScale: 2) == .reframe(size: display, scale: 2), "scaled destination is reframed")
require(GeometryVerdict.decide(laidOutSize: CGSize(width: 1_920, height: 1_080), laidOutScale: 2, displaySize: display, displayScale: 2) == .differentPixels, "resolution change waits for system reacquire")
print("PASS: display geometry and Go ↔ Swift runtime progress")
