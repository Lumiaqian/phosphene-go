package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) view(c *ui.Context) {
	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(180).FillHeight().Padding(24, 16).Gap(12).Children(func() {
			ui.Text(c, "Phosphene Go").FontSize(20).Bold()
			ui.Text(c, "本地视频壁纸").FontSize(12)
			ui.Divider(c)
			for i, label := range []string{"资料库", "播放列表与显示器", "设置"} {
				ui.Button(c.Key(label), label).FillWidth().OnClick(func() { a.page = i })
			}
			ui.Spacer(c).Grow(1)
			ui.Text(c, "0.1.0 · 个人版").FontSize(12)
		})
		ui.Column(c).Grow(1).FillHeight().Padding(28).Gap(16).Children(func() {
			switch a.page {
			case 0:
				a.libraryView(c)
			case 1:
				a.playlistView(c)
			case 2:
				a.settingsView(c)
			}
			if a.status != "" {
				ui.Text(c, a.status).FontSize(12)
			}
		})
	})
}
func (a *application) libraryView(c *ui.Context) {
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "资料库").FontSize(26).Bold().Grow(1)
		ui.PrimaryButton(c, "导入视频…").Disabled(a.busy).OnClick(a.chooseImport)
	})
	ui.Text(c, fmt.Sprintf("%d 个原画质视频 · 壁纸静音播放", len(a.config.Media)))
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).Gap(12).FillWidth().Children(func() {
			for _, m := range a.config.Media {
				ui.Row(c.Key(m.ID)).Gap(16).Padding(12).FillWidth().Children(func() {
					if thumb := a.thumbs[m.ID]; thumb != nil {
						ui.Image(c, thumb).Size(176, 99).Fit(ui.Cover).Radius(6)
					}
					ui.Column(c).Gap(8).Grow(1).Children(func() {
						ui.Text(c, m.Name).Bold()
						ui.Text(c, fmt.Sprintf("%d × %d   ·   %.2f 秒   ·   %.2f fps", m.Width, m.Height, m.DurationSeconds, m.FrameRate)).FontSize(12)
						ui.Row(c).Gap(8).Children(func() {
							ui.Button(c, "预览").OnClick(func() { a.previewMedia(m) })
							ui.Button(c, "重命名").Disabled(a.busy).OnClick(func() { a.selected = m.ID; a.rename = m.Name })
							inList := slices.Contains(a.config.Playlist.VideoIDs, m.ID)
							label := "加入轮播"
							if inList {
								label = "移出轮播"
							}
							ui.Button(c, label).Disabled(a.busy).OnClick(func() {
								if inList {
									a.config.Playlist.VideoIDs = slices.DeleteFunc(a.config.Playlist.VideoIDs, func(id string) bool { return id == m.ID })
								} else {
									a.config.Playlist.VideoIDs = append(a.config.Playlist.VideoIDs, m.ID)
								}
								a.save()
							})
							ui.Button(c, "删除").Disabled(a.busy).OnClick(func() { a.deleteMedia(m) })
						})
						if a.selected == m.ID {
							ui.Row(c).Gap(8).Children(func() {
								ui.TextInput(c, &a.rename).Grow(1).Label("视频名称")
								ui.Button(c, "保存名称").Disabled(a.busy || strings.TrimSpace(a.rename) == "").OnClick(func() {
									for i := range a.config.Media {
										if a.config.Media[i].ID == m.ID {
											a.config.Media[i].Name = strings.TrimSpace(a.rename)
										}
									}
									a.selected = ""
									a.save()
								})
							})
						}
					})
				})
				ui.Divider(c)
			}
			if len(a.config.Media) == 0 {
				ui.Text(c, "导入 MP4、MOV 或 M4V，开始建立你的壁纸列表。")
			}
		})
	})
}
func (a *application) playlistView(c *ui.Context) {
	ui.Text(c, "播放列表与显示器").FontSize(26).Bold()
	ui.Text(c, "首次均匀错开，之后每块屏独立续播。")
	ui.Row(c).Gap(12).Children(func() {
		if ui.Radio(c, &a.config.Playlist.AdvanceMode, "end", "播完一遍切换").Disabled(a.busy).Changed() {
			a.save()
		}
		if ui.Radio(c, &a.config.Playlist.AdvanceMode, "duration", "按指定时长切换").Disabled(a.busy).Changed() {
			a.save()
		}
	})
	if a.config.Playlist.AdvanceMode == "duration" {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "每项播放")
			ui.TextInput(c, &a.interval).Width(95).Label("轮播秒数")
			ui.Text(c, "秒，短片自动循环")
			ui.Button(c, "应用").Disabled(a.busy).OnClick(func() {
				n, e := strconv.Atoi(strings.TrimSpace(a.interval))
				if e != nil || n < 1 {
					a.status = "请输入正整数秒"
					return
				}
				a.config.Playlist.IntervalSeconds = n
				a.save()
			})
		})
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			for i, id := range a.config.Playlist.VideoIDs {
				m, ok := a.entry(id)
				if !ok {
					continue
				}
				ui.Row(c.Key(id)).Gap(8).FillWidth().Children(func() {
					ui.Text(c, fmt.Sprintf("%02d   %s", i+1, m.Name)).Grow(1)
					ui.Button(c, "上移").Disabled(a.busy || i == 0).OnClick(func() {
						a.config.Playlist.VideoIDs[i-1], a.config.Playlist.VideoIDs[i] = a.config.Playlist.VideoIDs[i], a.config.Playlist.VideoIDs[i-1]
						a.save()
					})
					ui.Button(c, "下移").Disabled(a.busy || i == len(a.config.Playlist.VideoIDs)-1).OnClick(func() {
						a.config.Playlist.VideoIDs[i+1], a.config.Playlist.VideoIDs[i] = a.config.Playlist.VideoIDs[i], a.config.Playlist.VideoIDs[i+1]
						a.save()
					})
				})
			}
			if len(a.config.Playlist.VideoIDs) == 0 {
				ui.Text(c, "列表为空，壁纸会保留末帧。请从资料库加入视频。")
			}
			ui.Divider(c)
			ui.Text(c, "当前显示器").FontSize(18).Bold()
			fresh := time.Since(time.Unix(int64(a.runtime.UpdatedAtUnix), 0)) < 15*time.Second
			for _, d := range a.displays {
				label := "等待系统启用本壁纸"
				if fresh {
					for _, p := range a.runtime.Displays {
						if p.DisplayKey == d.Identifier {
							m, _ := a.entry(p.VideoID)
							label = fmt.Sprintf("%s · %.1f 秒", m.Name, p.PositionSeconds)
							if !slices.Contains(a.runtime.ActiveDisplayKeys, d.Identifier) {
								label += " · 等待系统恢复展示"
							}
						}
					}
				}
				ui.Text(c, d.Name+"  —  "+label)
			}
			for _, e := range a.runtime.Errors {
				if fresh {
					ui.Text(c, e).FontSize(12)
				}
			}
		})
	})
	ui.Button(c, "打开系统墙纸设置").OnClick(openWallpaperSettings)
}
func (a *application) settingsView(c *ui.Context) {
	ui.Text(c, "设置").FontSize(26).Bold()
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "接电锁屏时保持亮屏").Grow(1)
		if ui.Switch(c, &a.config.Playlist.KeepAwakeWhenLockedOnAC).Label("接电锁屏保持亮屏").Disabled(a.busy).Changed() {
			a.save()
		}
	})
	ui.Text(c, "解锁、拔掉电源或停用本墙纸时自动释放。系统密码要求保持不变。").FontSize(12)
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "登录时启动管理端").Grow(1)
		if ui.Switch(c, &a.login).Label("登录启动").Changed() {
			if e := mygo.App.SetOpenAtLogin(a.login); e != nil {
				a.status = e.Error()
				a.login = mygo.App.OpenAtLogin()
			}
		}
	})
	ui.Divider(c)
	ui.Text(c, "首次启用").FontSize(18).Bold()
	ui.Text(c, "在系统墙纸设置中选中「Phosphene Go 轮播」，应用到所需的显示器。此后在这里编辑即可。")
	ui.Button(c, "打开系统墙纸设置").OnClick(openWallpaperSettings)
	ui.Text(c, "关闭管理窗口后保留菜单栏。完全退出管理端后，壁纸仍由系统扩展播放。")
	ui.Button(c, "在 Finder 中打开资料库").OnClick(func() { _ = mygo.Shell.OpenPath(a.root) })
	ui.Spacer(c).Grow(1)
	ui.Text(c, "基于 Phosphene v1.11.4 · MyGO v0.9.0 · MIT License").FontSize(12)
}
