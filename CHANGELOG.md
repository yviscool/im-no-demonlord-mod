# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与 [语义化版本](https://semver.org/lang/zh-CN/)。

## [1.0.0] - 2026-11-04

### 新增

- 游戏内中文作弊面板（`F2`），支持资源与战斗两组共 14 项功能。
- 热键：`F3` 金币、`F9` 灵魂钱币、`F4` 无敌、`F5` 清空本波、`F6` 自动清怪、`F7` 免费商店、`F8` 无限刷新、`ESC` 关闭面板。
- Go 原生加载器：运行时通过 Chrome DevTools 协议注入，不修改游戏文件。
- 页面刷新后自动重新注入；断开后可手动重连。
- 离线安装：备份 `app.asar`、写入 `cheat.js`、同步 exe 的 `INTEGRITY/ELECTRONASAR` 哈希，失败自动回滚。
- 一键还原官方文件。
- 命令行：`status`、`install`、`uninstall`、`inject`，支持 `-game`、`-port`、`-dry-run`、`-force`、`-attach`、`-once`。
- mygo 原生 UI：侧边栏、工具栏、卡片、Segmented、Toast、Spinner、Progress、Badge、Tooltip 和图标，跟随系统主题。
- 启动参数 `--disable-backgrounding-occluded-windows` 等，避免窗口被遮挡时 Cocos 暂停。
- 应用图标与 exe 版本资源。
- 图文文档：加载器与游戏内面板实机截图、架构图、安装、命令行和逆向笔记。
- GitHub Actions：每次推送构建 exe 与发布 zip，打 tag 自动发布 Release。

### 修复

- 双击 exe 不再出现多余的控制台窗口（GUI 子系统 + 子命令附加父控制台）。
- CLI 输出在 cmd、批处理和重定向下均正常。
- 批处理脚本统一为 ASCII，避免 cmd 的 UTF-8 解析问题。

### 变更

- 移除全部 Python 工具，构建、图标生成与打包全部使用 Go / 批处理。
- 发布包命名为 `ImNoDemonlord-LocalCheat-v<version>.zip`。

[1.0.0]: https://github.com/yviscool/im-no-demonlord-mod/releases/tag/v1.0.0
