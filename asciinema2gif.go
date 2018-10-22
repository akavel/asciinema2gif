package main

import (
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/cirocosta/asciinema-edit/cast"
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

	for _, ev := range c.EventStream {
		if ev.Type != "o" {
			continue
		}
		lex := ansi.NewLexer([]byte(ev.Data))
		for tok := lex.NextItem(); tok.T != ansi.EOF; tok = lex.NextItem() {
			fmt.Printf("%s %q\n", tok.T.String(), string(tok.Value))
		}
	}

	// TODO: loop events:
	// TODO: - parse event's data
	// TODO: - render event's contents on simulated window, using the font
	// TODO: - add image into gif struct
	// TODO: render animated gif asciinema.gif
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
	rect := image.Rect(0, 0, cw.Ceil(), ch.Ceil())
	palette := make(color.Palette, 256)
	return Screen{
		Image:      image.NewPaletted(rect, palette),
		CellBounds: b,
	}
}

type Screen struct {
	Image      *image.Paletted
	CellBounds fixed.Rectangle26_6
}
