package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// repoURL is filled in once the public repository exists.
const repoURL = "https://github.com/yviscool/im-no-demonlord-mod"

// icon parses the shapes of a 24×24 stroked icon, drawn in the text color.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconPlay     = icon(`<polygon points="6 3 20 12 6 21 6 3"/>`)
	iconDownload = icon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>`)
	iconLogs     = icon(`<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>`)
	iconInfo     = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>`)
	iconFolder   = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	iconRefresh  = icon(`<path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6"/>`)
	iconPower    = icon(`<path d="M12 3v9M6.6 5.6a8 8 0 1 0 10.8 0"/>`)
	iconTrash    = icon(`<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v5M14 11v5"/>`)
	iconCopy     = icon(`<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h8"/>`)
	iconKeyboard = icon(`<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M7 14h10"/>`)
)

func launchLabel(mode int) string {
	switch mode {
	case 1:
		return "仅启动游戏"
	case 2:
		return "连接到已运行游戏"
	default:
		return "启动游戏并注入"
	}
}

// card is the standard raised surface of the window, as the gallery uses.
func card(c *ui.Context, fn func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16).Gap(10).Radius(t.Radius+2).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06)).Children(fn)
}

func sectionTitle(c *ui.Context, title string) {
	t := c.Theme()
	ui.Text(c, title).FontSize(13).Bold().TextColor(t.Text)
}

func muted(c *ui.Context, text string) {
	t := c.Theme()
	ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).Wrap()
}

func infoRow(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(86).Shrink(0)
		ui.Text(c, value).FontSize(12).TextColor(t.Text).Grow(1).Wrap()
	})
}

func keycap(c *ui.Context, key string) {
	t := c.Theme()
	ui.Text(c, key).Font("monospace").FontSize(11).Bold().
		Padding(1, 6).Radius(5).Background(t.Surface).Border(1, t.Border).TextColor(t.Text)
}

func hotkeyRow(c *ui.Context, key, what string) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		keycap(c, key)
		ui.Text(c, what).FontSize(12).TextColor(t.TextMuted).Grow(1)
	})
}

func statusDot(c *ui.Context, color ui.Color, label string) {
	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(c, "●").FontSize(11).TextColor(color)
		ui.Text(c, label).FontSize(12).Bold().TextColor(color)
	})
}

func (a *App) view(c *ui.Context) {
	st := a.uiState()

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, st)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			a.toolbar(c, st)
			switch st.page {
			case "logs":
				a.pageLogs(c, st)
			default:
				ui.Scroll(c).Grow(1).Padding(20, 24, 24).Gap(16).Children(func() {
					switch st.page {
					case "install":
						a.pageInstall(c, st)
					case "about":
						a.pageAbout(c, st)
					default:
						a.pageRun(c, st)
					}
				})
			}
		})
	})

	// Actions run on goroutines, so they queue their outcome here and the
	// view shows it as a toast, as dialogs and saves do.
	for _, n := range a.drainNotices() {
		c.AddToast(ui.Toast{Title: n.text, Type: n.kind})
	}
}

func (a *App) sidebar(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Width(188).Shrink(0).Padding(14, 14, 12).Gap(8).
		Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Column(c).Padding(2, 4, 10).Gap(1).Children(func() {
			ui.Text(c, "我不是魔王").FontSize(16).Bold()
			ui.Text(c, "作弊 Mod 加载器").FontSize(11).TextColor(t.TextMuted)
		})

		switch {
		case st.injected:
			statusDot(c, t.Success, "已注入")
		case st.connected:
			statusDot(c, t.Warning, "已连接")
		default:
			statusDot(c, t.TextMuted, "未连接")
		}

		page := st.page
		if ui.Sidebar(c, &page, func() {
			ui.SidebarItem(c, "run", iconPlay, "运行")
			ui.SidebarItem(c, "install", iconDownload, "离线安装")
			ui.SidebarItem(c, "logs", iconLogs, "日志").Children(func() {
				if n := len(st.logs); n > 0 {
					ui.Badge(c, fmt.Sprint(n))
				}
			})
			ui.SidebarItem(c, "about", iconInfo, "关于")
		}).Grow(1).Label("导航").Changed() {
			a.setPage(page)
		}

		ui.Spacer(c)
		ui.Divider(c)
		ui.Column(c).Padding(2, 4, 0).Gap(2).Children(func() {
			ui.Text(c, fmt.Sprintf("调试端口 %d", st.port)).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "native loader v"+loaderVersion).FontSize(11).TextColor(t.TextMuted)
		})
	})
}

func (a *App) toolbar(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Children(func() {
		ui.Toolbar(c, func() {
			switch {
			case st.injected:
				statusDot(c, t.Success, "已注入")
			case st.connected:
				statusDot(c, t.Warning, "已连接")
			case st.busy:
				ui.Spinner(c).FontSize(12)
				ui.Text(c, "连接中…").FontSize(12).TextColor(t.TextMuted)
			default:
				statusDot(c, t.TextMuted, "未连接")
			}

			ui.Spacer(c)

			if ui.Button(c, "").Label("打开游戏目录").Tooltip("在资源管理器中打开游戏目录").
				Children(func() { ui.Icon(c, iconFolder) }).Clicked() {
				go a.openGameDir()
			}
			if ui.Button(c, "").Label("断开并重连").Tooltip("重新连接调试端口并注入").
				Children(func() { ui.Icon(c, iconRefresh) }).Clicked() {
				go func() {
					a.stopSession()
					a.attach()
				}()
			}
			if ui.Button(c, "").Label("关闭全部作弊").Tooltip("把游戏内的所有作弊开关关掉").
				Children(func() { ui.Icon(c, iconPower) }).Clicked() {
				go a.resetCheat()
			}
			if ui.PrimaryButton(c, launchLabel(st.mode)).Disabled(st.busy || st.installing).Clicked() {
				go a.launchCurrent()
			}
		}).Label("操作").Padding(6, 12, 0)
		ui.Divider(c)
	})
}

func (a *App) pageRun(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, "运行时注入").FontSize(24).Bold()
		muted(c, "启动游戏时打开 Chrome DevTools 调试端口，把 cheat.js 注入页面。不会修改 app.asar，也不会改动 exe 的完整性校验。")
	})

	mode := st.mode
	card(c, func() {
		sectionTitle(c, "启动方式")
		ui.Segmented(c, &mode, "启动并注入", "仅启动游戏", "附加到已运行").Label("启动方式")
		if mode != st.mode {
			a.setMode(mode)
		}
		muted(c, "游戏本身不带调试端口，所以「附加」只适用于已经用调试端口启动的游戏。")
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			if ui.PrimaryButton(c, launchLabel(mode)).Disabled(st.busy || st.installing).Clicked() {
				go a.launchCurrent()
			}
			if st.busy || st.installing {
				ui.Spinner(c).FontSize(13)
				ui.Text(c, "正在处理…").FontSize(12).TextColor(t.TextMuted)
			}
		})
	})

	card(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			sectionTitle(c, "状态")
			ui.Spacer(c)
			if st.connected {
				ui.Text(c, "已连接").FontSize(11).TextColor(t.Success)
			}
			if st.injected {
				ui.Text(c, "已注入").FontSize(11).TextColor(t.Success)
			}
			if st.cheatVer != "" {
				ui.Text(c, "cheat v"+st.cheatVer).FontSize(11).TextColor(t.TextMuted)
			}
		})
		ui.Text(c, st.status).FontSize(12).TextColor(t.Text).Wrap()
		ui.Divider(c)
		infoRow(c, "调试端口", fmt.Sprintf("%d", st.port))
		infoRow(c, "cheat 版本", a.cheatVersionLabel())
		infoRow(c, "连接状态", map[bool]string{true: "已连接游戏页面", false: "未连接"}[st.connected])
		infoRow(c, "注入状态", map[bool]string{true: "已注入，按 F2 打开菜单", false: "未注入"}[st.injected])
	})

	card(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			sectionTitle(c, "游戏目录")
			ui.Spacer(c)
			if st.running {
				ui.Text(c, "游戏运行中").FontSize(11).TextColor(t.Success)
			}
		})
		if st.gameDir == "" {
			ui.Text(c, "还没有找到游戏目录。请选择包含 resources/app.asar 的游戏根目录。").FontSize(12).TextColor(t.Warning).Wrap()
		} else {
			ui.Text(c, st.gameDir).Font("monospace").FontSize(11.5).TextColor(t.Text).Wrap()
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconFolder)
				ui.Text(c, "选择目录…").SingleLine()
			}).Clicked() {
				go a.chooseGameDir()
			}
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconPlay)
				ui.Text(c, "打开目录").SingleLine()
			}).Clicked() {
				go a.openGameDir()
			}
		})
	})
}

func (a *App) pageInstall(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, "离线安装").FontSize(24).Bold()
		muted(c, "可选：把 cheat.js 直接写进 app.asar，让作弊菜单在每次启动时自动出现。")
	})

	callout := t.Background.Mix(t.Warning, 0.12)
	ui.Column(c).Padding(12).Gap(4).Radius(t.Radius+2).
		Background(callout).Border(1, t.Warning.Alpha(0.4)).Children(func() {
		ui.Text(c, "会修改游戏文件").FontSize(12).Bold().TextColor(t.Text)
		ui.Text(c, "安装前会自动备份为 app.asar.bak，并同步 exe 内的 asar 完整性校验值。还原时用备份覆盖回去。").
			FontSize(11.5).TextColor(t.TextMuted).Wrap()
	})

	card(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			sectionTitle(c, "当前状态")
			ui.Spacer(c)
			if st.installing {
				ui.Spinner(c).FontSize(13)
			}
		})
		ui.Divider(c)
		infoRow(c, "app.asar", map[bool]string{true: "已安装作弊脚本", false: "官方原始文件"}[st.install.Installed])
		infoRow(c, "cheat 版本", map[bool]string{true: st.install.Version, false: "—"}[st.install.Version != ""])
		infoRow(c, "原始备份", map[bool]string{true: "app.asar.bak 存在", false: "不存在"}[st.install.HasBackup])
		infoRow(c, "游戏进程", map[bool]string{true: "正在运行（请先完全退出）", false: "未运行"}[st.running])

		if st.installing {
			ui.Progress(c, -1).Height(4)
			ui.Text(c, "正在处理 app.asar，请稍候…").FontSize(11.5).TextColor(t.TextMuted)
		}

		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "安装到游戏文件").Disabled(st.installing || st.running || st.gameDir == "").Clicked() {
				go a.installToGame()
			}
			if ui.Button(c, "还原官方文件").Disabled(st.installing || st.running || !st.install.HasBackup).TextColor(t.Danger).Clicked() {
				go a.restoreGame()
			}
		})
		if st.running {
			ui.Text(c, "游戏正在运行，安装和还原都要求游戏完全退出。").FontSize(11.5).TextColor(t.Danger)
		}
	})
}

func (a *App) pageLogs(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(20, 24, 20).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "日志").FontSize(24).Bold()
			ui.Spacer(c)
			if ui.Button(c, "").Label("复制全部").Tooltip("把日志复制到剪贴板").Disabled(len(st.logs) == 0).
				Children(func() {
					ui.Icon(c, iconCopy)
					ui.Text(c, "复制").SingleLine()
				}).Clicked() {
				go a.copyLogs()
			}
			if ui.Button(c, "").Label("清空日志").Tooltip("清空窗口中的日志").Disabled(len(st.logs) == 0).
				Children(func() {
					ui.Icon(c, iconTrash)
					ui.Text(c, "清空").SingleLine()
				}).Clicked() {
				go a.clearLogs()
			}
		})
		ui.Scroll(c).Grow(1).Padding(10, 12).Gap(3).Radius(t.Radius+2).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			if len(st.logs) == 0 {
				ui.Text(c, "（还没有日志）").FontSize(11.5).TextColor(t.TextMuted)
				return
			}
			for _, line := range st.logs {
				ui.Text(c, line).Font("monospace").FontSize(11).TextColor(t.TextMuted).Wrap()
			}
		})
	})
}

func (a *App) pageAbout(c *ui.Context, st uiState) {
	t := c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, "关于").FontSize(24).Bold()
		muted(c, fmt.Sprintf("我不是魔王 · 本地作弊 Mod，native 加载器 v%s。", loaderVersion))
	})

	card(c, func() {
		sectionTitle(c, "它做什么")
		muted(c, "通过 Chrome DevTools 协议连接游戏页面，注入一份本地 cheat.js，提供金币、灵魂钱币、无敌、商店、刷新、清怪等功能。")
		muted(c, "默认的运行时注入不改动任何游戏文件；只有主动使用「离线安装」时才会备份并写入 app.asar。")
	})

	card(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, iconKeyboard).FontSize(14).TextColor(t.TextMuted)
			sectionTitle(c, "游戏内热键")
		})
		ui.Divider(c)
		hotkeyRow(c, "F2", "打开 / 关闭作弊面板")
		hotkeyRow(c, "F3", "金币 +999999")
		hotkeyRow(c, "F9", "灵魂钱币 +999999")
		hotkeyRow(c, "F4", "无敌")
		hotkeyRow(c, "F5", "清空本波敌人")
		hotkeyRow(c, "F6", "自动清怪")
		hotkeyRow(c, "F7", "免费商店")
		hotkeyRow(c, "F8", "无限刷新")
		hotkeyRow(c, "ESC", "关闭作弊面板")
	})

	card(c, func() {
		sectionTitle(c, "实现")
		infoRow(c, "界面", "mygo 原生 UI（Direct3D 11 / Metal / OpenGL）")
		infoRow(c, "注入", "Chrome DevTools Protocol · Runtime.evaluate")
		infoRow(c, "安装", "Go 原生 asar 读写 + exe 完整性同步")
		infoRow(c, "依赖", "只有一个 exe，不需要 Python 或运行时")
	})

	card(c, func() {
		muted(c, "仅供单机娱乐。使用前请自行备份存档；联机或涉及他人数据时请关闭全部功能。")
		ui.Row(c).Gap(8).Children(func() {
			ui.Link(c, "GitHub 仓库", repoURL)
			if ui.Button(c, "打开游戏目录").Disabled(st.gameDir == "").Clicked() {
				go a.openGameDir()
			}
		})
	})
}
