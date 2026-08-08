// AI GENERATED - Tom Barone
package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// protonTheme is the default theme with Proton's purple as the primary
// color, which paints the main action buttons and the progress bar fill.
type protonTheme struct {
	fyne.Theme
}

func (t *protonTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x6D, G: 0x4A, B: 0xFF, A: 0xFF}
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	}
	return t.Theme.Color(n, v)
}
