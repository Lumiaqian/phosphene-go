# 契约与播放状态

## 职责

Go 是资料库和配置的唯一产品写入方，Swift 扩展是进度的唯一写入方。实际壁纸视频通过 AVAssetReader → AVSampleBufferDisplayLayer → 远程 CAContext 播放，帧数据不经过 Go。管理端自己的预览使用 MyGO 的系统视频播放器，与桌面会话分开。

原 Phosphene 的 Steam/Workshop、SwiftUI 管理端、视频变体生成、全屏/遮挡/游戏/电量暂停均已移除。保留 Apple 调用者验证、独立表面上下文、原生解码、缓存快照、屏幕尺寸恢复与系统生命周期处理。

## JSON v1

所有播放时间单位为秒，日期为 Unix 秒。数组用 `[]` 表示空值，不能用 `null` 替代必填数组。外层 `schemaVersion` 为 1，不支持的版本不覆盖当前有效配置。

`catalog.json` 的字段：

| 字段 | 含义 |
| --- | --- |
| schemaVersion / revision | 契约版本、配置修订号 |
| media | `MediaEntry` 数组 |
| playlist | `PlaylistSpec` |
| removedMediaIDs | Go 内部的显式删除记录，可省略；扩展忽略此字段 |

`MediaEntry` 保存 `id`、`name`、`filename`、`width`、`height`、`durationSeconds`、`frameRate`、`addedAtUnix`。ID 为 UUID，文件名必须是安全的单个路径组件。原片在 `videos/<id>/<filename>`，缩略图为同目录 `thumbnail.jpg`。

`PlaylistSpec` 保存 `videoIDs`、`advanceMode`（`end` 或 `duration`）、`intervalSeconds`（正整数）和 `keepAwakeWhenLockedOnAC`。

`runtime.json` 保存 `schemaVersion`、`appliedRevision`、`updatedAtUnix`、`ruleSignature`、`displays`、`activeDisplayKeys`、`openVideoIDs`、`keepAwakeActive`、`errors`。每个 `DisplayProgress` 保存 `displayKey`、`videoID`、`positionSeconds`、`itemElapsedSeconds`。

Go 通过同目录临时文件、同步并原子 rename 发布配置；扩展通过 `.atomic` 写运行进度。Darwin 通知 `local.lumiaqian.phosphenego.libraryChanged` 通知扩展重读，扩展无进程时由下次启动读取。管理端读取运行进度并标明陈旧状态。

## 导入与删除

导入复制到隐藏临时目录，原生验证视频轨道、真实尺寸、时长以及能否解码首帧，完成后再改名为 UUID 目录，发布配置后才进入列表。失败不改动原片，也不会向配置暴露半成品。

删除将素材从资料库索引和播放列表移除，并在同一份原子配置中记录其 ID。扩展先切到下一个可播放项目；只有其确认新配置、运行记录仍新鲜、且所有原生读者不再持有该 ID 后，管理端才清理资料库副本。清理只处理显式删除记录，绝不把尚未发布的批量导入文件当作垃圾。

列表为空时保留已经显示的末帧，并在管理端提示。如果末帧仍由旧原片支持，副本会暂留到播放器真正释放该素材。原始文件和原 Phosphene 的资料库均不参与清理。

## 每屏会话

持久身份为 `CGDisplayCreateUUIDFromDisplayID` 对应 UUID，显示器名称只用于界面，系统运行时数字 ID 只用于当前远程表面。初次出现的屏幕按系统当前屏幕顺序在完整播放周期中均匀错开；已保存的屏幕恢复自己的视频和片内位置。

桌面与锁屏的远程层各有自己的 CAContext，避免同一上下文被两个宿主抢占。它们共享同一显示器的逻辑进度。设置预览永远不建立或推进持久会话。表面交接时检查时间轴，必要时对齐本屏的会话。

轮播增量来自原生展示时钟，并限制在已排入展示层的时间范围；AVAssetReader 提前读到 EOF 只准备无缝循环，不触发换片。换素材、seek 或换表面会建立新的时钟基线，避免把时钟原点变化计入播放时长。

在 `end` 模式按视频剩余时长切换。在 `duration` 模式按本项累计展示时长计时，短片可循环多次。修改规则保留片内位置，重置本项指定时长计时。素材缺失、被删或解码失败时按原顺序跳到下一有效项。

## 电源与生命周期

系统的屏幕休眠、断开和 suspended 表面会停止推进其时钟；持续挂起会释放读者，恢复后重建。无窗口覆盖或全屏应用检测器，也没有用户暂停标记。

接电锁屏亮屏请求由扩展持有，使用 `PreventUserIdleDisplaySleep`。锁状态通过系统通知及 sessionagent 状态读取，电源状态由 IOKit 读取。解锁、拔电、选项关闭、墙纸停用和扩展退出都会释放。它不改变密码、登录窗口或系统认证配置。

## 系统边界

墙纸接入使用上游已验证的私有 WallpaperExtensionKit 接口。当前本机 ad-hoc 签名可以注册、加载并读写独立容器；这一结果只代表已实测的 macOS 版本和本机环境，不是 Developer ID 公证发行验证。公开发行不在首版范围。
