package ui

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"

	"gioui.org/io/pointer"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	xdraw "golang.org/x/image/draw"
)

// coffeePNG is the official Buy Me a Coffee button
// (cdn.buymeacoffee.com/buttons/v2/default-yellow.png).
//
//go:embed coffee.png
var coffeePNG []byte

// coffeeButton draws the Buy Me a Coffee button at a given height. The image
// is resampled to the exact size on screen, since the GPU would only scale
// it with a plain linear filter, which looks rough at a third of its size.
type coffeeButton struct {
	click widget.Clickable
	src   image.Image
	op    paint.ImageOp
	size  image.Point
}

func (b *coffeeButton) Layout(gtx C, height unit.Dp) D {
	if b.src == nil {
		img, err := png.Decode(bytes.NewReader(coffeePNG))
		if err != nil {
			panic(err) // the embedded image is always valid
		}
		b.src = img
	}
	sb := b.src.Bounds()
	h := gtx.Dp(height)
	size := image.Pt(h*sb.Dx()/sb.Dy(), h)
	if size != b.size {
		dst := image.NewRGBA(image.Rectangle{Max: size})
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), b.src, sb, xdraw.Src, nil)
		b.op, b.size = paint.NewImageOp(dst), size
	}
	return b.click.Layout(gtx, func(gtx C) D {
		pointer.CursorPointer.Add(gtx.Ops)
		if b.click.Hovered() {
			defer paint.PushOpacity(gtx.Ops, 0.85).Pop()
		}
		return widget.Image{Src: b.op, Scale: 1 / gtx.Metric.PxPerDp}.Layout(gtx)
	})
}
