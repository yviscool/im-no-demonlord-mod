package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

const loaderVersion = "1.0.0"

type App struct {
	gameDir string
	port    int
	cheat   string
	win     *mygo.Window

	mu         sync.Mutex
	status     string
	logs       []string
	connected  bool
	injected   bool
	cheatVer   string
	busy       bool
	client     *cdpClient
	cancel     context.CancelFunc
	cliMode    bool
	install    installStatus
	installing bool

	// UI state: which page shows, how the primary button launches the game,
	// whether the game process is running, and outcomes waiting to be
	// shown as toasts.
	page    string
	mode    int
	running bool
	notices []notice
}

// notice is a message that an action queued for the view to toast.
type notice struct {
	text string
	kind string
}

func newApp(gameDir string, port int, cheat string) *App {
	return &App{
		gameDir: gameDir,
		port:    port,
		cheat:   cheat,
		page:    "run",
		mode:    0,
		status:  "就绪：点击「启动游戏并注入」，或在游戏已开启调试端口时点击「连接到已运行游戏」。",
	}
}

func (a *App) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	a.mu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 200 {
		a.logs = a.logs[len(a.logs)-200:]
	}
	cli := a.cliMode
	a.mu.Unlock()
	if cli {
		fmt.Println(line)
	}
	a.redraw()
}

func (a *App) injectedState() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.injected
}

func (a *App) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.redraw()
}

func (a *App) setConnected(v bool) {
	a.mu.Lock()
	a.connected = v
	a.mu.Unlock()
	a.redraw()
}

func (a *App) setInjected(v bool) {
	a.mu.Lock()
	a.injected = v
	a.mu.Unlock()
	a.redraw()
}

func (a *App) setBusy(v bool) {
	a.mu.Lock()
	a.busy = v
	a.mu.Unlock()
	a.redraw()
}

func (a *App) setPage(page string) {
	a.mu.Lock()
	a.page = page
	a.mu.Unlock()
}

func (a *App) setMode(mode int) {
	a.mu.Lock()
	a.mode = mode
	a.mu.Unlock()
}

// notify queues an outcome for the view, which shows it as a toast.
func (a *App) notify(text, kind string) {
	a.mu.Lock()
	if a.cliMode {
		a.mu.Unlock()
		return
	}
	a.notices = append(a.notices, notice{text: text, kind: kind})
	a.mu.Unlock()
	a.redraw()
}

func (a *App) drainNotices() []notice {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := a.notices
	a.notices = nil
	return n
}

// uiState is the snapshot the view builds from.
type uiState struct {
	page       string
	mode       int
	status     string
	logs       []string
	connected  bool
	injected   bool
	busy       bool
	installing bool
	cheatVer   string
	install    installStatus
	port       int
	gameDir    string
	running    bool
}

func (a *App) uiState() uiState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return uiState{
		page:       a.page,
		mode:       a.mode,
		status:     a.status,
		logs:       append([]string(nil), a.logs...),
		connected:  a.connected,
		injected:   a.injected,
		busy:       a.busy,
		installing: a.installing,
		cheatVer:   a.cheatVer,
		install:    a.install,
		port:       a.port,
		gameDir:    a.gameDir,
		running:    a.running,
	}
}

// launchCurrent runs the action chosen with the segmented control.
func (a *App) launchCurrent() {
	a.mu.Lock()
	mode := a.mode
	a.mu.Unlock()
	switch mode {
	case 1:
		a.launchAndInject(false)
	case 2:
		a.attach()
	default:
		a.launchAndInject(true)
	}
}

// monitor keeps the game's process state fresh for the view.
func (a *App) monitor() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		running := gameRunning()
		a.mu.Lock()
		changed := running != a.running
		a.running = running
		a.mu.Unlock()
		if changed {
			a.redraw()
		}
	}
}

func (a *App) copyLogs() {
	a.mu.Lock()
	logs := append([]string(nil), a.logs...)
	a.mu.Unlock()
	if len(logs) == 0 {
		a.notify("日志是空的", "")
		return
	}
	if err := mygo.Clipboard.Write(transfer.TextData(strings.Join(logs, "\n"))); err != nil {
		a.notify("复制日志失败："+err.Error(), "error")
		return
	}
	a.notify("日志已复制到剪贴板", "success")
}

func (a *App) clearLogs() {
	a.mu.Lock()
	a.logs = nil
	a.mu.Unlock()
	a.redraw()
}

func (a *App) redraw() {
	win := a.win
	if win == nil {
		return
	}
	win.Update(func() {})
}

func (a *App) setGameDir(dir string) {
	a.mu.Lock()
	a.gameDir = dir
	a.mu.Unlock()
	a.redraw()
}

// startSession cancels a previous watcher and returns a fresh context.
func (a *App) startSession() context.Context {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()
	return ctx
}

func (a *App) stopSession() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.mu.Unlock()
	a.setConnected(false)
	a.setInjected(false)
}

func (a *App) launchAndInject(inject bool) {
	if a.gameDir == "" {
		a.setStatus("没有找到游戏目录，请先点击「选择目录…」。")
		return
	}
	if debugPortOpen(context.Background(), a.port) {
		a.logf("端口 %d 已被占用，改为附加到已运行的游戏。", a.port)
		if inject {
			a.attach()
		}
		return
	}

	a.setStatus("正在启动游戏…")
	a.logf("启动：%s --remote-debugging-port=%d", filepath.Join(a.gameDir, exeName), a.port)
	if _, err := launchGame(a.gameDir, a.port); err != nil {
		a.logf("启动失败：%v", err)
		a.setStatus("启动失败：" + err.Error())
		return
	}
	a.logf("游戏已启动，等待调试端口…")
	if inject {
		a.watchAndInject(true)
	}
}

func (a *App) attach() {
	if a.gameDir == "" {
		a.setStatus("没有找到游戏目录，请先点击「选择目录…」。")
		return
	}
	if !debugPortOpen(context.Background(), a.port) {
		a.setStatus(fmt.Sprintf("端口 %d 没有响应：请用「启动游戏并注入」重启游戏（游戏本身不带调试端口）。", a.port))
		a.logf("附加失败：端口 %d 未开放。", a.port)
		return
	}
	a.watchAndInject(false)
}

// watchAndInject waits for the game page, then keeps it injected.
func (a *App) watchAndInject(justLaunched bool) {
	ctx := a.startSession()
	a.setBusy(true)
	a.setStatus("等待游戏页面出现在调试端口…")
	a.logf("等待 %s …", pageHint)

	go func() {
		timeout := 90 * time.Second
		if !justLaunched {
			timeout = 10 * time.Second
		}
		target, err := waitForGamePage(ctx, a.port, timeout)
		if err != nil {
			a.setBusy(false)
			if ctx.Err() == nil {
				a.logf("连接失败：%v", err)
				a.setStatus("连接失败：" + err.Error())
			}
			return
		}
		a.logf("已找到游戏页面：%s", target.URL)
		a.injectLoop(ctx, target)
	}()
}

func (a *App) injectLoop(ctx context.Context, target cdpTarget) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := a.injectOnce(ctx, target)
		a.mu.Lock()
		a.client = nil
		a.mu.Unlock()
		a.setConnected(false)
		a.setInjected(false)
		a.setBusy(false)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if !gameRunning() {
				a.logf("游戏已退出，停止注入。")
				a.setStatus("游戏已退出。")
				return
			}
			a.logf("连接断开：%v", err)
			a.setStatus("与游戏页面的连接断开，2 秒后重试…")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (a *App) injectOnce(ctx context.Context, target cdpTarget) error {
	onEvent := func(method string, _ json.RawMessage) {
		if method == "Page.loadEventFired" {
			// The page (re)loaded: inject again once its DOM is ready.
			time.AfterFunc(700*time.Millisecond, func() {
				client := a.currentClient()
				if client == nil {
					return
				}
				if err := a.injectSource(ctx, client); err != nil {
					a.logf("页面刷新后重新注入失败：%v", err)
				}
			})
		}
	}
	client, err := dialCDP(ctx, target.WebSocketDebuggerURL, onEvent)
	if err != nil {
		return err
	}
	defer client.Close()

	a.mu.Lock()
	a.client = client
	a.mu.Unlock()

	if _, err := client.call(ctx, "Page.enable", map[string]any{}); err != nil {
		return err
	}
	if _, err := client.call(ctx, "Runtime.enable", map[string]any{}); err != nil {
		return err
	}
	a.setConnected(true)
	a.setBusy(false)
	a.setStatus("已连接游戏页面，正在注入 cheat.js …")
	if err := a.injectSource(ctx, client); err != nil {
		return err
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-client.closed:
			return fmt.Errorf("WebSocket 已关闭")
		case <-ticker.C:
			if a.isInjected(client) {
				continue
			}
			if err := a.injectSource(ctx, client); err != nil {
				return err
			}
		}
	}
}

func (a *App) currentClient() *cdpClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

func (a *App) isInjected(client *cdpClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	out, err := client.evaluate(ctx, `(function(){try{return window.__IMND_CHEAT__===true&&typeof window.__IMND_CHEAT_VERSION__==="string"?JSON.stringify({ok:true,v:window.__IMND_CHEAT_VERSION__}):JSON.stringify({ok:false})}catch(e){return JSON.stringify({ok:false})}})()`)
	if err != nil {
		return false
	}
	s, _ := out.Value.(string)
	var res struct {
		OK bool   `json:"ok"`
		V  string `json:"v"`
	}
	if json.Unmarshal([]byte(s), &res) != nil {
		return false
	}
	if res.OK {
		a.mu.Lock()
		changed := !a.injected || a.cheatVer != res.V
		a.injected = true
		a.cheatVer = res.V
		a.mu.Unlock()
		if changed {
			a.logf("作弊功能已就绪（cheat v%s），按 F2 打开菜单。", res.V)
			a.setStatus("已注入：按 F2 打开作弊菜单。")
			a.notify("已连接并注入，按 F2 打开作弊菜单", "success")
			a.redraw()
		}
	}
	return res.OK
}

func (a *App) injectSource(ctx context.Context, client *cdpClient) error {
	out, err := client.evaluate(ctx, a.cheat)
	if err != nil {
		return err
	}
	if out.Exception != "" {
		trimmed := out.Exception
		if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		a.logf("注入脚本抛出异常：%s", trimmed)
	}
	if !a.isInjected(client) {
		return fmt.Errorf("注入后没有检测到 window.__IMND_CHEAT__（游戏可能还没加载完，稍后重试）")
	}
	return nil
}

// resetCheat turns every cheat toggle off inside the running page.
func (a *App) resetCheat() {
	client := a.currentClient()
	if client == nil {
		a.setStatus("还没有连接到游戏页面。")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := client.evaluate(ctx, `(function(){try{if(window.__IMND_CHEAT_API__){window.__IMND_CHEAT_API__.reset();return "ok"}return "missing"}catch(e){return "err: "+e}})()`)
	if err != nil {
		a.logf("关闭作弊功能失败：%v", err)
		return
	}
	a.logf("已在游戏内关闭全部作弊功能（%v）。", out.Value)
	a.setStatus("已关闭全部作弊功能；按 F2 仍可重新打开菜单。")
	a.notify("已在游戏内关闭全部作弊功能", "success")
}

func (a *App) chooseGameDir() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       "选择《我不是魔王》游戏目录（含 resources/app.asar）",
		ButtonLabel: "选择",
		Directory:   true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := filepath.Clean(paths[0])
	if !isGameDir(dir) && isGameDir(filepath.Join(dir, "..")) {
		dir = filepath.Clean(filepath.Join(dir, ".."))
	}
	if !isGameDir(dir) {
		a.logf("目录无效：%s 中找不到 resources/app.asar", dir)
		a.setStatus("选择的目录不是游戏根目录（需要包含 resources/app.asar）。")
		return
	}
	a.setGameDir(dir)
	_ = saveConfig(loaderConfig{GameDir: dir, Port: a.port})
	a.logf("游戏目录已设置为：%s", dir)
	a.setStatus("游戏目录已更新。")
}

func (a *App) openGameDir() {
	if a.gameDir == "" {
		a.setStatus("还没有设置游戏目录。")
		return
	}
	if err := openFolder(a.gameDir); err != nil {
		a.logf("打开目录失败：%v", err)
	}
}

func (a *App) detected() string {
	if a.gameDir == "" {
		return "未找到游戏目录"
	}
	return a.gameDir
}

func (a *App) cheatVersionLabel() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cheatVer == "" {
		return "cheat.js v" + cheatVersionFromSource(a.cheat)
	}
	return "cheat.js v" + a.cheatVer
}

func cheatVersionFromSource(src string) string {
	const marker = "var VERSION = '"
	idx := strings.Index(src, marker)
	if idx < 0 {
		return "?"
	}
	rest := src[idx+len(marker):]
	if end := strings.IndexByte(rest, '\''); end > 0 {
		return rest[:end]
	}
	return "?"
}

func (a *App) sanityCheck() {
	if a.gameDir == "" {
		a.logf("提示：没有自动找到游戏，请点击「选择目录…」指定游戏根目录。")
		return
	}
	if !isGameDir(a.gameDir) {
		a.logf("提示：%s 下没有 resources/app.asar。", a.gameDir)
	}
	if exe, err := os.Stat(filepath.Join(a.gameDir, exeName)); err != nil || exe.IsDir() {
		a.logf("提示：%s 下没有 %s。", a.gameDir, exeName)
	}
}

// --- 离线安装（修改 app.asar） ------------------------------------------------

func (a *App) refreshInstallStatus() {
	a.mu.Lock()
	dir := a.gameDir
	installing := a.installing
	a.mu.Unlock()
	if dir == "" || installing {
		return
	}
	st := inspectInstall(dir)
	a.mu.Lock()
	a.install = st
	a.mu.Unlock()
	a.redraw()
}

func (a *App) installToGame() {
	a.mu.Lock()
	dir := a.gameDir
	installing := a.installing
	a.mu.Unlock()
	if dir == "" {
		a.setStatus("没有找到游戏目录，请先点击「选择目录…」。")
		return
	}
	if installing {
		return
	}
	if gameRunning() {
		a.setStatus("游戏正在运行：请先完全退出游戏，再点「安装到游戏文件」。")
		a.logf("安装被拒绝：游戏正在运行。")
		return
	}

	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:  a.win,
		Type:    mygo.MessageWarning,
		Message: "把 cheat.js 离线写入 app.asar？",
		Detail:  "会先备份原文件为 app.asar.bak，并同步 exe 内的 asar 完整性校验值。\n如果只想临时使用，不需要这一步——直接「启动游戏并注入」即可。",
		Buttons: []string{"取消", "安装"},
	})
	if err != nil || res.Button != 1 {
		return
	}

	a.mu.Lock()
	a.installing = true
	a.mu.Unlock()
	a.setStatus("正在写入 app.asar，请稍候…")
	go func() {
		err := installOffline(dir, a.cheat, false, false, a.logf)
		a.mu.Lock()
		a.installing = false
		a.mu.Unlock()
		if err != nil {
			a.logf("安装失败：%v", err)
			a.setStatus("安装失败：" + err.Error())
			a.notify("离线安装失败："+err.Error(), "error")
		} else {
			a.logf("离线安装完成。")
			a.setStatus("已写入 app.asar：启动游戏后按 F2 打开菜单。")
			a.notify("离线安装完成，启动游戏后按 F2 打开菜单", "success")
		}
		a.refreshInstallStatus()
	}()
}

func (a *App) restoreGame() {
	a.mu.Lock()
	dir := a.gameDir
	installing := a.installing
	a.mu.Unlock()
	if dir == "" {
		a.setStatus("没有找到游戏目录。")
		return
	}
	if installing {
		return
	}
	if gameRunning() {
		a.setStatus("游戏正在运行：请先完全退出游戏，再点「还原官方文件」。")
		return
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:  a.win,
		Type:    mygo.MessageWarning,
		Message: "用 app.asar.bak 还原官方文件？",
		Detail:  "会覆盖当前的 app.asar，并把 exe 的完整性校验值同步回原始值。",
		Buttons: []string{"取消", "还原"},
	})
	if err != nil || res.Button != 1 {
		return
	}
	a.mu.Lock()
	a.installing = true
	a.mu.Unlock()
	a.setStatus("正在还原 app.asar…")
	go func() {
		err := uninstallOffline(dir, false, a.logf)
		a.mu.Lock()
		a.installing = false
		a.mu.Unlock()
		if err != nil {
			a.logf("还原失败：%v", err)
			a.setStatus("还原失败：" + err.Error())
			a.notify("还原失败："+err.Error(), "error")
		} else {
			a.logf("已还原官方 app.asar。")
			a.setStatus("已还原官方文件。")
			a.notify("已还原官方 app.asar", "success")
		}
		a.refreshInstallStatus()
	}()
}

func (a *App) installLabel() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.installing {
		return "处理中…"
	}
	if a.install.Installed {
		label := "已安装"
		if a.install.Version != "" {
			label += "（cheat v" + a.install.Version + "）"
		}
		return label
	}
	return "未安装"
}
