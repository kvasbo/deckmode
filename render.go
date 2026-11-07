package main

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	colorIdle    = color.RGBA{0x00, 0x00, 0x00, 0xff}
	colorOffline = color.RGBA{0xc6, 0x28, 0x28, 0xff}
	colorText    = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colorMuted   = color.RGBA{0xb0, 0xb0, 0xb0, 0xff}
	colorFaint   = color.RGBA{0x50, 0x50, 0x50, 0xff}
)

const (
	alphaFull    = 0xff
	alphaIdle    = 0xb0 // icons of the modes not showing
	alphaOffline = 0x40
)

var labelFont = func() *opentype.Font {
	f, err := opentype.Parse(gomedium.TTF)
	if err != nil {
		panic(err) // embedded, so it can't happen at run time without tests failing first
	}
	return f
}()

// renderKey draws a key: the icon above its label, or whichever of them there
// is, big. The active mode's key is the highlight colour with a white edge.
// Without a broker every key goes faint with a red OFFLINE band.
func renderKey(v KeyView, rect image.Rectangle, highlight color.RGBA) *image.RGBA {
	img := image.NewRGBA(rect)
	bg := colorIdle
	if v.Active {
		bg = highlight
	}
	draw.Draw(img, rect, image.NewUniform(bg), image.Point{}, draw.Src)

	band := rect
	band.Min.Y = rect.Max.Y - rect.Dy()*3/10
	above := rect
	above.Max.Y = band.Min.Y

	alpha, text := uint8(alphaIdle), colorMuted
	switch {
	case v.Active:
		alpha, text = alphaFull, colorText
	case v.Offline:
		alpha, text = alphaOffline, colorFaint
	}

	size := float64(rect.Dy()) * 13 / 72
	big := float64(rect.Dy()) * 18 / 72
	switch {
	case v.Offline:
		if v.Icon != nil {
			drawIcon(img, v.Icon, above.Inset(rect.Dy()/12), alpha)
		} else {
			drawText(img, v.Label, above.Inset(2), text, size)
		}
		draw.Draw(img, band, image.NewUniform(colorOffline), image.Point{}, draw.Src)
		drawText(img, "OFFLINE", band.Inset(2), colorText, size)
	case v.Icon != nil && v.Label != "":
		drawIcon(img, v.Icon, above.Inset(rect.Dy()/12), alpha)
		drawText(img, v.Label, band.Inset(2), text, size)
	case v.Icon != nil:
		drawIcon(img, v.Icon, rect.Inset(rect.Dy()/8), alpha)
	default:
		drawText(img, v.Label, rect.Inset(4), text, big)
	}

	if v.Active {
		drawBorder(img, max(rect.Dy()/24, 1))
	}
	return img
}

// drawIcon scales the icon to fit area, keeping its shape.
func drawIcon(dst *image.RGBA, icon image.Image, area image.Rectangle, alpha uint8) {
	src := icon.Bounds()
	if src.Empty() || area.Empty() {
		return
	}
	w, h := area.Dx(), src.Dy()*area.Dx()/src.Dx()
	if h > area.Dy() {
		w, h = src.Dx()*area.Dy()/src.Dy(), area.Dy()
	}
	x := area.Min.X + (area.Dx()-w)/2
	y := area.Min.Y + (area.Dy()-h)/2
	box := image.Rect(x, y, x+w, y+h)

	scaled := image.NewRGBA(box)
	draw.CatmullRom.Scale(scaled, box, icon, src, draw.Src, nil)
	draw.DrawMask(dst, box, scaled, box.Min, image.NewUniform(color.Alpha{A: alpha}), image.Point{}, draw.Over)
}

// drawText centres s in area, shrinking it to fit and cutting what still won't.
func drawText(dst *image.RGBA, s string, area image.Rectangle, col color.Color, size float64) {
	const smallest = 8
	if size < smallest {
		size = smallest
	}
	face := newFace(size)
	for size > smallest && font.MeasureString(face, s).Ceil() > area.Dx() {
		size--
		face.Close()
		face = newFace(size)
	}
	defer face.Close()

	s = cutToFit(face, s, area.Dx())
	m := face.Metrics()
	x := area.Min.X + (area.Dx()-font.MeasureString(face, s).Ceil())/2
	y := area.Min.Y + (area.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
	d := font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// cutToFit returns s as it is if it fits in width, or else as much of it as
// fits with an ellipsis.
func cutToFit(face font.Face, s string, width int) string {
	if font.MeasureString(face, s).Ceil() <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 && font.MeasureString(face, string(runes)+"…").Ceil() > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func newFace(size float64) font.Face {
	face, err := opentype.NewFace(labelFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // only for a size of zero or less
	}
	return face
}

func drawBorder(img *image.RGBA, width int) {
	r := img.Bounds()
	white := image.NewUniform(colorText)
	for _, edge := range []image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+width),
		image.Rect(r.Min.X, r.Max.Y-width, r.Max.X, r.Max.Y),
		image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y),
		image.Rect(r.Max.X-width, r.Min.Y, r.Max.X, r.Max.Y),
	} {
		draw.Draw(img, edge, white, image.Point{}, draw.Src)
	}
}
