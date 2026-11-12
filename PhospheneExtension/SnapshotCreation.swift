import AVFoundation
import CoreMedia
@preconcurrency import IOSurface

/// Create a WallpaperSnapshotXPC containing an IOSurface with a video frame.
///
/// The caller supplies the surface's actual asset and clock; displays never borrow
/// one another's snapshot. Loop offsets are normalized back into the video.
func createSnapshotViaRuntime(videoURL: URL, currentTime: CMTime? = nil) async -> AnyObject? {
    let asset = AVURLAsset(url: videoURL)
    let generator = AVAssetImageGenerator(asset: asset)
    generator.appliesPreferredTrackTransform = true
    let duration = await (try? asset.load(.duration).seconds) ?? 0
    let seconds = currentTime?.seconds ?? 0
    let position = duration > 0 && seconds.isFinite ? max(0, seconds).truncatingRemainder(dividingBy: duration) : 0
    let requestTime = CMTime(seconds: position, preferredTimescale: 60_000)

    let image: CGImage
    do {
        let result = try await generator.image(at: requestTime)
        image = result.image
    } catch {
        extensionLog("  [Snapshot] Failed to get video frame: \(error)")
        return nil
    }

    guard let snapshotXPC = renderSnapshotToIOSurface(image: image) else {
        return nil
    }

    traceLog("  [Snapshot] Created WallpaperSnapshotXPC \(image.width)x\(image.height)")
    return snapshotXPC
}

/// Render a CGImage to an IOSurface and wrap it in a WallpaperSnapshotXPC.
/// Synchronous to avoid async Sendable constraints on IOSurface's `[Key: Any]` dictionary.
private func renderSnapshotToIOSurface(image: CGImage) -> AnyObject? {
    let width = image.width
    let height = image.height

    let surfaceProps: [IOSurfacePropertyKey: any Sendable] = [
        .width: width,
        .height: height,
        .bytesPerElement: 4,
        .pixelFormat: 0x4247_5241, // 'BGRA'
    ]
    guard let surface = IOSurface(properties: surfaceProps) else {
        extensionLog("  [Snapshot] Failed to create IOSurface")
        return nil
    }

    surface.lock(options: [], seed: nil)
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    if let ctx = CGContext(
        data: surface.baseAddress,
        width: width,
        height: height,
        bitsPerComponent: 8,
        bytesPerRow: surface.bytesPerRow,
        space: colorSpace,
        bitmapInfo: CGImageAlphaInfo.premultipliedFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue,
    ) {
        ctx.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
    } else {
        extensionLog("  [Snapshot] Failed to create CGContext for IOSurface")
        surface.unlock(options: [], seed: nil)
        return nil
    }
    surface.unlock(options: [], seed: nil)

    return createSnapshotXPC(surface: surface)
}
