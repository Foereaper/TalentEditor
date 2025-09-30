package main

import "fyne.io/fyne/v2"

type grid4x15 struct {
    Rows          int
    Cols          int
    HorizontalPad float32
    VerticalPad   float32
}

func (g *grid4x15) MinSize(objects []fyne.CanvasObject) fyne.Size {
    colWidths := make([]float32, g.Cols)
    rowHeights := make([]float32, g.Rows)

    for i, obj := range objects {
        row := i / g.Cols
        col := i % g.Cols
        size := obj.MinSize()
        if col < g.Cols && size.Width > colWidths[col] {
            colWidths[col] = size.Width
        }
        if row < g.Rows && size.Height > rowHeights[row] {
            rowHeights[row] = size.Height
        }
    }

    var totalWidth, totalHeight float32
    for _, w := range colWidths {
        totalWidth += w
    }
    for _, h := range rowHeights {
        totalHeight += h
    }

    // Add padding spaces between cells
    totalWidth += float32(g.Cols-1) * g.HorizontalPad
    totalHeight += float32(g.Rows-1) * g.VerticalPad

    if totalWidth == 0 {
        totalWidth = float32(g.Cols)*50 + float32(g.Cols-1)*g.HorizontalPad
    }
    if totalHeight == 0 {
        totalHeight = float32(g.Rows)*50 + float32(g.Rows-1)*g.VerticalPad
    }

    return fyne.NewSize(totalWidth, totalHeight)
}

func (g *grid4x15) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
    colWidths := make([]float32, g.Cols)
    rowHeights := make([]float32, g.Rows)

    // First pass: compute max width/height per col/row
    for i, obj := range objects {
        row := i / g.Cols
        col := i % g.Cols
        size := obj.MinSize()
        if col < g.Cols && size.Width > colWidths[col] {
            colWidths[col] = size.Width
        }
        if row < g.Rows && size.Height > rowHeights[row] {
            rowHeights[row] = size.Height
        }
    }

    // Compute cumulative offsets with padding
    colOffsets := make([]float32, g.Cols)
    rowOffsets := make([]float32, g.Rows)
    for c := 1; c < g.Cols; c++ {
        colOffsets[c] = colOffsets[c-1] + colWidths[c-1] + g.HorizontalPad
    }
    for r := 1; r < g.Rows; r++ {
        rowOffsets[r] = rowOffsets[r-1] + rowHeights[r-1] + g.VerticalPad
    }

    // Place objects
    for i, obj := range objects {
        row := i / g.Cols
        col := i % g.Cols
        if row < g.Rows && col < g.Cols {
            obj.Move(fyne.NewPos(colOffsets[col], rowOffsets[row]))
            obj.Resize(fyne.NewSize(colWidths[col], rowHeights[row]))
        }
    }
}