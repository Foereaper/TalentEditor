// Copyright (c) 2025 TalentEditor
//
// TalentEditor is licensed under the MIT License.
// See the LICENSE file for details.

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// TalentButton is a reusable icon button with tooltip support
type TalentButton struct {
	widget.BaseWidget          // Embed BaseWidget so this is a fyne.Widget
	ttwidget.ToolTipWidgetExtend // Embed tooltip support

	Icon     fyne.Resource
	OnTapped func()
	BtnSize  fyne.Size
}

// NewTalentButton constructor
func NewTalentButton(icon fyne.Resource, size fyne.Size, tooltip string, tapped func()) *TalentButton {
	btn := &TalentButton{
		Icon:     icon,
		BtnSize:  size,
		OnTapped: tapped,
	}
	btn.ExtendBaseWidget(btn)
	btn.SetToolTip(tooltip)
	return btn
}

// ExtendBaseWidget ensures tooltip and base widget are initialized
func (b *TalentButton) ExtendBaseWidget(wid fyne.Widget) {
	b.BaseWidget.ExtendBaseWidget(wid)
	b.ExtendToolTipWidget(wid)
}

// CreateRenderer draws the button
func (b *TalentButton) CreateRenderer() fyne.WidgetRenderer {
	img := canvas.NewImageFromResource(b.Icon)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(b.BtnSize)

	return &talentButtonRenderer{
		button:  b,
		image:   img,
		objects: []fyne.CanvasObject{img},
	}
}

type talentButtonRenderer struct {
	button  *TalentButton
	image   *canvas.Image
	objects []fyne.CanvasObject
}

func (r *talentButtonRenderer) Layout(size fyne.Size)        { r.image.Resize(r.button.BtnSize) }
func (r *talentButtonRenderer) MinSize() fyne.Size           { return r.button.BtnSize }
func (r *talentButtonRenderer) Refresh()                     { r.image.Resource = r.button.Icon; r.image.Refresh() }
func (r *talentButtonRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *talentButtonRenderer) Destroy()                     {}

// Tapped triggers the button’s action
func (b *TalentButton) Tapped(*fyne.PointEvent) {
	if b.OnTapped != nil {
		b.OnTapped()
	}
}

// Hover events forwarded to tooltip
func (b *TalentButton) MouseIn(e *desktop.MouseEvent) {
	b.ToolTipWidgetExtend.MouseIn(e)
}

func (b *TalentButton) MouseMoved(e *desktop.MouseEvent) {
	b.ToolTipWidgetExtend.MouseMoved(e)
}

func (b *TalentButton) MouseOut() {
	b.ToolTipWidgetExtend.MouseOut()
}
