package main

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"golang.org/x/image/font"
)

var keyRect = image.Rect(0, 0, 72, 72)

func testIcon() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, color.White)
		}
	}
	return img
}

var testHighlight = color.RGBA{0x2e, 0x7d, 0x32, 0xff}

func TestActiveKeyIsTheHighlightWithAWhiteEdge(t *testing.T) {
	img := renderKey(KeyView{Label: "Home", Active: true}, keyRect, testHighlight)
	if got := img.RGBAAt(36, 8); got != testHighlight {
		t.Errorf("background = %v", got)
	}
	if got := img.RGBAAt(0, 0); got != colorText {
		t.Errorf("edge = %v", got)
	}
}

func TestIdleKeyIsBlack(t *testing.T) {
	img := renderKey(KeyView{Label: "Home"}, keyRect, testHighlight)
	if got := img.RGBAAt(0, 0); got != colorIdle {
		t.Errorf("corner = %v", got)
	}
}

func TestOfflineKeyHasARedBand(t *testing.T) {
	for _, v := range []KeyView{
		{Label: "Home", Offline: true},
		{Icon: testIcon(), Offline: true},
		{Label: "Home", Icon: testIcon(), Offline: true},
	} {
		img := renderKey(v, keyRect, testHighlight)
		if got := img.RGBAAt(1, 70); got != colorOffline {
			t.Errorf("%+v: band = %v", v, got)
		}
	}
}

// The offline key's icon is much fainter than an idle one, whatever it's drawn with.
func TestOfflineIconIsFaint(t *testing.T) {
	idle := renderKey(KeyView{Icon: testIcon(), Label: "x"}, keyRect, testHighlight).RGBAAt(36, 25)
	off := renderKey(KeyView{Icon: testIcon(), Label: "x", Offline: true}, keyRect, testHighlight).RGBAAt(36, 25)
	if idle.R == 0 || off.R == 0 || off.R >= idle.R/2 {
		t.Errorf("idle %v, offline %v", idle, off)
	}
}

// Text that's too long shrinks and is cut, staying inside its key.
func TestLongLabels(t *testing.T) {
	for _, label := range []string{"Kalender", "Blåbærsyltetøy og mer til", strings.Repeat("W", 200), "Ø"} {
		img := renderKey(KeyView{Label: label}, keyRect, testHighlight)
		if img.Bounds() != keyRect {
			t.Errorf("%q: bounds %v", label, img.Bounds())
		}
		if lit(img) == 0 {
			t.Errorf("%q drew nothing", label)
		}
	}
}

func TestCutToFit(t *testing.T) {
	face := newFace(13)
	defer face.Close()
	// What fits stays whole, however little room is left for an ellipsis.
	exact := font.MeasureString(face, "OFFLINE").Ceil()
	if got := cutToFit(face, "OFFLINE", exact); got != "OFFLINE" {
		t.Errorf("got %q", got)
	}
	got := cutToFit(face, "OFFLINE", exact-1)
	if !strings.HasSuffix(got, "…") || font.MeasureString(face, got).Ceil() > exact-1 {
		t.Errorf("got %q", got)
	}
}

// The offline word is the point of the band: at a real key's size it must fit.
func TestOfflineFitsOnAKey(t *testing.T) {
	face := newFace(float64(keyRect.Dy()) * 13 / 72)
	defer face.Close()
	if got := cutToFit(face, "OFFLINE", keyRect.Dx()-4); got != "OFFLINE" {
		t.Errorf("got %q", got)
	}
}

func TestIconKeepsItsShape(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 80, 8))
	for x := range 80 {
		for y := range 8 {
			wide.Set(x, y, color.White)
		}
	}
	img := renderKey(KeyView{Icon: wide}, keyRect, testHighlight)
	if got := img.RGBAAt(36, 2); got != colorIdle {
		t.Errorf("above a wide icon = %v, want the background", got)
	}
	if got := img.RGBAAt(36, 36); got == colorIdle {
		t.Errorf("the wide icon's middle is empty")
	}
}

func lit(img *image.RGBA) int {
	n := 0
	for x := range img.Bounds().Dx() {
		for y := range img.Bounds().Dy() {
			if img.RGBAAt(x, y) != colorIdle {
				n++
			}
		}
	}
	return n
}
