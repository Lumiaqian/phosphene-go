package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Lumiaqian/phosphene-go/internal/catalog"
	"github.com/Lumiaqian/phosphene-go/internal/platform"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

type application struct {
	host          platform.Host
	root          string
	config        catalog.Config
	window        *mygo.Window
	page          int
	selected      string
	rename        string
	interval      string
	status        string
	busy          bool
	login         bool
	thumbs        map[string]*ui.Bitmap
	runtime       catalog.Runtime
	displays      []platform.DisplayInfo
	preview       *ui.VideoPlayer
	previewWindow *mygo.Window
}

func main() {
	mygo.App.SetName("Phosphene Go")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app := &application{root: catalog.Root(), thumbs: map[string]*ui.Bitmap{}}
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(visible bool) {
		if !visible && !(len(os.Args) > 2 && os.Args[1] == "--import") {
			app.open()
		}
	})
	mygo.App.OnSecondInstance(func(_ []string, _ string) { app.open() })
	mygo.App.OnOpenURL(func(_ string) { app.open() })
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu}}))
	mygo.App.WhenReady(func() {
		var err error
		app.host, err = platform.NewHost()
		if err != nil {
			mygo.Dialog.Error("无法加载原生模块", err.Error())
			mygo.App.Quit()
			return
		}
		app.config, err = catalog.Load(app.root)
		if err != nil {
			mygo.Dialog.Error("资料库读取失败", err.Error())
			mygo.App.Quit()
			return
		}
		app.interval = strconv.Itoa(app.config.Playlist.IntervalSeconds)
		app.login = mygo.App.OpenAtLogin()
		if len(os.Args) > 2 && os.Args[1] == "--import" {
			go func() {
				imported, errs := app.importFiles(os.Args[2:])
				if err := catalog.Save(app.root, &app.config); err != nil {
					errs = append(errs, err.Error())
				} else if err := app.host.NotifyLibraryChanged(); err != nil {
					errs = append(errs, err.Error())
				}
				_ = catalog.AtomicJSON(filepath.Join(app.root, "import-result.json"), map[string]any{"imported": imported, "errors": errs})
				mygo.App.Quit()
			}()
			return
		}
		app.loadThumbnails()
		app.reapLater()
		tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "Phosphene Go", Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开资料库", Click: func(*mygo.MenuItem, *mygo.Window) { app.open() }},
			{Label: "系统墙纸设置", Click: func(*mygo.MenuItem, *mygo.Window) { openWallpaperSettings() }},
			mygo.Separator(), {Label: "退出管理端（壁纸继续播放）", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})})
		if err != nil {
			log.Println(err)
		}
		_ = tray
		if !mygo.App.WasOpenedAtLogin() {
			app.open()
		}
		go func() {
			for range time.NewTicker(2 * time.Second).C {
				r, _ := catalog.ReadRuntime(app.root)
				displays, err := app.host.Displays()
				mygo.RunOnMain(func() {
					app.runtime = r
					if err == nil {
						app.displays = displays
					}
					if app.window != nil {
						app.window.Invalidate()
					}
				})
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
func (a *application) open() {
	if a.host == nil {
		return
	}
	if a.window != nil {
		a.window.Show()
		a.window.Focus()
		return
	}
	a.window = mygo.NewWindow(mygo.WindowOptions{Title: "Phosphene Go", Width: 1060, Height: 760, Content: ui.View(a.view)})
	a.window.OnClosed(func() { a.window = nil })
}
func openWallpaperSettings() {
	_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.Wallpaper-Settings.extension")
}
func (a *application) save() bool {
	if err := catalog.Save(a.root, &a.config); err != nil {
		a.status = "保存失败：" + err.Error()
		return false
	}
	if err := a.host.NotifyLibraryChanged(); err != nil {
		a.status = "已保存，通知扩展失败：" + err.Error()
		return false
	}
	a.status = "已保存，壁纸会自动更新"
	return true
}
func (a *application) loadThumbnails() {
	for _, m := range a.config.Media {
		if a.thumbs[m.ID] == nil {
			if b, e := os.ReadFile(catalog.ThumbnailPath(a.root, m)); e == nil {
				a.thumbs[m.ID], _ = ui.DecodeBitmap(b)
			}
		}
	}
}
func (a *application) importFiles(paths []string) (int, []string) {
	count := 0
	errs := []string{}
	for _, path := range paths {
		m, e := catalog.Import(context.Background(), a.root, path, func(ctx context.Context, path, thumb string) (catalog.MediaEntry, error) {
			info, e := a.host.Probe(ctx, path, thumb)
			return catalog.MediaEntry{Width: info.Width, Height: info.Height, DurationSeconds: info.Duration, FrameRate: info.FrameRate}, e
		})
		if e != nil {
			errs = append(errs, filepath.Base(path)+"："+e.Error())
			continue
		}
		a.config.Media = append(a.config.Media, m)
		a.config.Playlist.VideoIDs = append(a.config.Playlist.VideoIDs, m.ID)
		count++
	}
	return count, errs
}
func (a *application) chooseImport() {
	if a.busy {
		return
	}
	a.busy = true
	a.status = "请选择视频…"
	parent := a.window
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "导入视频壁纸", Multiple: true, Filters: []mygo.FileFilter{{Name: "视频", Extensions: []string{"mp4", "mov", "m4v"}}}})
		if err != nil || len(paths) == 0 {
			mygo.RunOnMain(func() {
				a.busy = false
				a.status = "已取消导入"
				if err != nil {
					a.status = err.Error()
				}
				if a.window != nil {
					a.window.Invalidate()
				}
			})
			return
		}
		// Work against a private configuration; the UI never reads partially imported state.
		var worker application
		mygo.RunOnMain(func() {
			worker = application{host: a.host, root: a.root, config: a.config}
			worker.config.Media = slices.Clone(a.config.Media)
			worker.config.Playlist.VideoIDs = slices.Clone(a.config.Playlist.VideoIDs)
			a.status = "正在复制并验证视频…"
			if a.window != nil {
				a.window.Invalidate()
			}
		})
		n, errs := worker.importFiles(paths)
		mygo.RunOnMain(func() {
			a.config = worker.config
			a.busy = false
			saved := a.save()
			a.loadThumbnails()
			if saved {
				a.status = fmt.Sprintf("已导入 %d 个视频", n)
				if len(errs) > 0 {
					a.status += "；" + strings.Join(errs, "；")
				}
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	}()
}
func (a *application) entry(id string) (catalog.MediaEntry, bool) {
	for _, m := range a.config.Media {
		if m.ID == id {
			return m, true
		}
	}
	return catalog.MediaEntry{}, false
}
func (a *application) previewMedia(m catalog.MediaEntry) {
	if a.previewWindow != nil {
		a.previewWindow.Close()
	}
	p, err := video.Open(catalog.VideoPath(a.root, m))
	if err != nil {
		a.status = err.Error()
		return
	}
	p.SetLoop(true).SetMuted(true)
	a.preview = p
	w := mygo.NewWindow(mygo.WindowOptions{Title: m.Name + " · 预览", Width: 960, Height: 580, Content: ui.View(func(c *ui.Context) { ui.Video(c, p).Fill().Fit(ui.Contain) })})
	a.previewWindow = w
	w.OnClosed(func() {
		p.Close()
		if a.previewWindow == w {
			a.previewWindow = nil
			a.preview = nil
		}
	})
}
func (a *application) deleteMedia(m catalog.MediaEntry) {
	if a.busy {
		return
	}
	a.busy = true
	parent := a.window
	go func() {
		r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: parent, Type: mygo.MessageWarning, Message: "从资料库删除「" + m.Name + "」？", Detail: "原始文件会保留。正在播放的屏幕会切换到下一项。", Buttons: []string{"取消", "删除"}, CancelButton: 0, DefaultButton: 0, DestructiveButtons: []int{1}})
		mygo.RunOnMain(func() {
			a.busy = false
			if err == nil && r.Button == 1 {
				a.config.Playlist.VideoIDs = slices.DeleteFunc(a.config.Playlist.VideoIDs, func(id string) bool { return id == m.ID })
				a.config.Media = slices.DeleteFunc(a.config.Media, func(e catalog.MediaEntry) bool { return e.ID == m.ID })
				a.config.RemovedMediaIDs = append(a.config.RemovedMediaIDs, m.ID)
				delete(a.thumbs, m.ID)
				saved := a.save()
				if a.previewWindow != nil {
					a.previewWindow.Close()
				}
				if saved {
					a.status = "已删除；播放切换完成后会清理资料库副本"
					a.reapLater()
				}
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	}()
}
func (a *application) reapLater() {
	go func() {
		for i := 0; i < 3; i++ {
			time.Sleep(6 * time.Second)
			_ = catalog.Reap(a.root)
		}
	}()
}
func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 36, 36))
	ink := color.RGBA{0, 0, 0, 255}
	for y := 6; y < 29; y++ {
		for x := 3; x < 33; x++ {
			if x < 6 || x > 29 || y < 9 || y > 25 {
				img.Set(x, y, ink)
			}
		}
	}
	for y := 12; y < 23; y++ {
		for x := 14; x < 14+(y-12)/2+1 && x < 22; x++ {
			img.Set(x, y, ink)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
