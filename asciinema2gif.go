package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"

	"github.com/cirocosta/asciinema-edit/cast"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	ansi "github.com/icecrime/ansi/internals"
	fontdata "golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/math/fixed"
)

func main() {
	c, err := cast.Decode(os.Stdin)
	if err != nil {
		die(err.Error())
	}

	font, err := truetype.Parse(fontdata.TTF)
	if err != nil {
		die(err.Error())
	}

	scr := NewScreen(int(c.Header.Width), int(c.Header.Height), font)
	_ = scr

	x, y := 0, 0
	scr.Image.Palette[0] = color.RGBA{0, 0, 0, 128}
	scr.Image.Palette[1] = color.RGBA{255, 255, 255, 128}
	scr.Image.Palette[2] = color.RGBA{255, 0, 0, 128}
	for _, ev := range c.EventStream {
		if ev.Type != "o" {
			continue
		}
		lex := ansi.NewLexer([]byte(ev.Data))
		for tok := lex.NextItem(); tok.T != ansi.EOF; tok = lex.NextItem() {
			fmt.Printf("%s %q\n", tok.T.String(), string(tok.Value))
			switch tok.T {
			case ansi.RawBytes:
				for _, ch := range tok.Value {
					scr.SetCell(x, y, rune(ch), 1, 2)
					x++
				}
			}
		}
	}
	for i := 0; i < scr.Image.Bounds().Max.X && i < scr.Image.Bounds().Max.Y; i++ {
		scr.Image.SetColorIndex(i, i, 1)
		// scr.Image.Set(i, i, 1)
	}

	// TODO: loop events:
	// TODO: - parse event's data
	// TODO: - render event's contents on simulated window, using the font
	// TODO: - add image into gif struct
	// TODO: render animated gif asciinema.gif

	pal := scr.Image.Palette
	for pal[len(pal)-1] == nil {
		pal = pal[:len(pal)-1]
	}
	scr.Image.Palette = pal

	img := &gif.GIF{
		Image: []*image.Paletted{scr.Image},
		Delay: []int{0},
		Config: image.Config{
			Width:  scr.Image.Bounds().Max.X,
			Height: scr.Image.Bounds().Max.Y,
		},
	}
	f, err := os.Create("asciinema.gif")
	if err != nil {
		die(err.Error())
	}
	defer f.Close()
	err = gif.EncodeAll(f, img)
	if err != nil {
		die(err.Error())
	}
}

func die(msg string) {
	os.Stdout.WriteString("error: " + msg + "\n")
	os.Exit(1)
}

func NewScreen(w, h int, font *truetype.Font) Screen {
	// FIXME: variable scale, as flag
	b := font.Bounds(fixed.I(10))
	cw := b.Max.X - b.Min.X
	ch := b.Max.Y - b.Min.Y
	rect := image.Rect(0, 0, w*cw.Ceil(), h*ch.Ceil())
	// palette := make(color.Palette, 256)
	palette := make(color.Palette, 3)
	img := image.NewPaletted(rect, palette)

	ctx := freetype.NewContext()
	ctx.SetFont(font)
	// FIXME: match scale used for bounds
	ctx.SetFontSize(12)
	ctx.SetDst(img)
	ctx.SetClip(img.Bounds())

	return Screen{
		Image: img,
		CellW: cw.Ceil(),
		CellH: ch.Ceil(),
		Font:  ctx,
	}
}

type Screen struct {
	Image        *image.Paletted
	CellW, CellH int
	Font         *freetype.Context
}

func (s *Screen) SetCell(x, y int, ch rune, fg, bg int) {
	draw.Draw(s.Image, image.Rect(
		x*s.CellW, y*s.CellH,
		(x+1)*s.CellW, (y+1)*s.CellH),
		image.NewUniform(s.Image.Palette[bg]),
		image.Pt(0, 0), draw.Src)
	// FIXME: adjust x and y appropriately
	s.Font.SetSrc(image.NewUniform(s.Image.Palette[fg]))
	// FIXME: ensure below multiplications are correct
	s.Font.DrawString(string(ch), fixed.P(x*s.CellW, (y+1)*s.CellH))
}
