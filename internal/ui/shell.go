package ui

import (
	"sqdoc/internal/editor"
	"sqdoc/internal/render"
)

type Layout struct {
	MenuH     int
	ToolbarH  int
	StatusH   int
	CanvasY   int
	CanvasH   int
	PageX     int
	PageY     int
	PageW     int
	PageH     int
	ContentX  int
	ContentY  int
	ContentW  int
	ContentH  int
	StatusBar int
}

func ComputeLayout(w, h int, theme Theme, scale float32) Layout {
	if scale <= 0 {
		scale = 1
	}

	dp := func(v int) int { return int(float32(v) * scale) }

	menuH := dp(theme.MenuHeightDp)
	toolbarH := dp(theme.ToolbarHeightDp)
	statusH := dp(theme.StatusHeightDp)
	margin := dp(theme.PageMarginDp)

	canvasY := menuH + toolbarH
	canvasH := h - canvasY - statusH
	if canvasH < 0 {
		canvasH = 0
	}

	pageW := w - margin*2
	pageH := canvasH - margin*2
	maxPageW := dp(900)
	if pageW > maxPageW {
		pageW = maxPageW
	}
	if pageW < dp(320) {
		pageW = dp(320)
	}
	if pageH < dp(200) {
		pageH = dp(200)
	}
	pageX := (w - pageW) / 2
	pageY := canvasY + margin
	contentPad := dp(18)

	contentW := pageW - contentPad*2
	contentH := pageH - contentPad*2 - dp(4)
	if contentW < dp(100) {
		contentW = dp(100)
	}
	if contentH < dp(100) {
		contentH = dp(100)
	}

	return Layout{
		MenuH:     menuH,
		ToolbarH:  toolbarH,
		StatusH:   statusH,
		CanvasY:   canvasY,
		CanvasH:   canvasH,
		PageX:     pageX,
		PageY:     pageY,
		PageW:     pageW,
		PageH:     pageH,
		ContentX:  pageX + contentPad,
		ContentY:  pageY + contentPad + dp(8),
		ContentW:  contentW,
		ContentH:  contentH,
		StatusBar: h - statusH,
	}
}

func DrawShell(fb *render.FrameBuffer, state *editor.State, theme Theme, scale float32) Layout {
	layout := ComputeLayout(fb.W, fb.H, theme, scale)

	fb.Clear(theme.AppBackground)

	// Menu + toolbar
	fb.FillRect(0, 0, fb.W, layout.MenuH, theme.TopBar)
	fb.FillRect(0, layout.MenuH, fb.W, layout.ToolbarH, theme.Toolbar)
	fb.StrokeRect(0, 0, fb.W, layout.MenuH+layout.ToolbarH, 1, theme.Border)

	// Canvas region
	fb.FillRect(0, layout.CanvasY, fb.W, layout.CanvasH, theme.Canvas)

	// Centered page
	pageX := layout.PageX
	pageY := layout.PageY
	pageW := layout.PageW
	pageH := layout.PageH
	fb.FillRect(pageX+2, pageY+2, pageW, pageH, theme.Shadow)
	fb.FillRect(pageX, pageY, pageW, pageH, theme.Page)
	fb.StrokeRect(pageX, pageY, pageW, pageH, 1, theme.Border)

	// Accent line at top of page as visual anchor.
	accentH := int(3 * scale)
	if accentH < 1 {
		accentH = 1
	}
	fb.FillRect(pageX, pageY, pageW, accentH, theme.Accent)

	// Status bar
	fb.FillRect(0, layout.StatusBar, fb.W, layout.StatusH, theme.StatusBar)
	fb.StrokeRect(0, layout.StatusBar, fb.W, layout.StatusH, 1, theme.Border)

	_ = state
	return layout
}
