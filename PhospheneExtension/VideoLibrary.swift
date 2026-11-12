import Foundation
import os

// The extension only reads configuration. The Go app is the sole library writer.
typealias VideoEntry = MediaEntry
final class VideoLibrary: Sendable {
    static let shared = VideoLibrary()
    static var documents: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Documents")
    }
    private let lock = OSAllocatedUnfairLock(initialState: LibraryConfig())
    private let videosDir = documents.appendingPathComponent("videos")
    var config: LibraryConfig {
        lock.withLock { $0 }
    }
    var entries: [MediaEntry] {
        config.media
    }

    func entry(for id: String) -> MediaEntry? {
        entries.first { $0.id == id }
    }
    func videoURL(for entry: MediaEntry) -> URL {
        videosDir.appendingPathComponent(entry.id).appendingPathComponent(entry.filename)
    }
    func videoURL(for id: String) -> URL? {
        guard let entry = entry(for: id) else { return nil }
        let url = videoURL(for: entry)
        return PathSafety.contained(url, in: videosDir) && FileManager.default.fileExists(atPath: url.path) ? url : nil
    }
    @discardableResult
    func scan() -> Bool {
        let url = Self.documents.appendingPathComponent("catalog.json")
        guard FileManager.default.fileExists(atPath: url.path) else { return false }
        do {
            let config = try JSONDecoder().decode(LibraryConfig.self, from: Data(contentsOf: url)).validated()
            lock.withLock { $0 = config }
            return true
        } catch {
            extensionLog("[Library] Keeping last valid configuration: \(error)")
            return false
        }
    }
    func generateThumbnail(for entry: MediaEntry) async -> URL? {
        let url = videosDir.appendingPathComponent(entry.id).appendingPathComponent("thumbnail.jpg")
        return FileManager.default.fileExists(atPath: url.path) ? url : nil
    }
}
