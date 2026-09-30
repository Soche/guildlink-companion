package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

// trayIcon draws two linked rings. Windows wants an .ico (which may simply
// wrap a PNG); macOS and Linux take the PNG directly.
func trayIcon() []byte {
	const size = 32
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := color.NRGBA{0x2f, 0x6f, 0xdd, 0xff}
	ring := func(cx, cy, r, w float64) {
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
				d := dx*dx + dy*dy
				if d <= r*r && d >= (r-w)*(r-w) {
					img.Set(x, y, ink)
				}
			}
		}
	}
	ring(11, 16, 9, 3.5)
	ring(21, 16, 9, 3.5)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	if runtime.GOOS != "windows" {
		return buf.Bytes()
	}
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, 1})
	ico.Write([]byte{size, size, 0, 0})
	_ = binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})
	_ = binary.Write(&ico, binary.LittleEndian, []uint32{uint32(buf.Len()), 22})
	ico.Write(buf.Bytes())
	return ico.Bytes()
}
