package main

import (
	_ "embed"
	"sync"

	"github.com/energye/systray"
)

//go:embed build/windows/icon.ico
var trayIconICO []byte

type trayLabels struct {
	show    string
	quit    string
	tooltip string
}

func trayLabelsFor(language string) trayLabels {
	if language == "en-US" {
		return trayLabels{show: "Show window", quit: "Quit", tooltip: "Feisuo"}
	}
	return trayLabels{show: "显示窗口", quit: "退出", tooltip: "飞梭"}
}

// tray owns the notification area icon. systray keeps its own event loop, so it
// must run on a goroutine separate from the Wails main loop.
type tray struct {
	mu       sync.Mutex
	showItem *systray.MenuItem
	quitItem *systray.MenuItem
}

var appTray = &tray{}

func startTray(app *App) {
	systray.Run(func() { appTray.onReady(app) }, nil)
}

func stopTray() {
	systray.Quit()
}

func (t *tray) onReady(app *App) {
	labels := trayLabelsFor(app.GetLanguage())
	systray.SetIcon(trayIconICO)
	systray.SetTooltip(labels.tooltip)
	systray.SetOnClick(func(systray.IMenu) { app.showWindow() })
	systray.SetOnRClick(func(menu systray.IMenu) {
		if menu != nil {
			_ = menu.ShowMenu()
		}
	})

	show := systray.AddMenuItem(labels.show, "")
	systray.AddSeparator()
	quit := systray.AddMenuItem(labels.quit, "")

	show.Click(func() { app.showWindow() })
	quit.Click(func() { app.requestQuit() })

	t.mu.Lock()
	t.showItem = show
	t.quitItem = quit
	t.mu.Unlock()
}

func (t *tray) refreshLabels(language string) {
	labels := trayLabelsFor(language)
	t.mu.Lock()
	show, quit := t.showItem, t.quitItem
	t.mu.Unlock()
	if show == nil || quit == nil {
		return
	}
	show.SetTitle(labels.show)
	quit.SetTitle(labels.quit)
	systray.SetTooltip(labels.tooltip)
}
