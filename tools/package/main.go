// Command package builds the portable release zip from the files in dist and
// the documentation in the repository root. It does not compile anything: run
// build.bat (or go build) first.
//
//	go run tools/package/main.go
//	go run tools/package/main.go -version 1.0.0 -out dist/custom.zip
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const folder = "ImNoDemonlord-LocalCheat"

func main() {
	version := flag.String("version", "", "release version (default: read from src/cheat.js)")
	out := flag.String("out", "", "output zip (default: dist/ImNoDemonlord-LocalCheat-v<version>.zip)")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if *version == "" {
		*version, err = cheatVersion(filepath.Join(root, "src", "cheat.js"))
		if err != nil {
			fail(err)
		}
	}
	if *out == "" {
		*out = filepath.Join(root, "dist", "ImNoDemonlord-LocalCheat-v"+*version+".zip")
	}

	exe := filepath.Join(root, "dist", "ImNoDemonlord-ModLoader.exe")
	if _, err := os.Stat(exe); err != nil {
		fail(fmt.Errorf("%s is missing: run build.bat first", exe))
	}

	type releaseFile struct {
		path string
		name string
	}

	files := []releaseFile{
		{exe, "ImNoDemonlord-ModLoader.exe"},
		{filepath.Join(root, "install.bat"), "install.bat"},
		{filepath.Join(root, "uninstall.bat"), "uninstall.bat"},
		{filepath.Join(root, "start-loader.bat"), "start-loader.bat"},
		{filepath.Join(root, "README.md"), "README.md"},
		{filepath.Join(root, "CHANGELOG.md"), "CHANGELOG.md"},
		{filepath.Join(root, "LICENSE"), "LICENSE"},
	}
	docs, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		fail(err)
	}
	for _, path := range docs {
		files = append(files, releaseFile{path, "docs/" + filepath.Base(path)})
	}
	images, err := filepath.Glob(filepath.Join(root, "docs", "images", "*"))
	if err != nil {
		fail(err)
	}
	for _, path := range images {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			files = append(files, releaseFile{path, "docs/images/" + filepath.Base(path)})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fail(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for _, item := range files {
		if err := add(zw, item.path, folder+"/"+item.name); err != nil {
			fail(err)
		}
	}
	if err := zw.Close(); err != nil {
		fail(err)
	}
	st, err := os.Stat(*out)
	if err != nil {
		fail(err)
	}
	fmt.Printf("packaged %d files: %s (%.1f MB)\n", len(files), *out, float64(st.Size())/(1<<20))
}

// add writes path into the zip under name.
func add(zw *zip.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		header.Method = zip.Store
	} else {
		header.Method = zip.Deflate
	}

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(w, src)
	return err
}

var versionPattern = regexp.MustCompile(`var VERSION = '([^']+)'`)

func cheatVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := versionPattern.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("no version in %s", path)
	}
	return string(m[1]), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "package:", err)
	os.Exit(1)
}
