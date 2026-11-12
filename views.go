package main

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Lumiaqian/phosphene-go/internal/catalog"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const (
	titleBarHeight = 52
	headerHeight   = 72
	sidebarWidth   = 220
	sectionLibrary = "library"
	sectionPlay    = "playlist"
	sectionSetting = "settings"
)

func (a *application) view(c *ui.Context) {
	current = lookFor(a.prefs.Glass)
	t := a.prefs.theme(c.Theme(), c.Preferences())
	t.Radius = 7
	// Everything sits on glass: control faces, hovers, selections and lines
	// are tints of the text color, so what is under them shows through.
	t.Surface, t.SurfaceHover, t.SurfacePressed = t.Text.Alpha(0.06), t.Text.Alpha(0.08), t.Text.Alpha(0.12)
	t.Border = t.Text.Alpha(0.12)
	c.SetTheme(&t)
	if a.status != "" {
		c.Toast(a.status)
		a.status = ""
	}
	ui.Box(c).Fill().Clip().Children(func() {
		a.backdrop(c, current, 1)
		overlay(ui.Row(c)).AlignItems(ui.Stretch).Children(func() {
			// The sidebar floats as a pane of Liquid Glass over the backdrop.
			current.fillPane(c, ui.Column(c).Width(sidebarWidth).Shrink(0).Margin(8, 0, 8, 8).Radius(18)).
				Children(func() { a.sidebar(c) })
			ui.Box(c).Grow(1).MinWidth(0).Children(func() {
				switch a.section {
				case sectionPlay:
					a.playlistView(c)
				case sectionSetting:
					a.settingsView(c)
				default:
					a.libraryView(c)
				}
			})
		})
		glassToasts(c)
	})
}

// glassToasts draws the window's toasts as capsules of glass rising from
// the bottom of the content, after it so the glass shows it through.
func glassToasts(c *ui.Context) {
	t := c.Theme()
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(0, 0, 28, sidebarWidth+8).AlignItems(ui.Center).Gap(8)
		for _, tt := range toasts {
			toast := ui.ToastBase(c, tt)
			current.fillControl(c, toast.Root.Padding(10, 20).Radius(999), glass.Glass{}).
				Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 12}, Exit: &ui.Motion{Y: 12}})
			toast.Root.Children(func() {
				ui.Text(c, tt.Title).FontWeight(600).TextColor(t.Text)
			})
		}
	})
}

// backdrop fills its parent with the wallpaper now playing, blurred and
// toned as l says, for the glass to show through; scale shrinks the blur
// for a preview. Without glass it is the plain background.
func (a *application) backdrop(c *ui.Context, l *look, scale float32) {
	t := c.Theme()
	if !l.glass {
		overlay(ui.Box(c)).Background(solidBackground(t))
		return
	}
	if bm := a.thumbs[a.ambientID()]; bm != nil {
		// Overscan so the blur does not fade in at the edges.
		over := 2 * l.blur * scale
		ui.Image(c, bm).Absolute().Top(-over).Left(-over).Right(-over).Bottom(-over).Fit(ui.Cover).Blur(l.blur * scale).Cache()
	} else {
		overlay(ui.Box(c)).Gradient(t.Accent.Alpha(0.35), t.Background, 135)
	}
	overlay(ui.Box(c)).Background(pick(t, ui.RGBA(255, 255, 255, 1.1*l.tone), ui.RGBA(0, 0, 0, l.tone)))
}

// ambientID is the video the backdrop shows: the one on the first display
// that reports playback, else the first of the playlist or the library.
func (a *application) ambientID() string {
	for _, d := range a.displays {
		if s := a.displayState(d.Identifier); s.ok {
			return s.media.ID
		}
	}
	if len(a.config.Playlist.VideoIDs) > 0 {
		return a.config.Playlist.VideoIDs[0]
	}
	if len(a.config.Media) > 0 {
		return a.config.Media[0].ID
	}
	return ""
}

func (a *application) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()
	ui.Column(c).Padding(0, 16, 14, 18).Gap(2).Children(func() {
		ui.Text(c, "Phosphene Go").FontSize(17).Bold()
		ui.Text(c, "本地视频壁纸").FontSize(12).TextColor(t.TextMuted)
	})
	list := ui.Sidebar(c, &a.section, func() {
		ui.SidebarItem(c, sectionLibrary, iconLibrary, "资料库").Children(func() {
			ui.Badge(c, strconv.Itoa(len(a.config.Media)))
		})
		ui.SidebarItem(c, sectionPlay, iconPlaylist, "播放与显示器").Children(func() {
			ui.Badge(c, strconv.Itoa(len(a.config.Playlist.VideoIDs)))
		})
		ui.SidebarItem(c, sectionSetting, iconSettings, "设置")
	}).Grow(1).Label("分区")
	list.Background(ui.Transparent)
	// Live state of each display, always in view.
	ui.Column(c).Padding(12, 14, 16, 14).Gap(10).Children(func() {
		ui.Text(c, "正在播放").FontSize(11).Bold().TextColor(t.TextMuted)
		if len(a.displays) == 0 {
			ui.Text(c, "未检测到显示器").FontSize(12).TextColor(t.TextMuted)
		}
		if a.wallpaperStalled(time.Now()) {
			glassButton(c, iconAlert, "修复壁纸", true).OnClick(func() { a.repairWallpaper("正在重建墙纸服务…", time.Now()) })
		}
		for _, d := range a.displays {
			s := a.displayState(d.Identifier)
			ui.Row(c.Key("side-" + d.Identifier)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Shrink(0).Background(s.color(t))
				ui.Column(c).Gap(1).MinWidth(0).Grow(1).Children(func() {
					ui.Text(c, d.Name).FontSize(12).FontWeight(600).SingleLine()
					ui.Text(c, s.short()).FontSize(11).TextColor(t.TextMuted).SingleLine()
				})
			})
		}
	})
}

// pageFrame lays out a page whose content scrolls under a floating header:
// the content blurs progressively as it reaches the top, and the header's
// title and glass buttons stay on it.
func pageFrame(c *ui.Context, title, subtitle string, actions func(), body func(top float32)) {
	bar := c.TitleBar()
	ui.Box(c).Fill().Children(func() {
		body(headerHeight)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(headerHeight).PassThrough().
			Material(glass.Blur{Radius: 10, Mask: &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180, Start: 0.5, End: 1}})
		ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(headerHeight).AlignItems(ui.Center).Gap(10).
			Padding(0, bar.Right+20, 0, 24).DragWindow().Children(func() {
			ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
				current.onBackdrop(c, ui.Text(c, title).FontSize(20).Bold().SingleLine(), false)
				if subtitle != "" {
					current.onBackdrop(c, ui.Text(c, subtitle).FontSize(12).SingleLine(), true)
				}
			})
			if actions != nil {
				actions()
			}
		})
	})
}

// scrollBody is a page's scrolling column, starting under the header.
func scrollBody(c *ui.Context, top, maxWidth float32, fn func()) {
	overlay(ui.Scroll(c)).ScrollbarInsets(top, 0, 0, 0).Children(func() {
		col := ui.Column(c).FillWidth().Padding(top+14, 24, 28, 24).Gap(26)
		if maxWidth > 0 {
			col.MaxWidth(maxWidth)
		}
		col.Children(fn)
	})
}

func (a *application) libraryView(c *ui.Context) {
	inList := 0
	for _, m := range a.config.Media {
		if slices.Contains(a.config.Playlist.VideoIDs, m.ID) {
			inList++
		}
	}
	subtitle := fmt.Sprintf("%d 个视频 · %d 个在轮播中 · 原画质，静音播放", len(a.config.Media), inList)
	pageFrame(c, "资料库", subtitle, func() {
		label := "导入视频"
		if a.busy {
			label = "处理中…"
		}
		glassButton(c, iconImport, label, true).Disabled(a.busy).OnClick(a.chooseImport)
	}, func(top float32) {
		if len(a.config.Media) == 0 {
			emptyState(c, iconFilm, "还没有视频", "点右上角「导入视频」添加 MP4、MOV 或 M4V，原画质复制，不重新编码。")
			return
		}
		a.grid.Key = func(i int) any { return a.config.Media[i].ID }
		a.grid.Label = func(i int) string { return a.config.Media[i].Name }
		overlay(ui.GridView(c, &a.grid, len(a.config.Media), 220, 214, func(i int) {
			a.mediaCard(c, a.config.Media[i])
		})).ScrollbarInsets(top, 0, 0, 0).Gap(18).Padding(top+14, 24, 24, 24)
	})
}

func (a *application) mediaCard(c *ui.Context, m catalog.MediaEntry) {
	t := c.Theme()
	order := slices.Index(a.config.Playlist.VideoIDs, m.ID)
	ui.Column(c).Grow(1).Gap(8).Children(func() {
		// Thumbnail with the order in the playlist and the duration on it.
		thumb := ui.Box(c).Grow(1).MinHeight(0).Radius(10).Clip().Background(t.Surface).Cursor(ui.CursorPointer).
			Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12)).Label("预览 " + m.Name)
		if thumb.Clicked() {
			a.previewMedia(m)
		}
		hovered := thumb.Hovered()
		thumb.Children(func() {
			if bm := a.thumbs[m.ID]; bm != nil {
				ui.Image(c, bm).Fill().Fit(ui.Cover)
			} else {
				ui.Box(c).Fill().Center().TextColor(t.TextMuted).Children(func() { ui.Icon(c, iconFilm).FontSize(28) })
			}
			if hovered {
				overlay(ui.Box(c)).PassThrough().Center().Children(func() {
					current.fillControl(c, ui.Box(c).Size(48, 48).Radius(24).Center(), glass.Glass{}).TextColor(t.Text).Children(func() {
						ui.Icon(c, iconPlay).FontSize(18)
					})
				})
			}
			if order >= 0 {
				glassPill(c, fmt.Sprintf("#%d 轮播", order+1), glass.Glass{Tint: t.Accent}, t.AccentText).Absolute().Top(8).Left(8)
			}
			glassPill(c, clock(m.DurationSeconds), glass.Glass{}, t.Text).Absolute().Bottom(8).Right(8)
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if a.selected == m.ID {
				ui.TextInput(c, &a.rename).Grow(1).Radius(999).Label("视频名称").OnSubmit(func() { a.commitRename(m.ID) })
				iconButton(c, iconCheck, "保存名称").Disabled(a.busy || strings.TrimSpace(a.rename) == "").OnClick(func() { a.commitRename(m.ID) })
				iconButton(c, iconX, "取消").OnClick(func() { a.selected = "" })
				return
			}
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				current.onBackdrop(c, ui.Text(c, m.Name).FontSize(13).FontWeight(600).SingleLine(), false)
				current.onBackdrop(c, ui.Text(c, fmt.Sprintf("%s · %s fps", resolution(m.Width, m.Height), trimFloat(m.FrameRate))).FontSize(11).SingleLine(), true)
			})
			if order >= 0 {
				iconButton(c, iconCheck, "移出轮播").TextColor(t.Accent).Disabled(a.busy).OnClick(func() { a.togglePlaylist(m.ID) })
			} else {
				iconButton(c, iconPlus, "加入轮播").Disabled(a.busy).OnClick(func() { a.togglePlaylist(m.ID) })
			}
			iconButton(c, iconMore, "更多").Disabled(a.busy).Menu(func(menu *ui.Menu) {
				if menu.Item("预览").Chosen() {
					a.previewMedia(m)
				}
				if menu.Item("重命名…").Chosen() {
					a.selected, a.rename = m.ID, m.Name
				}
				menu.Separator()
				if menu.Item("从资料库删除…").Chosen() {
					a.deleteMedia(m)
				}
			})
		})
	})
}

func (a *application) commitRename(id string) {
	name := strings.TrimSpace(a.rename)
	if a.busy || name == "" {
		return
	}
	for i := range a.config.Media {
		if a.config.Media[i].ID == id {
			a.config.Media[i].Name = name
		}
	}
	a.selected = ""
	a.save()
}

func (a *application) togglePlaylist(id string) {
	if slices.Contains(a.config.Playlist.VideoIDs, id) {
		a.config.Playlist.VideoIDs = slices.DeleteFunc(a.config.Playlist.VideoIDs, func(v string) bool { return v == id })
	} else {
		a.config.Playlist.VideoIDs = append(a.config.Playlist.VideoIDs, id)
	}
	a.save()
}

func (a *application) playlistView(c *ui.Context) {
	t := c.Theme()
	pageFrame(c, "播放与显示器", "首次均匀错开，之后每块屏独立续播", func() {
		glassButton(c, iconExternal, "系统墙纸设置", false).OnClick(openWallpaperSettings)
	}, func(top float32) {
		scrollBody(c, top, 0, func() {
			a.repairBanner(c)
			// Displays.
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, "显示器")
				if len(a.displays) == 0 {
					ui.Text(c, "未检测到显示器").TextColor(t.TextMuted)
				}
				ui.Row(c).Wrap().Gap(14).FillWidth().Children(func() {
					for _, d := range a.displays {
						a.displayCard(c, d.Identifier, d.Name, d.BuiltIn)
					}
				})
				if a.runtimeFresh() {
					for _, e := range a.runtime.Errors {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 12).Radius(8).FillWidth().
							Background(t.Warning.Alpha(0.12)).TextColor(t.Warning).Children(func() {
							ui.Icon(c, iconAlert)
							ui.Text(c, e).FontSize(12).Grow(1)
						})
					}
				}
			})
			// Advance rule.
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, "切换方式")
				group(c, func() {
					groupRow(c, "切换时机", "", func() {
						mode := 0
						if a.config.Playlist.AdvanceMode == "duration" {
							mode = 1
						}
						if glassSegmented(c, &mode, "切换时机", a.busy, "播完一遍", "按时长") {
							a.config.Playlist.AdvanceMode = []string{"end", "duration"}[mode]
							a.save()
						}
					})
					if a.config.Playlist.AdvanceMode == "duration" {
						ui.Divider(c)
						groupRow(c, "每项播放时长", "短于该时长的视频会自动循环", func() {
							ui.TextInput(c, &a.interval).Width(72).Radius(999).Label("轮播秒数").OnSubmit(a.applyInterval)
							ui.Text(c, "秒").TextColor(t.TextMuted)
							glassButton(c, nil, "应用", false).Disabled(a.busy || strings.TrimSpace(a.interval) == strconv.Itoa(a.config.Playlist.IntervalSeconds)).OnClick(a.applyInterval)
						})
					}
				})
			})
			// Order.
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, fmt.Sprintf("播放顺序 · %d", len(a.config.Playlist.VideoIDs)))
				if len(a.config.Playlist.VideoIDs) == 0 {
					group(c, func() {
						ui.Column(c).Padding(22).Gap(4).Center().FillWidth().Children(func() {
							ui.Text(c, "列表为空").FontWeight(600)
							ui.Text(c, "壁纸会保留末帧。在资料库里点 + 加入视频。").FontSize(12).TextColor(t.TextMuted)
						})
					})
					return
				}
				group(c, func() {
					ids := a.config.Playlist.VideoIDs
					for i, id := range ids {
						m, ok := a.entry(id)
						if !ok {
							continue
						}
						if i > 0 {
							ui.Divider(c)
						}
						a.orderRow(c, i, len(ids), m)
					}
				})
			})
		})
	})
}

func (a *application) orderRow(c *ui.Context, i, n int, m catalog.MediaEntry) {
	t := c.Theme()
	row := ui.Row(c.Key(m.ID)).Gap(12).AlignItems(ui.Center).Padding(8, 12).FillWidth()
	hovered := row.Hovered()
	row.Children(func() {
		ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Width(20)
		thumbBox(c, a.thumbs[m.ID], 64, 36, 5)
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, m.Name).FontWeight(600).SingleLine()
			ui.Text(c, fmt.Sprintf("%s · %s", clock(m.DurationSeconds), resolution(m.Width, m.Height))).FontSize(11).TextColor(t.TextMuted)
		})
		ids := a.config.Playlist.VideoIDs
		controls := ui.Row(c).Gap(2).Opacity(0.55)
		if hovered {
			controls.Opacity(1)
		}
		controls.Children(func() {
			iconButton(c, iconUp, "上移").Disabled(a.busy || i == 0).OnClick(func() {
				ids[i-1], ids[i] = ids[i], ids[i-1]
				a.save()
			})
			iconButton(c, iconDown, "下移").Disabled(a.busy || i == n-1).OnClick(func() {
				ids[i+1], ids[i] = ids[i], ids[i+1]
				a.save()
			})
			iconButton(c, iconX, "移出轮播").Disabled(a.busy).OnClick(func() { a.togglePlaylist(m.ID) })
		})
	})
}

func (a *application) displayCard(c *ui.Context, key, name string, builtIn bool) {
	t := c.Theme()
	s := a.displayState(key)
	current.fillPane(c, ui.Row(c.Key("disp-"+key)).Gap(14).Padding(12).Radius(16).Width(340).Grow(1)).Children(func() {
		var bm *ui.Bitmap
		if s.ok {
			bm = a.thumbs[s.media.ID]
		}
		thumbBox(c, bm, 128, 72, 7)
		ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconMonitor).TextColor(t.TextMuted)
				ui.Text(c, name).FontWeight(600).SingleLine().Shrink(1)
				if builtIn {
					ui.Badge(c, "内建")
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Shrink(0).Background(s.color(t))
				ui.Text(c, s.title()).FontSize(12).SingleLine().Shrink(1)
			})
			if s.ok && s.media.DurationSeconds > 0 {
				ui.Progress(c, math.Min(1, s.position/s.media.DurationSeconds)).Label("播放进度")
				ui.Text(c, fmt.Sprintf("%s / %s", clock(s.position), clock(s.media.DurationSeconds))).FontSize(11).TextColor(t.TextMuted)
			} else {
				ui.Text(c, s.detail()).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func (a *application) applyInterval() {
	n, e := strconv.Atoi(strings.TrimSpace(a.interval))
	if e != nil || n < 1 {
		a.status = "请输入正整数秒"
		return
	}
	a.config.Playlist.IntervalSeconds = n
	a.save()
}

func (a *application) settingsView(c *ui.Context) {
	pageFrame(c, "设置", "", nil, func(top float32) {
		scrollBody(c, top, 720, func() {
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, "外观")
				group(c, func() {
					groupRow(c, "外观", "", func() {
						mode := max(0, slices.Index(appearances, a.prefs.Appearance))
						if glassSegmented(c, &mode, "外观", false, "跟随系统", "浅色", "深色") {
							a.prefs.Appearance = appearances[mode]
							a.savePrefs()
						}
					})
					ui.Divider(c)
					groupRow(c, "强调色", "按钮、开关、选中项和角标的颜色。", func() { a.accentSwatches(c) })
				})
			})
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				current.onBackdrop(c, ui.Row(c).Gap(6).AlignItems(ui.Center), true).Children(func() {
					ui.Icon(c, iconDrop).FontSize(14)
					sectionTitle(c, "液态玻璃")
				})
				group(c, func() {
					for i := range looks {
						if i > 0 {
							ui.Divider(c)
						}
						a.lookRow(c, &looks[i])
					}
				})
			})
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, "通用")
				group(c, func() {
					groupRow(c, "接电锁屏时保持亮屏", "解锁、拔掉电源或停用本墙纸时自动释放，不改变系统密码要求。", func() {
						if glassSwitch(c, &a.config.Playlist.KeepAwakeWhenLockedOnAC, "接电锁屏保持亮屏", a.busy) {
							a.save()
						}
					})
					ui.Divider(c)
					groupRow(c, "登录时启动管理端", "关闭窗口后保留菜单栏图标；完全退出后壁纸仍由系统扩展继续播放。管理端在运行时，才能在墙纸服务卡住时自动修复。", func() {
						if glassSwitch(c, &a.login, "登录启动", false) {
							if e := mygo.App.SetOpenAtLogin(a.login); e != nil {
								a.status = e.Error()
								a.login = mygo.App.OpenAtLogin()
							}
						}
					})
				})
			})
			ui.Column(c).Gap(10).FillWidth().Children(func() {
				sectionTitle(c, "启用与资料库")
				group(c, func() {
					groupRow(c, "系统墙纸", "首次使用：在系统墙纸设置中选中「Phosphene Go 轮播」并应用到需要的显示器。", func() {
						glassButton(c, iconExternal, "打开", false).OnClick(openWallpaperSettings)
					})
					ui.Divider(c)
					groupRow(c, "资料库位置", shortPath(a.root), func() {
						glassButton(c, iconFolder, "在 Finder 中显示", false).OnClick(func() { _ = mygo.Shell.OpenPath(a.root) })
					})
				})
			})
			current.onBackdrop(c, ui.Text(c, "Phosphene Go 0.1.0 · 基于 Phosphene v1.11.4 · MyGO v0.9.0 · MIT License").FontSize(11), true)
		})
	})
}

// accentSwatches lets the user pick the accent: a dot per color, the
// desktop's shown as a rainbow, the chosen one ringed.
func (a *application) accentSwatches(c *ui.Context) {
	t := c.Theme()
	for i, ac := range accents {
		chosen := ac.key == a.prefs.Accent || (a.prefs.Accent == "" && i == 0)
		dot := ui.ButtonBase(c.Key("accent-"+ac.key)).Size(22, 22).Radius(11).Label(ac.label).Tooltip(ac.label)
		if ac.color.A > 0 {
			dot.Background(ac.color)
		} else {
			dot.ConicGradient(ui.ConicGradient{Oklab: true, Stops: []ui.GradientStop{
				{Color: ui.Oklch(0.7, 0.2, 25), At: 0}, {Color: ui.Oklch(0.8, 0.17, 90), At: 0.25}, {Color: ui.Oklch(0.7, 0.17, 160), At: 0.5},
				{Color: ui.Oklch(0.6, 0.2, 270), At: 0.75}, {Color: ui.Oklch(0.7, 0.2, 25), At: 1},
			}})
		}
		if chosen {
			dot.Shadow(0, 0, 0, 2, t.Background).Shadow(0, 0, 0, 4, t.Text.Alpha(0.6))
		}
		if dot.Clicked() && !chosen {
			a.prefs.Accent = ac.key
			a.savePrefs()
		}
	}
}

// lookRow offers a look: a preview of it, its name and what it does, and a
// check on the one chosen.
func (a *application) lookRow(c *ui.Context, l *look) {
	t := c.Theme()
	row := ui.ButtonBase(c.Key("look-"+l.label)).FillWidth().Gap(14).Padding(10, 14).Label("液态玻璃：" + l.label)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		a.lookPreview(c, l)
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, l.label)
			ui.Text(c, l.about).FontSize(11).TextColor(t.TextMuted)
		})
		if l == current {
			ui.Icon(c, iconCheck).FontSize(16).TextColor(t.Accent)
		}
	})
	if row.Clicked() && l != current {
		a.prefs.Glass = l.key
		a.savePrefs()
	}
}

// lookPreview draws a tiny window in look l, over the wallpaper now
// playing: a sidebar, two panes and a button in the accent.
func (a *application) lookPreview(c *ui.Context, l *look) {
	t := c.Theme()
	ui.Box(c).Size(72, 46).Shrink(0).Radius(9).Clip().Border(1, t.Text.Alpha(0.12)).Children(func() {
		a.backdrop(c, l, 0.12)
		l.fillPane(c, ui.Box(c).Absolute().Top(3).Left(3).Bottom(3).Width(18).Radius(6))
		l.fillPane(c, ui.Box(c).Absolute().Top(9).Left(25).Right(4).Height(14).Radius(5))
		l.fillPane(c, ui.Box(c).Absolute().Top(26).Left(25).Right(4).Height(14).Radius(5))
		l.fillControl(c, ui.Box(c).Absolute().Top(14).Right(8).Size(12, 5).Radius(3), glass.Glass{Tint: t.Accent})
	})
}

// repairBanner offers to rebuild WallpaperAgent while the wallpaper looks
// stuck (recovery.go).
func (a *application) repairBanner(c *ui.Context) {
	if !a.wallpaperStalled(time.Now()) {
		return
	}
	t := c.Theme()
	current.fillControl(c, ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(12, 14).Radius(14).FillWidth(),
		glass.Glass{Tint: t.Warning.Alpha(0.3)}).Children(func() {
		ui.Icon(c, iconAlert).FontSize(18).TextColor(t.Warning)
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, "壁纸好像停了").FontWeight(600)
			ui.Text(c, "系统墙纸服务没有响应。修复会重建它，所有显示器的墙纸会闪一下；锁屏时出现属于正常，没在用本壁纸也可以忽略。").FontSize(11).TextColor(t.TextMuted)
		})
		glassButton(c, nil, "修复壁纸", true).OnClick(func() { a.repairWallpaper("正在重建墙纸服务…", time.Now()) })
	})
}

// --- display state ---

type displayState struct {
	ok       bool // a fresh report names a video for this display
	waiting  bool // reported but not shown by the system yet
	media    catalog.MediaEntry
	position float64
}

func (a *application) runtimeFresh() bool {
	return time.Since(time.Unix(int64(a.runtime.UpdatedAtUnix), 0)) < 15*time.Second
}

func (a *application) displayState(key string) displayState {
	var s displayState
	if !a.runtimeFresh() {
		return s
	}
	for _, p := range a.runtime.Displays {
		if p.DisplayKey == key {
			s.media, s.ok = a.entry(p.VideoID)
			s.position = p.PositionSeconds
			s.waiting = !slices.Contains(a.runtime.ActiveDisplayKeys, key)
		}
	}
	return s
}

func (s displayState) color(t *ui.Theme) ui.Color {
	switch {
	case s.ok && !s.waiting:
		return t.Success
	case s.ok:
		return t.Warning
	}
	return t.TextMuted
}

func (s displayState) title() string {
	if s.ok {
		return s.media.Name
	}
	return "未启用本壁纸"
}

func (s displayState) short() string {
	switch {
	case s.ok && s.waiting:
		return "等待系统恢复展示"
	case s.ok:
		return s.media.Name
	}
	return "未启用"
}

func (s displayState) detail() string {
	if s.ok && s.waiting {
		return "等待系统恢复展示"
	}
	return "在系统墙纸设置中选择「Phosphene Go 轮播」"
}

// --- formatting ---

// shortPath writes the home directory as ~, as Finder's path bar does.
func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func clock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := int(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func resolution(w, h int64) string {
	switch {
	case w >= 7680 || h >= 4320:
		return "8K"
	case w >= 3840 || h >= 2160:
		return "4K"
	case h >= 1440:
		return "1440p"
	case h >= 1080:
		return "1080p"
	}
	return fmt.Sprintf("%d×%d", w, h)
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64)
}
