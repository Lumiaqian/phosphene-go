import AppKit
import Foundation

/// A stable tile: MyGO owns the list and timing, so edits never rewrite the system's selection.
func buildSettingsViewModelsXPC() async -> AnyObject? {
    let bundleID = "local.lumiaqian.phosphenego.extension"
    let library = VideoLibrary.shared
    library.scan()
    let first = library.config.playlist.videoIDs.compactMap { library.entry(for: $0) }.first
    let thumbnail = if let first {
        await library.generateThumbnail(for: first)
    } else {
        placeholderThumbnailURL()
    }
    guard let thumbnail else { return makeEmptyGroupsResponse() }
    let menu = ContextMenu(items: [ContextMenuItem(id: ContextMenuItemID(id: "manage-library"), localizedTitle: "打开 Phosphene Go", isDestructive: false)])
    let choiceID = ChoiceID(id: playlistChoiceID, descriptor: ChoiceIDDescriptor(
        provider: ChoiceProviderID(rawValue: bundleID), identifier: playlistChoiceID,
        files: [], configuration: Data(playlistChoiceID.utf8),
    ))
    let item = SettingsItem(
        id: choiceID,
        localizedName: "Phosphene Go 轮播",
        thumbnail: .image(url: thumbnail),
        choice: ChoiceDescriptor(
            id: choiceID,
            provider: ChoiceProviderID(rawValue: bundleID),
            identifier: playlistChoiceID,
            name: "Phosphene Go 轮播",
            localizedDescription: "各屏独立续播 · 在 Phosphene Go 中编辑列表",
            thumbnail: .image(url: thumbnail),
            isDownloaded: true,
            options: [],
        ),
        contentBadge: .video,
        showInTopLevel: true,
        sortOrder: 0,
        disposability: .none,
        contextMenu: menu,
    )
    let group = SettingsGroup(
        id: GroupID(id: "phosphenego-video-wallpapers"),
        items: [item],
        localizedName: "Phosphene Go",
        disposability: .none,
        sortOrder: -100,
        sortID: GroupSortID(id: "com.apple.wallpaper.aerials"),
        allChoiceID: nil,
        shouldHideItemLabels: false,
        contextMenu: menu,
        thumbnail: nil,
    )
    let model = SettingsViewModel(groups: [group], refreshPolicy: .default, isModificationDisabled: false)
    return remapToRealXPC(SettingsViewModels(desktop: model, screenSaver: model))
}

/// The placeholder tile's thumbnail: the same gradient the renderer falls back to
/// for a choice with no video file, plus a centered plus badge. Drawn once into
/// the container Documents directory and reused.
private func placeholderThumbnailURL() -> URL? {
    let url = FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent("Documents/add-video-tile.png")
    if FileManager.default.fileExists(atPath: url.path) {
        return url
    }

    let size = NSSize(width: 480, height: 270)
    let image = NSImage(size: size, flipped: false) { rect in
        let gradient = NSGradient(colors: [
            NSColor(red: 0.2, green: 0.0, blue: 0.5, alpha: 1.0),
            NSColor(red: 0.0, green: 0.3, blue: 0.7, alpha: 1.0),
            NSColor(red: 0.0, green: 0.6, blue: 0.4, alpha: 1.0),
        ])
        gradient?.draw(in: rect, angle: 45)

        let symbolConfiguration = NSImage.SymbolConfiguration(pointSize: 72, weight: .regular)
            .applying(.init(paletteColors: [.white]))
        if let plus = NSImage(systemSymbolName: "plus.circle.fill", accessibilityDescription: "Add")?
            .withSymbolConfiguration(symbolConfiguration) {
            let plusSize = NSSize(width: 80, height: 80)
            let origin = NSPoint(x: rect.midX - plusSize.width / 2, y: rect.midY - plusSize.height / 2)
            plus.draw(in: NSRect(origin: origin, size: plusSize))
        }
        return true
    }

    guard let tiff = image.tiffRepresentation,
          let rep = NSBitmapImageRep(data: tiff),
          let png = rep.representation(using: .png, properties: [:])
    else { return nil }
    do {
        try png.write(to: url)
        return url
    } catch {
        extensionLog("  [Settings] Placeholder thumbnail write failed: \(error)")
        return nil
    }
}

/// Fallback: create a WallpaperSettingsViewModelsXPC with empty groups.
func makeEmptyGroupsResponse() -> AnyObject? {
    let emptyViewModels = SettingsViewModels(
        desktop: SettingsViewModel(
            groups: [],
            refreshPolicy: .default,
            isModificationDisabled: false,
        ),
        screenSaver: nil,
    )
    return remapToRealXPC(emptyViewModels)
}

/// Archive via ShimViewModelsXPC, remap class name on unarchive to the real XPC type.
///
/// Secure coding cannot be required here: the whole point is to archive our own
/// `ShimViewModelsXPC` and decode it back as the private `WallpaperSettingsViewModelsXPC`
/// via `setClass(_:forClassName:)`, a substitution secure coding is designed to
/// forbid. This is safe because the archive is never persisted or received over
/// any boundary — it is produced and consumed in-process within this one function
/// from values we just constructed, so there is no untrusted input to defend
/// against. The decoded object is handed straight back to WallpaperAgent.
private func remapToRealXPC(_ viewModels: SettingsViewModels) -> AnyObject? {
    let shimXPC = ShimViewModelsXPC(value: viewModels)

    let data: Data
    do {
        data = try NSKeyedArchiver.archivedData(withRootObject: shimXPC, requiringSecureCoding: false)
    } catch {
        extensionLog("  [Remap] Archive failed: \(error)")
        return nil
    }

    guard let realClass = objc_getClass("WallpaperSettingsViewModelsXPC") as? AnyClass else {
        extensionLog("  [Remap] WallpaperSettingsViewModelsXPC class not found")
        return nil
    }

    guard let unarchiver = try? NSKeyedUnarchiver(forReadingFrom: data) else {
        extensionLog("  [Remap] Failed to create unarchiver")
        return nil
    }
    unarchiver.requiresSecureCoding = false
    unarchiver.decodingFailurePolicy = .setErrorAndReturn
    unarchiver.setClass(realClass, forClassName: "ShimViewModelsXPC")

    let result = unarchiver.decodeObject(forKey: NSKeyedArchiveRootObjectKey)
    if let error = unarchiver.error {
        extensionLog("  [Remap] Unarchive error: \(error)")
    }
    unarchiver.finishDecoding()

    if result == nil {
        extensionLog("  [Remap] Decoded result is nil")
    }
    return result as AnyObject?
}
