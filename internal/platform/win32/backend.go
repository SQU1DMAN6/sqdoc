package win32

import (
	"sqdoc/internal/platform"
	"sqdoc/internal/render"
)

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return "win32" }

func (b *Backend) CreateWindow(cfg platform.WindowConfig) (platform.Window, error) {
	return &window{
		title: cfg.Title,
		w:     cfg.WidthPx,
		h:     cfg.HeightPx,
		scale: 1.0,
	}, nil
}

type window struct {
	title  string
	w      int
	h      int
	scale  float32
	closed bool
}

func (w *window) PollEvents() []platform.Event {
	if w.closed {
		return []platform.Event{{Type: platform.EventClose}}
	}
	return nil
}

func (w *window) SizePx() (int, int) { return w.w, w.h }
func (w *window) Scale() float32     { return w.scale }
func (w *window) SetTitle(title string) {
	w.title = title
}

func (w *window) Present(_ *render.FrameBuffer) error { return nil }
func (w *window) Close()                              { w.closed = true }
