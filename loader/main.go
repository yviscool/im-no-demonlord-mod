package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/cheat.js
var embeddedCheat string

//go:embed assets/icon.png
var embeddedIcon []byte

func main() {
	args := os.Args[1:]
	cmd := "ui"
	switch {
	case len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help"):
		cmd = "help"
		args = args[1:]
	case len(args) > 0 && !strings.HasPrefix(args[0], "-"):
		cmd = args[0]
		args = args[1:]
	}

	// The program is linked for the Windows GUI subsystem, so a window no
	// longer flashes up when it starts. Subcommands attach to the console of
	// the shell that ran them to keep printing there.
	if cmd != "ui" {
		attachParentConsole()
	}

	switch cmd {
	case "ui":
		cmdUI(args)
	case "inject":
		cmdInject(args)
	case "install":
		cmdInstall(args)
	case "uninstall":
		cmdUninstall(args)
	case "status":
		cmdStatus(args)
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`我不是魔王 · 本地作弊 Mod 加载器

用法：
  imnd-loader.exe                    打开图形界面（默认）
  imnd-loader.exe ui                同上
  imnd-loader.exe install           把 cheat.js 离线写入 app.asar（自动备份）
  imnd-loader.exe uninstall         用 app.asar.bak 还原官方文件
  imnd-loader.exe status            查看安装状态
  imnd-loader.exe inject            启动游戏并注入（无界面）

通用参数：
  -game <目录>   游戏根目录（默认自动查找）
  -port <端口>   调试端口，默认 9222

install / uninstall：
  -force         游戏正在运行时也强制写入
  -dry-run       只检查，不写文件

inject：
  -attach        不启动游戏，注入到已运行的游戏
  -no-inject     只启动游戏
  -once          注入成功后立即退出

示例：
  imnd-loader.exe install
  imnd-loader.exe inject -attach
`)
}

func loadCheat() string {
	if path := os.Getenv("IMND_CHEAT_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("读取 IMND_CHEAT_FILE 失败：%v", err)
		}
		return string(data)
	}
	return embeddedCheat
}

func resolveGame(flagValue string) string {
	dir := findGameDir(flagValue)
	if dir == "" {
		fmt.Fprintln(os.Stderr, "找不到游戏目录。请用 -game 指定，例如：")
		fmt.Fprintln(os.Stderr, `  imnd-loader.exe install -game "D:\game\ImNoDemonlord"`)
		os.Exit(1)
	}
	return dir
}

func cmdUI(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	game := fs.String("game", "", "游戏根目录")
	port := fs.Int("port", defaultPort, "Electron 远程调试端口")
	_ = fs.Parse(args)

	cfg := loadConfig()
	if *port == defaultPort && cfg.Port > 0 {
		*port = cfg.Port
	}
	gameDir := findGameDir(*game)
	app := newApp(gameDir, *port, loadCheat())
	if gameDir != "" {
		_ = saveConfig(loaderConfig{GameDir: gameDir, Port: *port})
	}
	runGUI(app)
}

func cmdInject(args []string) {
	fs := flag.NewFlagSet("inject", flag.ExitOnError)
	game := fs.String("game", "", "游戏根目录")
	port := fs.Int("port", defaultPort, "Electron 远程调试端口")
	attach := fs.Bool("attach", false, "不启动游戏，注入到已运行的游戏")
	noInject := fs.Bool("no-inject", false, "只启动游戏")
	once := fs.Bool("once", false, "注入成功后立即退出")
	_ = fs.Parse(args)

	cfg := loadConfig()
	if *port == defaultPort && cfg.Port > 0 {
		*port = cfg.Port
	}
	app := newApp(findGameDir(*game), *port, loadCheat())
	runCLI(app, *attach, *noInject, *once)
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	game := fs.String("game", "", "游戏根目录")
	dry := fs.Bool("dry-run", false, "只检查，不写文件")
	force := fs.Bool("force", false, "游戏正在运行时也强制写入")
	_ = fs.Parse(args)

	dir := resolveGame(*game)
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println(" 我不是魔王 · 离线安装（修改 app.asar）")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("  游戏目录 : %s\n\n", dir)

	err := installOffline(dir, loadCheat(), *dry, *force, cliLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n安装失败：%v\n", err)
		os.Exit(1)
	}
	if !*dry {
		fmt.Println("\n完成。启动游戏后按 F2 打开作弊菜单。")
		fmt.Println("还原：imnd-loader.exe uninstall")
	}
}

func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	game := fs.String("game", "", "游戏根目录")
	force := fs.Bool("force", false, "游戏正在运行时也强制写入")
	_ = fs.Parse(args)

	dir := resolveGame(*game)
	fmt.Println("正在还原官方 app.asar ...")
	if err := uninstallOffline(dir, *force, cliLog); err != nil {
		fmt.Fprintf(os.Stderr, "还原失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("还原完成。")
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	game := fs.String("game", "", "游戏根目录")
	port := fs.Int("port", defaultPort, "Electron 远程调试端口")
	_ = fs.Parse(args)

	cfg := loadConfig()
	if *port == defaultPort && cfg.Port > 0 {
		*port = cfg.Port
	}
	dir := findGameDir(*game)
	if dir == "" {
		fmt.Println("游戏目录：未找到（用 -game 指定）")
		return
	}
	st := inspectInstall(dir)
	fmt.Printf("游戏目录：%s\n", dir)
	fmt.Printf("游戏状态：%s\n", map[bool]string{true: "运行中", false: "未运行"}[gameRunning()])
	fmt.Printf("asar 安装：%s\n", map[bool]string{true: "已安装（cheat v" + st.Version + "）", false: "未安装"}[st.Installed])
	fmt.Printf("原始备份：%s\n", map[bool]string{true: "app.asar.bak 存在", false: "无"}[st.HasBackup])
	fmt.Printf("调试端口：%d %s\n", *port, map[bool]string{true: "可连接", false: "未开放"}[debugPortOpen(context.Background(), *port)])
}

func cliLog(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func runGUI(app *App) {
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "我不是魔王 · 作弊 Mod 加载器",
			Width:     920,
			Height:    700,
			MinWidth:  780,
			MinHeight: 580,
			StateKey:  "imnd-loader",
			Content:   ui.View(app.view),
		})
		_ = app.win.SetIcon(embeddedIcon)
		app.logf("加载器已启动（%s）。", loaderVersion)
		app.sanityCheck()
		app.refreshInstallStatus()
		go app.monitor()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func runCLI(app *App, attach, noInject, once bool) {
	app.cliMode = true
	app.sanityCheck()
	if attach {
		app.attach()
	} else {
		app.launchAndInject(!noInject)
	}
	if !once {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		<-ch
		return
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if app.injectedState() {
			fmt.Println("注入成功。")
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "注入超时。")
	os.Exit(2)
}
