package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var (
	bgDark   = color.NRGBA{R: 0x08, G: 0x20, B: 0x03, A: 0xff}
	bgLight  = color.NRGBA{R: 0x0d, G: 0x33, B: 0x08, A: 0xff}
	green    = color.NRGBA{R: 0x2c, G: 0xdb, B: 0x16, A: 0xff}
	greenDim = color.NRGBA{R: 0x24, G: 0xb4, B: 0x12, A: 0xff}
)

func main() {
	outDir := os.Args[1]
	master := render(1024)
	sizes := []int{1024, 512, 256, 128, 64, 32, 16}
	for _, s := range sizes {
		img := master
		if s != 1024 {
			img = scale(master, s)
		}
		if err := writePNG(filepath.Join(outDir, fmt.Sprintf("icon_%03d.png", s)), img); err != nil {
			panic(err)
		}
	}
	var buf bytes.Buffer
	if err := writeICO(&buf, master, []int{256, 128, 64, 48, 32, 16}); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "icon.ico"), buf.Bytes(), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("icons written to", outDir)
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) * 0.22
	drawRoundedRect(img, radius)
	cx, cy := float64(size)*0.5, float64(size)*0.5
	rOuter := float64(size) * 0.30
	rInner := float64(size) * 0.235
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d2 := dx*dx + dy*dy
			switch {
			case d2 <= rInner*rInner:
				img.SetNRGBA(x, y, green)
			case d2 <= rOuter*rOuter:
				img.SetNRGBA(x, y, greenDim)
			}
		}
	}
	return img
}

func drawRoundedRect(img *image.NRGBA, radius float64) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if insideRoundedRect(float64(x), float64(y), float64(w), float64(h), radius) {
				base := bgDark
				if (x/8+y/8)%2 == 0 {
					base = bgLight
				}
				img.SetNRGBA(x, y, base)
			}
		}
	}
}

func insideRoundedRect(px, py, w, h, r float64) bool {
	if px < 0 || py < 0 || px >= w || py >= h {
		return false
	}
	nx := px
	ny := py
	if nx < r {
		nx = r
	} else if nx > w-r-1 {
		nx = w - r - 1
	}
	if ny < r {
		ny = r
	} else if ny > h-r-1 {
		ny = h - r - 1
	}
	dx, dy := px-nx, py-ny
	return dx*dx+dy*dy <= r*r
}

func scale(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	sx := float64(sb.Dx()) / float64(size)
	sy := float64(sb.Dy()) / float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			srcX := int((float64(x) + 0.5) * sx)
			srcY := int((float64(y) + 0.5) * sy)
			dst.SetNRGBA(x, y, src.NRGBAAt(srcX, srcY))
		}
	}
	return dst
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

func writeICO(out *bytes.Buffer, master *image.NRGBA, sizes []int) error {
	_ = binary.Write(out, binary.LittleEndian, uint16(0))
	_ = binary.Write(out, binary.LittleEndian, uint16(1))
	_ = binary.Write(out, binary.LittleEndian, uint16(len(sizes)))

	type entry struct {
		data []byte
		size int
	}
	entries := make([]entry, len(sizes))
	for i, s := range sizes {
		img := master
		if s != master.Bounds().Dx() {
			img = scale(master, s)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		entries[i] = entry{data: buf.Bytes(), size: s}
	}
	offset := 6 + 16*len(sizes)
	for _, e := range entries {
		_ = binary.Write(out, binary.LittleEndian, byte(e.size%256))
		_ = binary.Write(out, binary.LittleEndian, byte(0))
		_ = binary.Write(out, binary.LittleEndian, byte(0))
		_ = binary.Write(out, binary.LittleEndian, byte(0))
		_ = binary.Write(out, binary.LittleEndian, uint16(1))
		_ = binary.Write(out, binary.LittleEndian, uint16(32))
		_ = binary.Write(out, binary.LittleEndian, uint32(len(e.data)))
		_ = binary.Write(out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	return nil
}
