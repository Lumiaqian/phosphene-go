import AppKit
import AVFoundation
import CoreGraphics
import Foundation
import ImageIO
import os
import UniformTypeIdentifiers

func makeHost() throws -> any Host {
    NativeHost()
}

/// Calls for help the extension posted (SpiralRecovery.agentStuckNotification).
private let stuckSignalCount = OSAllocatedUnfairLock(initialState: Int64(0))

final class NativeHost: Host {
    init() {
        // Darwin notifications carry no context; the observer only counts them.
        CFNotificationCenterAddObserver(
            CFNotificationCenterGetDarwinNotifyCenter(), nil,
            { _, _, _, _, _ in stuckSignalCount.withLock { $0 += 1 } },
            "local.lumiaqian.phosphenego.agentStuck" as CFString, nil, .deliverImmediately,
        )
    }

    func agentStuckSignals() throws -> Int64 {
        stuckSignalCount.withLock { $0 }
    }

    func probe(path: String, thumbnailPath: String) async throws -> MediaInfo {
        let asset = AVURLAsset(url: URL(fileURLWithPath: path))
        guard let track = try await asset.loadTracks(withMediaType: .video).first else {
            throw NSError(domain: "PhospheneGo", code: 1, userInfo: [NSLocalizedDescriptionKey: "文件没有视频轨道"])
        }
        let size = try await track.load(.naturalSize)
        let transform = try await track.load(.preferredTransform)
        let frame = CGRect(origin: .zero, size: size).applying(transform)
        let fps = try await track.load(.nominalFrameRate)
        let duration = try await track.load(.timeRange).duration.seconds
        guard duration.isFinite, duration > 0, frame.width > 0, frame.height > 0 else {
            throw NSError(domain: "PhospheneGo", code: 2, userInfo: [NSLocalizedDescriptionKey: "视频时长或尺寸无效"])
        }
        let generator = AVAssetImageGenerator(asset: asset)
        generator.appliesPreferredTrackTransform = true
        generator.maximumSize = CGSize(width: 640, height: 360)
        let image = try await generator.image(at: .zero).image
        guard let destination = CGImageDestinationCreateWithURL(
            URL(fileURLWithPath: thumbnailPath) as CFURL, UTType.jpeg.identifier as CFString, 1, nil,
        ) else {
            throw NSError(domain: "PhospheneGo", code: 3, userInfo: [NSLocalizedDescriptionKey: "无法写入视频缩略图"])
        }
        CGImageDestinationAddImage(destination, image, nil)
        guard CGImageDestinationFinalize(destination) else {
            throw NSError(domain: "PhospheneGo", code: 4, userInfo: [NSLocalizedDescriptionKey: "保存视频缩略图失败"])
        }
        return MediaInfo(width: Int64(abs(frame.width)), height: Int64(abs(frame.height)), frameRate: Double(fps), duration: duration)
    }

    @MainActor
    func displays() throws -> [DisplayInfo] {
        NSScreen.screens.compactMap { screen in
            guard let number = screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber,
                  let identifier = CGDisplayCreateUUIDFromDisplayID(number.uint32Value)?.takeRetainedValue() else { return nil }
            return DisplayInfo(
                identifier: CFUUIDCreateString(nil, identifier) as String,
                name: screen.localizedName,
                runtimeNumber: Int64(number.uint32Value),
                builtIn: CGDisplayIsBuiltin(number.uint32Value) != 0,
            )
        }
    }

    func notifyLibraryChanged() throws {
        var buffer = [CChar](repeating: 0, count: Int(PATH_MAX))
        let length = confstr(_CS_DARWIN_USER_CACHE_DIR, &buffer, buffer.count)
        if length > 0 {
            let root = URL(fileURLWithPath: String(cString: buffer))
                .appendingPathComponent("com.apple.wallpaper.agent/com.apple.wallpaper.view-model-cache")
            for surface in ["desktop", "screenSaver"] {
                let file = root.appendingPathComponent("extension-local.lumiaqian.phosphenego.extension-\(surface)")
                if FileManager.default.fileExists(atPath: file.path) {
                    try FileManager.default.removeItem(at: file)
                }
            }
        }
        CFNotificationCenterPostNotification(
            CFNotificationCenterGetDarwinNotifyCenter(),
            CFNotificationName("local.lumiaqian.phosphenego.libraryChanged" as CFString), nil, nil, true,
        )
    }
}
