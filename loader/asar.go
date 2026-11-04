package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	asarIndexPath = "web-mobile/index.html"
	asarCheatPath = "web-mobile/cheat.js"
	asarMarker    = "<!-- ImNoDemonlord Local-Cheat -->"
	integrityMark = `"alg":"SHA256","value":"`
	blockSize     = 4 * 1024 * 1024
)

// asarArchive is a read-only view of an Electron asar file.
type asarArchive struct {
	path      string
	header    map[string]any
	dataStart int64
	headerRaw []byte
}

type asarLeaf struct {
	Path  string
	Entry map[string]any
}

func openAsar(path string) (*asarArchive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	first := make([]byte, 16)
	if _, err := io.ReadFull(f, first); err != nil {
		return nil, fmt.Errorf("不是有效的 asar 文件：%v", err)
	}
	jsonSize := int64(binary.LittleEndian.Uint32(first[12:16]))
	if jsonSize <= 0 || jsonSize > 64*1024*1024 {
		return nil, fmt.Errorf("asar 头长度异常：%d", jsonSize)
	}
	raw := make([]byte, jsonSize)
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, fmt.Errorf("asar 头被截断：%v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("解析 asar 头失败：%v", err)
	}
	pad := (4 - jsonSize%4) % 4
	return &asarArchive{
		path:      path,
		header:    header,
		dataStart: 16 + jsonSize + pad,
		headerRaw: raw,
	}, nil
}

func (a *asarArchive) headerHash() string {
	sum := sha256.Sum256(a.headerRaw)
	return hex.EncodeToString(sum[:])
}

func (a *asarArchive) find(asarPath string) map[string]any {
	node := a.header
	parts := strings.Split(strings.Trim(asarPath, "/"), "/")
	for i, part := range parts {
		files, ok := node["files"].(map[string]any)
		if !ok {
			return nil
		}
		next, ok := files[part].(map[string]any)
		if !ok {
			return nil
		}
		node = next
		if i < len(parts)-1 {
			if _, ok := node["files"]; !ok {
				return nil
			}
		}
	}
	return node
}

func (a *asarArchive) readFile(asarPath string) ([]byte, error) {
	entry := a.find(asarPath)
	if entry == nil {
		return nil, fmt.Errorf("asar 中找不到文件：%s", asarPath)
	}
	if _, isDir := entry["files"]; isDir {
		return nil, fmt.Errorf("asar 条目是目录：%s", asarPath)
	}
	if unpacked, _ := entry["unpacked"].(bool); unpacked {
		return os.ReadFile(a.path + ".unpacked" + string(os.PathSeparator) + filepath.FromSlash(asarPath))
	}
	size, err := entryInt(entry, "size")
	if err != nil {
		return nil, err
	}
	offset, err := entryInt(entry, "offset")
	if err != nil {
		return nil, err
	}
	f, err := os.Open(a.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(a.dataStart+offset, io.SeekStart); err != nil {
		return nil, err
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("读取 asar 文件被截断：%s", asarPath)
	}
	return data, nil
}

func entryInt(entry map[string]any, key string) (int64, error) {
	switch v := entry[key].(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case string:
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return 0, fmt.Errorf("asar 字段 %s 无效：%q", key, v)
		}
		return n, nil
	case nil:
		return 0, fmt.Errorf("asar 条目缺少字段 %s", key)
	default:
		return 0, fmt.Errorf("asar 字段 %s 类型异常", key)
	}
}

func asarLeaves(header map[string]any) []asarLeaf {
	var out []asarLeaf
	var walk func(node map[string]any, prefix string)
	walk = func(node map[string]any, prefix string) {
		files, _ := node["files"].(map[string]any)
		for name, raw := range files {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "/" + name
			}
			if _, isDir := entry["files"]; isDir {
				walk(entry, path)
			} else {
				out = append(out, asarLeaf{Path: path, Entry: entry})
			}
		}
	}
	walk(header, "")
	return out
}

func sumIntegrity(data []byte) map[string]any {
	whole := sha256.Sum256(data)
	blocks := make([]string, 0, (len(data)+blockSize-1)/blockSize)
	for start := 0; start < len(data); start += blockSize {
		end := start + blockSize
		if end > len(data) {
			end = len(data)
		}
		sum := sha256.Sum256(data[start:end])
		blocks = append(blocks, hex.EncodeToString(sum[:]))
	}
	if len(blocks) == 0 {
		blocks = []string{hex.EncodeToString(whole[:])}
	}
	return map[string]any{
		"algorithm": "SHA256",
		"hash":      hex.EncodeToString(whole[:]),
		"blockSize": blockSize,
		"blocks":    blocks,
	}
}

// writeRepackedAsar writes a new asar where changed files are replaced and
// all other packed files are copied byte for byte.
func writeRepackedAsar(archive *asarArchive, changed map[string][]byte, outPath string) (string, error) {
	// Deep copy the header so the original stays untouched.
	raw, err := json.Marshal(archive.header)
	if err != nil {
		return "", err
	}
	var header map[string]any
	if err := json.Unmarshal(raw, &header); err != nil {
		return "", err
	}

	// Map iteration order is random in Go, so keep one ordered leaf list and
	// use it both for laying out offsets and for writing data.
	leaves := asarLeaves(header)

	// Remember original offsets before they are overwritten.
	oldOffsets := map[string]int64{}
	for _, leaf := range leaves {
		if unpacked, _ := leaf.Entry["unpacked"].(bool); unpacked {
			continue
		}
		if offset, err := entryInt(leaf.Entry, "offset"); err == nil {
			oldOffsets[leaf.Path] = offset
		}
	}

	// Apply changed file sizes and integrity.
	for path, data := range changed {
		entry := findInHeader(header, path)
		if entry == nil {
			return "", fmt.Errorf("asar 中不存在条目：%s", path)
		}
		entry["size"] = len(data)
		entry["integrity"] = sumIntegrity(data)
	}

	// Lay out packed files one after another.
	newOffsets := map[string]int64{}
	var offset int64
	for _, leaf := range leaves {
		if unpacked, _ := leaf.Entry["unpacked"].(bool); unpacked {
			delete(leaf.Entry, "offset")
			continue
		}
		leaf.Entry["offset"] = fmt.Sprintf("%d", offset)
		newOffsets[leaf.Path] = offset
		size, err := entryInt(leaf.Entry, "size")
		if err != nil {
			return "", err
		}
		offset += size
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	pad := (4 - len(headerBytes)%4) % 4
	payloadSize := 4 + len(headerBytes) + pad
	pickleSize := 4 + payloadSize

	src, err := os.Open(archive.path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		out.Close()
		if !success {
			os.Remove(outPath)
		}
	}()

	fixed := make([]byte, 16)
	binary.LittleEndian.PutUint32(fixed[0:4], 4)
	binary.LittleEndian.PutUint32(fixed[4:8], uint32(pickleSize))
	binary.LittleEndian.PutUint32(fixed[8:12], uint32(payloadSize))
	binary.LittleEndian.PutUint32(fixed[12:16], uint32(len(headerBytes)))
	if _, err := out.Write(fixed); err != nil {
		return "", err
	}
	if _, err := out.Write(headerBytes); err != nil {
		return "", err
	}
	if pad > 0 {
		if _, err := out.Write(make([]byte, pad)); err != nil {
			return "", err
		}
	}

	dataPos := int64(16 + len(headerBytes) + pad)
	for _, leaf := range leaves {
		if unpacked, _ := leaf.Entry["unpacked"].(bool); unpacked {
			continue
		}
		size, err := entryInt(leaf.Entry, "size")
		if err != nil {
			return "", err
		}
		if data, ok := changed[leaf.Path]; ok {
			if _, err := out.Write(data); err != nil {
				return "", err
			}
		} else {
			oldOffset, ok := oldOffsets[leaf.Path]
			if !ok {
				return "", fmt.Errorf("找不到原始 offset：%s", leaf.Path)
			}
			if _, err := src.Seek(archive.dataStart+oldOffset, io.SeekStart); err != nil {
				return "", err
			}
			if _, err := io.CopyN(out, src, size); err != nil {
				return "", fmt.Errorf("复制 %s 失败：%v", leaf.Path, err)
			}
		}
		pos, err := out.Seek(0, io.SeekCurrent)
		if err != nil {
			return "", err
		}
		want := dataPos + newOffsets[leaf.Path] + size
		if pos != want {
			return "", fmt.Errorf("写入偏移不一致：%s（%d != %d）", leaf.Path, pos, want)
		}
	}

	if err := out.Sync(); err != nil {
		return "", err
	}
	success = true
	sum := sha256.Sum256(headerBytes)
	return hex.EncodeToString(sum[:]), nil
}

func findInHeader(header map[string]any, asarPath string) map[string]any {
	node := header
	parts := strings.Split(strings.Trim(asarPath, "/"), "/")
	for i, part := range parts {
		files, ok := node["files"].(map[string]any)
		if !ok {
			return nil
		}
		next, ok := files[part].(map[string]any)
		if !ok {
			return nil
		}
		node = next
		if i < len(parts)-1 {
			if _, ok := node["files"]; !ok {
				return nil
			}
		}
	}
	if _, isDir := node["files"]; isDir {
		return nil
	}
	return node
}

// --- exe integrity resource ---------------------------------------------------

// findExeIntegrity returns the byte offset of the asar header hash inside the
// PE resource INTEGRITY/ELECTRONASAR and its current value.
func findExeIntegrity(exePath string) (int64, string, error) {
	data, err := os.ReadFile(exePath)
	if err != nil {
		return 0, "", err
	}
	searchFrom := 0
	for {
		idx := bytes.Index(data[searchFrom:], []byte(integrityMark))
		if idx < 0 {
			return 0, "", nil
		}
		pos := searchFrom + idx
		start := pos + len(integrityMark)
		if start+64 <= len(data) {
			value := data[start : start+64]
			if isHex(value) {
				from := pos - 300
				if from < 0 {
					from = 0
				}
				if bytes.Contains(data[from:pos], []byte("app.asar")) {
					return int64(start), string(value), nil
				}
			}
		}
		searchFrom = pos + len(integrityMark)
	}
}

func isHex(b []byte) bool {
	for _, c := range b {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return len(b) == 64
}

func updateExeIntegrity(exePath, newHash string) error {
	offset, old, err := findExeIntegrity(exePath)
	if err != nil {
		return err
	}
	if offset == 0 {
		return fmt.Errorf("exe 中没有找到 asar 完整性资源（可能未启用 fuse）")
	}
	if old == newHash {
		return nil
	}
	f, err := os.OpenFile(exePath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt([]byte(newHash), offset); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	_, check, err := findExeIntegrity(exePath)
	if err != nil {
		return err
	}
	if check != newHash {
		return fmt.Errorf("写入 exe 完整性校验值后回读不一致")
	}
	return nil
}

// --- install / restore --------------------------------------------------------

type installStatus struct {
	Installed bool
	Version   string
	HasBackup bool
}

func gamePaths(gameDir string) (asar, backup, exe string) {
	asar = filepath.Join(gameDir, "resources", "app.asar")
	backup = asar + ".bak"
	exe = filepath.Join(gameDir, exeName)
	return
}

func inspectInstall(gameDir string) installStatus {
	var st installStatus
	asarPath, backup, _ := gamePaths(gameDir)
	if _, err := os.Stat(backup); err == nil {
		st.HasBackup = true
	}
	archive, err := openAsar(asarPath)
	if err != nil {
		return st
	}
	index, err := archive.readFile(asarIndexPath)
	if err == nil && strings.Contains(string(index), asarMarker) {
		st.Installed = true
	}
	if cheat, err := archive.readFile(asarCheatPath); err == nil {
		st.Version = cheatVersionFromSource(string(cheat))
	}
	return st
}

func gameRunning() bool {
	out, err := hiddenCommand("tasklist", "/FI", "IMAGENAME eq "+exeName)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), strings.ToLower(exeName))
}

func installOffline(gameDir, cheat string, dryRun, force bool, log func(string, ...any)) error {
	asarPath, backup, exePath := gamePaths(gameDir)
	if !isGameDir(gameDir) {
		return fmt.Errorf("不是有效的游戏目录：%s", gameDir)
	}
	if gameRunning() && !force {
		return fmt.Errorf("游戏正在运行，请先完全退出游戏再安装")
	}

	archive, err := openAsar(asarPath)
	if err != nil {
		return err
	}
	index, err := archive.readFile(asarIndexPath)
	if err != nil {
		return err
	}
	newIndex := index
	if !strings.Contains(string(index), asarMarker) {
		tag := "\n  <script src=\"cheat.js\" charset=\"utf-8\"></script>\n"
		if !strings.Contains(string(index), "</body>") {
			return fmt.Errorf("index.html 中找不到 </body>")
		}
		newIndex = []byte(strings.Replace(string(index), "</body>", asarMarker+tag+"</body>", 1))
	}

	changed := map[string][]byte{
		asarIndexPath: newIndex,
		asarCheatPath: []byte(cheat),
	}
	if archive.find(asarCheatPath) == nil {
		rootFiles, _ := archive.header["files"].(map[string]any)
		webMobile, _ := rootFiles["web-mobile"].(map[string]any)
		files, _ := webMobile["files"].(map[string]any)
		if files == nil {
			return fmt.Errorf("app.asar 中没有 web-mobile 目录")
		}
		files["cheat.js"] = map[string]any{"size": len(cheat), "integrity": sumIntegrity([]byte(cheat))}
		log("  已新建 web-mobile/cheat.js 条目")
	}

	if dryRun {
		log("  [dry-run] 校验通过，没有写入文件")
		return nil
	}

	if _, err := os.Stat(backup); err != nil {
		log("  正在备份 app.asar -> app.asar.bak ...")
		if err := copyFile(asarPath, backup, 0o644); err != nil {
			return fmt.Errorf("备份失败：%v", err)
		}
	} else {
		log("  备份已存在，保留最早的原始包")
	}

	tmp := asarPath + ".tmp"
	log("  正在重打包 asar ...")
	newHash, err := writeRepackedAsar(archive, changed, tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}

	check, err := openAsar(tmp)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("回读新 asar 失败：%v", err)
	}
	if got, _ := check.readFile(asarCheatPath); string(got) != cheat {
		os.Remove(tmp)
		return fmt.Errorf("回读校验失败：cheat.js 内容不一致")
	}
	if got, _ := check.readFile(asarIndexPath); !strings.Contains(string(got), asarMarker) {
		os.Remove(tmp)
		return fmt.Errorf("回读校验失败：index.html 中没有注入标记")
	}

	// Check the fuse resource before replacing anything, so a failed install
	// can never leave the game with an out-of-sync executable.
	integrityOffset, _, _ := findExeIntegrity(exePath)
	hasIntegrity := integrityOffset != 0

	if err := os.Rename(tmp, asarPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换 app.asar 失败：%v", err)
	}
	log("  已写入 %s", asarPath)

	if hasIntegrity {
		if err := updateExeIntegrity(exePath, newHash); err != nil {
			// Roll back to the backup so the game keeps working.
			_ = copyFile(backup, asarPath, 0o644)
			return fmt.Errorf("同步 exe 完整性校验值失败，已回滚：%v", err)
		}
		log("  已同步 exe 的 asar 完整性校验值")
	} else {
		log("  [提示] 未检测到 exe 的 asar 完整性资源，跳过同步")
	}
	return nil
}

func uninstallOffline(gameDir string, force bool, log func(string, ...any)) error {
	asarPath, backup, exePath := gamePaths(gameDir)
	if !isGameDir(gameDir) {
		return fmt.Errorf("不是有效的游戏目录：%s", gameDir)
	}
	if gameRunning() && !force {
		return fmt.Errorf("游戏正在运行，请先完全退出游戏再还原")
	}
	if _, err := os.Stat(backup); err != nil {
		return fmt.Errorf("找不到备份文件：%s", backup)
	}
	if err := copyFile(backup, asarPath, 0o644); err != nil {
		return err
	}
	restored, err := openAsar(asarPath)
	if err != nil {
		return err
	}
	if err := updateExeIntegrity(exePath, restored.headerHash()); err != nil {
		return fmt.Errorf("还原后同步 exe 校验值失败：%v", err)
	}
	log("  已用 app.asar.bak 还原官方 app.asar")
	log("  已同步 exe 的 asar 完整性校验值")
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		// Windows and some filesystems ignore chmod; not fatal.
		_ = err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
