package platform

import "sqdoc/internal/render"

type WindowConfig struct {
	Title       string
	WidthPx     int
	HeightPx    int
	MinWidthPx  int
	MinHeightPx int
}

type EventType int

const (
	EventUnknown EventType = iota
	EventClose
	EventResize
	EventDPIChanged
	EventKeyDown
	EventKeyUp
	EventTextInput
	EventMouseMove
	EventMouseDown
	EventMouseUp
	EventMouseWheel
)

type Event struct {
	Type   EventType
	Width  int
	Height int
	Scale  float32
	Rune   rune
	DeltaX int
	DeltaY int
	X      int
	Y      int
	Key    string
}

type Platform interface {
	Name() string
	CreateWindow(cfg WindowConfig) (Window, error)
}

type Window interface {
	PollEvents() []Event
	SizePx() (int, int)
	Scale() float32
	Present(fb *render.FrameBuffer) error
	SetTitle(title string)
	Close()
}
