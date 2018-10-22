// TODO: GPL license

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"strconv"

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
	w, h := int(c.Header.Width), int(c.Header.Height)

	font, err := truetype.Parse(fontdata.TTF)
	if err != nil {
		die(err.Error())
	}

	scr := NewScreen(w, h, font)

	x, y := 0, 0
	fg, bg := 97, 30
	for iev, ev := range c.EventStream {
		if ev.Type != "o" {
			continue
		}
		lex := ansi.NewLexer([]byte(ev.Data))
		for tok := lex.NextItem(); tok.T != ansi.EOF; tok = lex.NextItem() {
			fmt.Printf("%s %q\n", tok.T.String(), string(tok.Value))
			switch tok.T {
			case ansi.RawBytes:
				for _, ch := range tok.Value {
					scr.SetCell(x, y, rune(ch), fg, bg)
					x++
				}
			case ansi.ControlSequence:
				seq, err := ansi.ParseControlSequence(tok.Value)
				if err != nil {
					// TODO(akavel): try to avoid having to import 'fmt' and see if it reduces binary size
					panic(fmt.Sprintf("cannot parse control sequence %q in event %d (t=%v)", tok.Value, iev, ev.Time))
				}
				switch seq.Command {
				case 'J': // clear parts of the screen
					switch seqMode(seq, "0") {
					case "2", "3":
						// clear whole screen
						draw.Draw(scr.Image, scr.Image.Bounds(), image.NewUniform(scr.Image.Palette[0]), image.Pt(0, 0), draw.Src)
					default:
						panic(fmt.Sprintf("unknown control sequence: %q %#v", tok.Value, seq))
					}
				case 'K': // clear parts of line
					switch seqMode(seq, "0") {
					case "0":
						// clear from cursor to end of line
						clearCells(scr, x, y, w, y, bg)
					default:
						panic(fmt.Sprintf("unknown control sequence: %q %#v", tok.Value, seq))
					}
				case 'H': // position cursor
					x, y = 0, 0
					if len(seq.Params) >= 1 {
						x = atoi(seq.Params[0]) - 1
					}
					if len(seq.Params) >= 2 {
						y = atoi(seq.Params[1]) - 1
					}
				case 'm': // set colors
					if len(seq.Params) == 0 {
						fg, bg = 97, 30
					} else {
						for _, p := range seq.Params {
							n := atoi(p)
							switch {
							case 90 <= n && n <= 97:
								fg = n
							case 100 <= n && n <= 107:
								bg = n - 10
							case n == 0:
								fg, bg = 97, 30
							default:
								panic("unknown color param " + string(p))
							}
						}
					}
				case 'h':
					// FIXME: "\x1b[?25h = show the cursor"
					// FIXME: "\x1b[?1049h = enable alternative screen buffer"
				case 'l':
					// FIXME: "\x1b[?25l = hide the cursor"
					// FIXME: "\x1b[?1049l = disable alternative screen buffer"
				default:
					panic(fmt.Sprintf("unknown control sequence: %q %#v", tok.Value, seq))
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
	// FIXME(akavel): variable font size & DPI, as flags
	// Note: that's the default value used in the truetype package
	const fontDPI = 72
	const fontSize = 12.0
	// See: freetype.Context#recalc()
	// at: https://github.com/golang/freetype/blob/41fa49aa5b23cc7c4082c9aaaf2da41e195602d9/freetype.go#L263
	// also a comment from the same file:
	// "scale is the number of 26.6 fixed point units in 1 em"
	// (where 26.6 means 26 bits integer and 6 fractional)
	// also from docs:
	// "If the device space involves pixels, 64 units
	// per pixel is recommended, since that is what
	// the bytecode hinter uses [...]".
	// TODO(akavel): check if something like this is maybe already available in new versions of freetype
	const fontScale = fixed.Int26_6(fontSize * fontDPI * (64.0 / 72.0))

	b := font.Bounds(fontScale)
	cell := image.Rect(
		b.Min.X.Ceil(), b.Min.Y.Ceil(),
		b.Max.X.Ceil(), b.Max.Y.Ceil())
	rect := image.Rect(0, 0, w*cell.Dx(), h*cell.Dy())

	// https://en.wikipedia.org/wiki/ANSI_escape_code#Colors
	pal := make(color.Palette, 256)
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }
	pal[90] = rgb(128, 128, 128)
	pal[91] = rgb(255, 0, 0)
	pal[92] = rgb(0, 255, 0)
	pal[93] = rgb(255, 255, 0)
	pal[94] = rgb(0, 0, 255)
	pal[95] = rgb(255, 0, 255)
	pal[96] = rgb(0, 255, 255)
	pal[97] = rgb(255, 255, 255)
	for i, col := range pal {
		if col == nil {
			pal[i] = rgb(0, 0, 0)
		}
	}
	img := image.NewPaletted(rect, pal)

	ctx := freetype.NewContext()
	ctx.SetFont(font)
	ctx.SetFontSize(fontSize)
	ctx.SetDPI(fontDPI)
	ctx.SetDst(img)
	ctx.SetClip(img.Bounds())

	return Screen{
		Image: img,
		Font:  ctx,
		Cell:  cell,
	}
}

type Screen struct {
	Image *image.Paletted
	Font  *freetype.Context
	Cell  image.Rectangle
}

func (s *Screen) SetCell(x, y int, ch rune, fg, bg int) {
	clearCells(*s, x, y, x, y, bg)
	// draw.Draw(s.Image, image.Rect(
	// 	x*s.Cell.Dx(), y*s.Cell.Dy(),
	// 	(x+1)*s.Cell.Dx(), (y+1)*s.Cell.Dy()),
	// 	image.NewUniform(s.Image.Palette[bg]),
	// 	image.Pt(0, 0), draw.Src)
	s.Font.SetSrc(image.NewUniform(s.Image.Palette[fg]))
	s.Font.DrawString(string(ch), fixed.P(x*s.Cell.Dx(), y*s.Cell.Dy()+s.Cell.Max.Y))
}

func atoi(b []byte) int {
	i, err := strconv.Atoi(string(b))
	if err != nil {
		panic(err)
	}
	return i
}

func seqMode(seq *ansi.SequenceData, default_ string) string {
	if len(seq.Params) == 0 {
		return default_
	}
	return string(seq.Params[0])
}

func clearCells(scr Screen, x1, y1, x2, y2 int, color int) {
	rect := image.Rect(
		x1*scr.Cell.Dx(), y1*scr.Cell.Dy(),
		(x2+1)*scr.Cell.Dx(), (y2+1)*scr.Cell.Dy())
	draw.Draw(scr.Image, rect, image.NewUniform(scr.Image.Palette[color]), image.Pt(0, 0), draw.Src)
}
