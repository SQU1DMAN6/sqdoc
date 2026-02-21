package render

import "image/color"

type FrameBuffer struct {
	W      int
	H      int
	Pixels []uint8 // RGBA
}

func NewFrameBuffer(w, h int) *FrameBuffer {
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return &FrameBuffer{W: w, H: h, Pixels: make([]uint8, w*h*4)}
}

func (fb *FrameBuffer) Clear(c color.RGBA) {
	for i := 0; i < len(fb.Pixels); i += 4 {
		fb.Pixels[i+0] = c.R
		fb.Pixels[i+1] = c.G
		fb.Pixels[i+2] = c.B
		fb.Pixels[i+3] = c.A
	}
}

func (fb *FrameBuffer) FillRect(x, y, w, h int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > fb.W {
		w = fb.W - x
	}
	if y+h > fb.H {
		h = fb.H - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	for row := 0; row < h; row++ {
		off := ((y+row)*fb.W + x) * 4
		for col := 0; col < w; col++ {
			idx := off + col*4
			fb.Pixels[idx+0] = c.R
			fb.Pixels[idx+1] = c.G
			fb.Pixels[idx+2] = c.B
			fb.Pixels[idx+3] = c.A
		}
	}
}

func (fb *FrameBuffer) StrokeRect(x, y, w, h, line int, c color.RGBA) {
	if line <= 0 {
		line = 1
	}
	fb.FillRect(x, y, w, line, c)
	fb.FillRect(x, y+h-line, w, line, c)
	fb.FillRect(x, y, line, h, c)
	fb.FillRect(x+w-line, y, line, h, c)
}
