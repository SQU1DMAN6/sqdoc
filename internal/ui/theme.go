package ui

import "image/color"

type Theme struct {
	AppBackground   color.RGBA
	TopBar          color.RGBA
	Toolbar         color.RGBA
	Canvas          color.RGBA
	Page            color.RGBA
	Border          color.RGBA
	StatusBar       color.RGBA
	Accent          color.RGBA
	Shadow          color.RGBA
	MenuHeightDp    int
	ToolbarHeightDp int
	StatusHeightDp  int
	PageMarginDp    int
}

func DefaultTheme() Theme {
	return Theme{
		AppBackground:   color.RGBA{0xF3, 0xF5, 0xF8, 0xFF},
		TopBar:          color.RGBA{0x2B, 0x57, 0x9A, 0xFF},
		Toolbar:         color.RGBA{0xF7, 0xF9, 0xFC, 0xFF},
		Canvas:          color.RGBA{0xE2, 0xE7, 0xEF, 0xFF},
		Page:            color.RGBA{0xFF, 0xFF, 0xFF, 0xFF},
		Border:          color.RGBA{0xB2, 0xBF, 0xD0, 0xFF},
		StatusBar:       color.RGBA{0xEA, 0xEF, 0xF6, 0xFF},
		Accent:          color.RGBA{0x2B, 0x57, 0x9A, 0xFF},
		Shadow:          color.RGBA{0xC8, 0xCF, 0xDB, 0xFF},
		MenuHeightDp:    34,
		ToolbarHeightDp: 42,
		StatusHeightDp:  28,
		PageMarginDp:    24,
	}
}
