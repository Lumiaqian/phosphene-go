# Phosphene Go

本地视频动态壁纸应用。使用 **MyGO v0.9.0 的 Go 原生界面**管理资料库，由独立的 **Swift 墙纸扩展**播放。基于 [Phosphene v1.11.4](https://github.com/kageroumado/phosphene/tree/v1.11.4)，保留原 MIT 版权声明。

面向 Apple Silicon、macOS 26+ 的个人使用。当前实机环境为 macOS 27.0.1；其他系统版本的验证边界见 [验收记录](docs/VALIDATION.md)。

## 使用

1. 打开 `Phosphene Go.app`，在资料库中批量导入 MP4、MOV 或 M4V。应用会复制原片、验证解码、读取真实尺寸并生成缩略图。
2. 点击“打开系统墙纸设置”，选中 **Phosphene Go 轮播**，应用到需要的显示器。这个入口固定不变，后续直接在管理端编辑列表。
3. 在“播放列表与显示器”中排序，选择“播完一遍切换”，或输入正整数秒后应用指定时长。指定时长模式默认 60 秒，短片循环、长片到时切换。
4. 各屏首次按整个列表周期均匀错开，随后独立续播。显示器插拔不重置其他屏幕，设置预览也不推进桌面进度。

壁纸始终静音，保留导入视频的画质。关闭管理窗口后保留菜单栏，完全退出管理端后壁纸继续由系统扩展播放。设置页可启用登录启动。

“接电锁屏时保持亮屏”默认开启，仅在本墙纸实际启用、接电且锁屏时持有电源请求；解锁、拔电、停用墙纸或关闭选项后释放。系统密码要求不受影响。

## 构建和安装

已验证工具链：Go 1.27.3、Xcode 27.0、MyGO v0.9.0。先安装 Xcode 命令行工具并接受 Xcode 的首次启动配置。

```sh
git clone https://github.com/Lumiaqian/phosphene-go.git
cd phosphene-go
git switch feature/mygo-local-wallpaper
scripts/test.sh
scripts/build-local.sh
scripts/install-local.sh
open "$HOME/Applications/Phosphene Go.app"
```

`build-local.sh` 生成 `dist/darwin-arm64/Phosphene Go.app`，将扩展嵌入 `Contents/Extensions`，从内到外执行本地 ad-hoc 签名并校验。无需本机 Apple 开发证书。`install-local.sh` 会先测试和构建，成功后才更新 `~/Applications/Phosphene Go.app`。

首次安装通过系统设置选中轮播入口。开发中更新一个正在运行的扩展后，若系统仍保留旧进程，可重新选择入口，或退出管理端后运行 `killall -TERM WallpaperAgent` 让系统重建墙纸服务。后者会短暂重建所有显示器的墙纸。

首次导入多段现有素材，也可以在管理端完全退出后运行：

```sh
open -n -W "$HOME/Applications/Phosphene Go.app" --args --import "/absolute/path/video.mp4" "/absolute/path/second.mov"
```

结果写入资料库的 `import-result.json`。视频不会提交到 Git 仓库。

## 工程与数据

- `main.go`、`views.go`：MyGO 应用、菜单栏、资料库与设置界面。
- `internal/catalog`：Go 管理的资料库、导入、原子配置发布与删除清理。
- `internal/platform`：Swift 原生媒体探测、缩略图、稳定显示器标识与变更通知。桥接代码由 MyGO 生成。
- `PhospheneExtension`：系统接入、原生解码、独立轮播、锁屏亮屏及尺寸恢复。
- [跨语言契约与播放状态](docs/ARCHITECTURE.md)、[使用及回退](docs/USAGE.md)、[实机验收记录](docs/VALIDATION.md)。

新身份为 `local.lumiaqian.phosphenego` 和 `local.lumiaqian.phosphenego.extension`。数据位于：

```text
~/Library/Containers/local.lumiaqian.phosphenego.extension/Data/Documents/
  catalog.json       # Go 写配置与资料库
  runtime.json       # 扩展写各屏进度，每 5 秒保存，切换/休眠/断开时另存
  videos/<UUID>/     # 原片和 thumbnail.jpg
  extension.log      # 有界运行日志
```

没有 Steam、创意工坊、场景转换、旧管理端或应用层暂停策略。只在系统休眠、显示器断开或系统不展示某个表面时挂起资源，并在恢复展示后续播。

## 检查

```sh
scripts/test.sh                         # Go race tests、Swift 轮播/状态/尺寸测试、JSON 双向互读、go vet
go tool mygo native check internal/platform
swiftformat --lint .
swiftlint lint PhospheneExtension internal/platform/swift/Sources/Platform/Platform.swift tests/main.swift
```

SwiftFormat / SwiftLint 使用 Homebrew 的独立工具。生成的桥接文件和构建目录不参与格式检查。测试不会修改正在使用的墙纸资料库。

## 来源和许可

基线提交：`cee21cef52380505d3593872ec53c1b56dc54e6e`，对应 Phosphene `v1.11.4`。

原生系统接入和渲染实现来自 [kageroumado/phosphene](https://github.com/kageroumado/phosphene)。[MyGO](https://github.com/egoist/mygo) 固定为 `v0.9.0`。[LICENSE](LICENSE) 保留原作者的 MIT 声明，并随本地应用打包。上游私有接口研究保留在 `docs/upstream/` 中，当前产品状态以本仓库文档为准。
