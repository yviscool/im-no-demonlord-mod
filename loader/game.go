package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	exeName     = "ImNoDemonlord.exe"
	pageHint    = "web-mobile/index.html"
	loaderName  = "ImNoDemonlord Mod Loader"
	configDir   = "imnd-loader"
	configFile  = "config.json"
	defaultPort = 9222
)

type loaderConfig struct {
	GameDir string `json:"gameDir"`
	Port    int    `json:"port"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, configDir, configFile)
}

func loadConfig() loaderConfig {
	var cfg loaderConfig
	path := configPath()
	if path == "" {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg loaderConfig) error {
	path := configPath()
	if path == "" {
		return fmt.Errorf("无法确定配置目录")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func isGameDir(dir string) bool {
	if dir == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, "resources", "app.asar"))
	return err == nil && !st.IsDir()
}

// findGameDir looks for the game next to the loader, in the config, or in the
// current working directory.
func findGameDir(explicit string) string {
	var candidates []string
	add := func(p string) {
		if p != "" {
			candidates = append(candidates, p)
		}
	}

	add(explicit)
	add(os.Getenv("IMND_GAME"))
	add(loadConfig().GameDir)

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(dir)
		add(filepath.Join(dir, ".."))
		add(filepath.Join(dir, "..", ".."))
		add(filepath.Join(dir, "..", "..", ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
		add(filepath.Join(wd, ".."))
		add(filepath.Join(wd, "..", ".."))
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if isGameDir(abs) {
			return abs
		}
	}
	return ""
}

// --- Chrome DevTools Protocol page discovery ---------------------------------

type cdpTarget struct {
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func listTargets(ctx context.Context, port int) ([]cdpTarget, error) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func pickGameTarget(targets []cdpTarget) (cdpTarget, bool) {
	for _, target := range targets {
		if target.Type != "page" || target.WebSocketDebuggerURL == "" {
			continue
		}
		if strings.Contains(target.URL, pageHint) {
			return target, true
		}
	}
	return cdpTarget{}, false
}

// waitForGamePage polls until the game page is exposed on the debug port.
func waitForGamePage(ctx context.Context, port int, timeout time.Duration) (cdpTarget, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return cdpTarget{}, ctx.Err()
		}
		targets, err := listTargets(ctx, port)
		if err != nil {
			lastErr = err
		} else if target, ok := pickGameTarget(targets); ok {
			return target, nil
		}
		select {
		case <-ctx.Done():
			return cdpTarget{}, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return cdpTarget{}, fmt.Errorf("等待调试端口 %d 超时：%v", port, lastErr)
	}
	return cdpTarget{}, fmt.Errorf("等待调试端口 %d 超时：游戏可能需要 --remote-debugging-port 参数启动", port)
}

func debugPortOpen(ctx context.Context, port int) bool {
	_, err := listTargets(ctx, port)
	return err == nil
}
