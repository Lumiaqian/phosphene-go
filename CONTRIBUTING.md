# 开发说明

首版基于 Phosphene v1.11.4，使用 MyGO v0.9.0 管理端和 Swift 扩展。开发入口与工具链见 README。

修改播放、获取表面或 teardown 前，先读 `docs/ARCHITECTURE.md`；私有系统接入还应参照 `REFERENCE.md` 和 `docs/UPSTREAM-PLAYBACK-STATE.md`。每个远程表面必须使用独立 CAContext；同一显示器的持久进度只由播放控制器管理。

Go 写配置，扩展写运行进度。不能把读取器 EOF 当成展示结束，也不能用进程运行或日志“播放”替代真实画面的验收。UI 预览不推进桌面会话，运行时显示器数字编号不持久化为身份。

## 构建与检查

```sh
scripts/test.sh
go tool mygo native check modules/platform
swiftformat --lint .
swiftlint lint PhospheneExtension modules/platform/swift/Sources/Platform/Platform.swift tests/main.swift
scripts/build-local.sh
```

`Phosphene.xcodeproj` 只构建扩展，主程序由 MyGO 构建。先签扩展，再签宿主；不得同时保留多个已注册的同 Bundle ID 构建副本。脚本会注销 `dist` 中的副本，只注册安装目录。

原生模块规则位于 `modules/platform/AGENTS.md`。修改接口后通过 `go tool mygo native check modules/platform` 生成桥接，不能手改 `platform_native.go` 或 `Platform.mygo.swift`。这些生成文件和构建产品不参加格式检查。

## 本机验收

真实观察两屏 4K、循环/切换、锁屏、主程序退出、显示器插拔、分辨率变化和睡眠唤醒。接电锁屏亮屏须在旧助手退出后单独测，确认新进程确实持有并释放电源请求。未具备的硬件和未执行的长测在 `docs/VALIDATION.md` 明确标记。

视频、私有配置、运行日志、原始墙纸索引与构建产物不要提交到 Git。
