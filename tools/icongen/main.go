// Command icongen draws the loader's application icon: a small demon head on
// a dark rounded plate. It writes a PNG for the window icon, and a multi-size
// ICO for the executable's resources.
//
//	go run tools/icongen/main.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const ss = 4 // supersampling factor

type canvas struct {
	w, h int
	pix  []color.RGBA
}

func newCanvas(size int) *canvas {
	return &canvas{w: size, h: size, pix: make([]color.RGBA, size*size)}
}

func (c *canvas) blend(x, y int, col color.RGBA) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h || col.A == 0 {
		return
	}
	dst := c.pix[y*c.w+x]
	a := float64(col.A) / 255
	c.pix[y*c.w+x] = color.RGBA{
		R: uint8(float64(col.R)*a + float64(dst.R)*(1-a) + 0.5),
		G: uint8(float64(col.G)*a + float64(dst.G)*(1-a) + 0.5),
		B: uint8(float64(col.B)*a + float64(dst.B)*(1-a) + 0.5),
		A: uint8(float64(col.A) + float64(dst.A)*(1-a) + 0.5),
	}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: f(a.A, b.A)}
}

func (c *canvas) roundedRect(x0, y0, x1, y1, r float64, col func(x, y float64) color.RGBA) {
	ix0, iy0 := int(math.Floor(x0)), int(math.Floor(y0))
	ix1, iy1 := int(math.Ceil(x1)), int(math.Ceil(y1))
	for y := iy0; y < iy1; y++ {
		for x := ix0; x < ix1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			dx, dy := 0.0, 0.0
			if px < x0+r {
				dx = x0 + r - px
			} else if px > x1-r {
				dx = px - (x1 - r)
			}
			if py < y0+r {
				dy = y0 + r - py
			} else if py > y1-r {
				dy = py - (y1 - r)
			}
			inside := px >= x0 && px <= x1 && py >= y0 && py <= y1
			if dx > 0 && dy > 0 {
				inside = dx*dx+dy*dy <= r*r
			}
			if inside {
				c.blend(x, y, col(px, py))
			}
		}
	}
}

func (c *canvas) ring(x0, y0, x1, y1, r, width float64, col color.RGBA) {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2, (y1-y0)/2
	for y := int(y0); y <= int(y1)+1; y++ {
		for x := int(x0); x <= int(x1)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			qx := math.Abs(px-cx) - (hx - r)
			qy := math.Abs(py-cy) - (hy - r)
			outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
			if outside <= 0 && outside >= -width {
				c.blend(x, y, col)
			}
		}
	}
}

func (c *canvas) ellipse(cx, cy, rx, ry float64, col func(x, y float64) color.RGBA) {
	for y := int(cy - ry); y <= int(cy+ry)+1; y++ {
		for x := int(cx - rx); x <= int(cx+rx)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			nx, ny := (px-cx)/rx, (py-cy)/ry
			if nx*nx+ny*ny <= 1 {
				c.blend(x, y, col(px, py))
			}
		}
	}
}

func (c *canvas) polygon(pts [][2]float64, col color.RGBA) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
		maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	for y := int(minY); y <= int(maxY)+1; y++ {
		for x := int(minX); x <= int(maxX)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			inside := false
			for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
				xi, yi := pts[i][0], pts[i][1]
				xj, yj := pts[j][0], pts[j][1]
				if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
					inside = !inside
				}
			}
			if inside {
				c.blend(x, y, col)
			}
		}
	}
}

// render draws the icon at an integer size, supersampled and box filtered.
func render(size int) *image.NRGBA {
	n := float64(size * ss)
	c := newCanvas(size * ss)

	plateTop := color.RGBA{61, 41, 28, 255}
	plateBottom := color.RGBA{22, 15, 11, 255}
	c.roundedRect(0, 0, n, n, 0.22*n, func(_, y float64) color.RGBA {
		return mix(plateTop, plateBottom, y/n)
	})

	// A warm rim keeps the plate visible on dark taskbars.
	c.ring(0.018*n, 0.018*n, 0.982*n, 0.982*n, 0.205*n, 0.012*n, color.RGBA{255, 178, 71, 150})

	headTop := color.RGBA{255, 219, 160, 255}
	headBottom := color.RGBA{255, 145, 58, 255}
	headFill := func(_, y float64) color.RGBA {
		t := (y/n - 0.38) / 0.42
		return mix(headTop, headBottom, math.Max(0, math.Min(1, t)))
	}
	dark := color.RGBA{38, 22, 13, 255}

	// Horns first, so the head covers their roots.
	c.polygon([][2]float64{
		{0.335 * n, 0.455 * n}, {0.145 * n, 0.145 * n}, {0.455 * n, 0.315 * n},
	}, color.RGBA{255, 178, 71, 255})
	c.polygon([][2]float64{
		{0.665 * n, 0.455 * n}, {0.855 * n, 0.145 * n}, {0.545 * n, 0.315 * n},
	}, color.RGBA{255, 178, 71, 255})

	c.ellipse(0.5*n, 0.615*n, 0.262*n, 0.255*n, headFill)

	// Eyes.
	c.ellipse(0.400*n, 0.575*n, 0.070*n, 0.042*n, func(_, _ float64) color.RGBA { return dark })
	c.ellipse(0.600*n, 0.575*n, 0.070*n, 0.042*n, func(_, _ float64) color.RGBA { return dark })

	// Grin with two fangs.
	c.roundedRect(0.425*n, 0.675*n, 0.575*n, 0.720*n, 0.022*n, func(_, _ float64) color.RGBA { return dark })
	fang := color.RGBA{255, 205, 140, 255}
	c.polygon([][2]float64{
		{0.455 * n, 0.700 * n}, {0.485 * n, 0.700 * n}, {0.470 * n, 0.762 * n},
	}, fang)
	c.polygon([][2]float64{
		{0.515 * n, 0.700 * n}, {0.545 * n, 0.700 * n}, {0.530 * n, 0.762 * n},
	}, fang)

	// Box filter down to the requested size.
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	area := float64(ss * ss)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					p := c.pix[(y*ss+dy)*c.w+(x*ss+dx)]
					r += float64(p.R) * float64(p.A)
					g += float64(p.G) * float64(p.A)
					b += float64(p.B) * float64(p.A)
					a += float64(p.A)
				}
			}
			col := color.NRGBA{}
			if a > 0 {
				col = color.NRGBA{
					R: uint8(r/a + 0.5),
					G: uint8(g/a + 0.5),
					B: uint8(b/a + 0.5),
					A: uint8(a/area + 0.5),
				}
			}
			out.SetNRGBA(x, y, col)
		}
	}
	return out
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// encodeICO packs PNG images into a Vista-style icon file.
func encodeICO(images []image.Image) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // icon
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(images)))

	type entry struct {
		size   int
		offset int
	}
	entries := make([]entry, len(images))
	data := make([][]byte, len(images))
	offset := 6 + 16*len(images)
	for i, img := range images {
		data[i] = encodePNG(img)
		entries[i] = entry{size: len(data[i]), offset: offset}
		offset += len(data[i])
	}
	for i, img := range images {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		wb, hb := byte(w), byte(h)
		if w >= 256 {
			wb = 0
		}
		if h >= 256 {
			hb = 0
		}
		out.Write([]byte{wb, hb, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))  // planes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32)) // bit count
		_ = binary.Write(&out, binary.LittleEndian, uint32(entries[i].size))
		_ = binary.Write(&out, binary.LittleEndian, uint32(entries[i].offset))
	}
	for _, d := range data {
		out.Write(d)
	}
	return out.Bytes()
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	assets := filepath.Join(root, "loader", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		panic(err)
	}

	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	imgs := make([]image.Image, len(sizes))
	for i, s := range sizes {
		imgs[i] = render(s)
	}

	if err := os.WriteFile(filepath.Join(assets, "icon.png"), encodePNG(imgs[len(imgs)-1]), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "icon.ico"), encodeICO(imgs), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote loader/assets/icon.png and loader/assets/icon.ico")
}
