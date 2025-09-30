// Copyright (c) 2025 TalentEditor
//
// TalentEditor is licensed under the MIT License.
// See the LICENSE file for details.

package main

import (
    "fyne.io/fyne/v2"
    "image/color"
)

type customTheme struct {
    base    fyne.Theme
    variant fyne.ThemeVariant
}

func (t *customTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
    return t.base.Color(name, t.variant)
}

func (t *customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
    return t.base.Icon(name)
}

func (t *customTheme) Font(style fyne.TextStyle) fyne.Resource {
    return t.base.Font(style)
}

func (t *customTheme) Size(name fyne.ThemeSizeName) float32 {
    return t.base.Size(name)
}