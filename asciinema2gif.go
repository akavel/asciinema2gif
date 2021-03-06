package main

// asciinema2gif - converts ASCIInema asciicast v2 files to animated GIFs
// Copyright (C) 2018 by the asciinema2gif AUTHORS
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.
// package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"strconv"
	"unicode/utf8"

	"github.com/cirocosta/asciinema-edit/cast"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	ansi "github.com/icecrime/ansi/internals"
	fontopt "golang.org/x/image/font"
	fontdata "golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/math/fixed"
)

var (
	dpi      = flag.Int("dpi", 144, "dots per inch (resolution)")
	fontPath = flag.String("font", "", "path to TrueType font to use; if not specified, Go Mono is used")
	fontSize = flag.Float64("font-size", 12, "font size")
	maxPause = flag.Float64("i", 0, "max pause between frames, in seconds (0 means unlimited)")
	cpuprof  = flag.String("cpuprof", "", "write CPU profiling info into specified `file`")
)

func main() {
	flag.Parse()

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			die(err.Error())
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			die(err.Error())
		}
		defer pprof.StopCPUProfile()
	}

	c, err := cast.Decode(os.Stdin)
	if err != nil {
		die(err.Error())
	}
	w, h := int(c.Header.Width), int(c.Header.Height)

	if *fontPath != "" {
		data, err := ioutil.ReadFile(*fontPath)
		if err != nil {
			die(err.Error())
		}
		fontdata.TTF = data
	}
	font, err := truetype.Parse(fontdata.TTF)
	if err != nil {
		die(err.Error())
	}

	scr := NewScreen(w, h, font)

	anim := &gif.GIF{
		Config: image.Config{
			Width:  scr.Image.Bounds().Max.X,
			Height: scr.Image.Bounds().Max.Y,
		},
	}

	// HACK: reduce max pauses between frames
	if *maxPause >= 0.01 {
		tprev := 0.0
		sub := 0.0
		for _, ev := range c.EventStream {
			ev.Time -= sub
			dt := ev.Time - tprev
			if dt > *maxPause {
				ev.Time -= dt - *maxPause
				sub += dt - *maxPause
			}
			tprev = ev.Time
		}
	}

	const blink = 0.5
	x, y := 0, 0
	fg, bg := 97, 30
	tprev := 0.0
	cursor := true
	clearCells(scr, 0, 0, w-1, h-1, fg, bg)
	for iev, ev := range c.EventStream {
		if iev%100 == 0 {
			os.Stderr.WriteString(".")
		}
		if ev.Type != "o" {
			continue
		}

		// TODO(akavel): is this correct calculation of delay, or not? should we rather store tprev as int?
		dt := int(ev.Time*100) - int(tprev*100)
		if dt > 0 {
			for tprev < ev.Time {
				t := float64(int(tprev/blink)+1)*blink + 0.00001
				if t > ev.Time {
					t = ev.Time
				}
				dt = int(t*100) - int(tprev*100)
				cur := scr.Grid[y*scr.GridW+x]
				// Note: we draw the *previous* frame, so we must base the
				// cursor state calculations on tprev, not on t.
				if cursor && int(tprev/blink)&1 == 1 {
					scr.SetCell(x, y, cur.Ch, cur.Bg, cur.Fg)
				}
				tprev = t
				frame := image.NewPaletted(scr.Dirty, scr.Image.Palette)
				draw.Draw(frame, frame.Bounds(), scr.Image, scr.Dirty.Min, draw.Src)
				scr.Dirty = image.Rectangle{}
				scr.SetCell(x, y, cur.Ch, cur.Fg, cur.Bg)
				anim.Image = append(anim.Image, frame)
				anim.Delay = append(anim.Delay, dt)
				anim.Disposal = append(anim.Disposal, gif.DisposalNone)
			}
		}

		unparsed := []byte(ev.Data)
		for len(unparsed) > 0 {
			var seq *ansi.SequenceData
			seq, unparsed = parseANSISequence(unparsed)

			if seq == nil {
				if len(unparsed) == 0 {
					continue
				}
				// sequence not detected, so we just have raw byte
				// TODO(akavel): handle non-utf8 encodings?
				ch, sz := utf8.DecodeRune(unparsed)
				unparsed = unparsed[sz:]
				switch ch {
				case '\t':
					newx := x/8*8 + 8
					clearCells(scr, x, y, newx-1, y, fg, bg)
					newx = x
				case '\n':
					y++
					// x = 0
				case '\r':
					x = 0
				case '\b':
					if x > 0 {
						x--
					}
				case 0x1b:
					// Undetected control sequence. Probably it was split
					// between this and next event... seen something like
					// this... let's try moving it to next event.
					if len(unparsed) < 30 && iev+1 < len(c.EventStream) { // sanity check for our assumption
						c.EventStream[iev+1].Data = string(ch) + string(unparsed) + c.EventStream[iev+1].Data
						unparsed = nil
						continue
					}
					panic(fmt.Sprintf("undetected control sequence in event %d (t=%v) = %q (unparsed = %q)", iev, ev.Time, ev.Data, unparsed))
				default:
					scr.SetCell(x, y, ch, fg, bg)
					x++
				}
			} else {
				// ANSI control sequence detected
				switch seq.Command {
				case 'J': // clear parts of the screen
					switch seqMode(seq, "0") {
					case "2", "3":
						// clear whole screen
						clearCells(scr, 0, 0, w-1, h-1, fg, bg)
					default:
						panic(fmt.Sprintf("unknown control sequence with J in: %q %#v", ev.Data, seq))
					}
				case 'K': // clear parts of line
					switch seqMode(seq, "0") {
					case "0", "":
						// clear from cursor to end of line
						clearCells(scr, x, y, w, y, fg, bg)
					default:
						panic(fmt.Sprintf("unknown control sequence with K in: %q %#v", ev.Data, seq))
					}
				case 'H': // position cursor
					x, y = 0, 0
					if len(seq.Params) >= 1 {
						y = atoi(seq.Params[0], 1) - 1
					}
					if len(seq.Params) >= 2 {
						x = atoi(seq.Params[1], 1) - 1
					}
				case 'C': // move cursor forward, unless past EOL already
					x += atoi([]byte(seqMode(seq, "1")), 1)
					// TODO: should we also clear? or not?
					if x >= w {
						x = w - 1
					}
				case 'm': // set colors
					if len(seq.Params) == 0 {
						fg, bg = 97, 30
					} else {
						for _, p := range seq.Params {
							n := atoi(p, 0)
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
				case '@':
					// Insert character
					// https://vt100.net/docs/vt510-rm/chapter4.html
					xx := w - 1
					for xx > x && scr.GetCell(xx, y) == scr.GetCell(xx-1, y) {
						xx-- // no need to redraw identical cells
					}
					for ; xx > x; xx-- {
						c := scr.GetCell(xx-1, y)
						scr.SetCell(xx, y, c.Ch, c.Fg, c.Bg)
					}
				case 'X':
					// Erase character
					// https://vt100.net/docs/vt510-rm/chapter4.html
					x2 := x + atoi(seq.Params[0], 1)
					for ; x < x2 && x < w; x++ {
						c := scr.GetCell(x, y)
						scr.SetCell(x, y, ' ', c.Fg, c.Bg)
					}
					if x >= w {
						x = w - 1
					}
				case 'h', 'l':
					// see also: https://www.real-world-systems.com/docs/ANSIcode.html
					switch cmd := string(seq.Params[0]) + string(seq.Command); cmd {
					case "?25h": // TODO: show the cursor
						cursor = true
					case "?25l": // TODO: hide the cursor
						cursor = false
					case "?1049h": // TODO: enable alternative screen buffer
					case "?1049l": // TODO: disable alternative screen buffer
					case "?12l": // TODO: local echo - input from keyboard sent to screen
					case "?1l": // TODO: transmit only unprotected characters ???
					case "?1000l": // TODO: ??? part of "rs2" reset sequence for VTE (?)
					case "?1002l": // TODO: ??? something related to mouse?
					case "?1003l": // TODO: ??? something related to mouse?
					case "?1006l": // TODO: ??? something related to mouse?
					default:
						panic(fmt.Sprintf("unknown control sequence: %q", cmd))
					}
				default:
					panic(fmt.Sprintf("unknown control sequence: %q %#v (command %c)", ev.Data, seq, seq.Command))
				}
			}
		}
	}

	os.Stderr.WriteString("\n")

	f, err := os.Create("asciinema.gif")
	if err != nil {
		die(err.Error())
	}
	defer f.Close()
	err = gif.EncodeAll(f, anim)
	if err != nil {
		die(err.Error())
	}
}

func die(msg string) {
	os.Stdout.WriteString("error: " + msg + "\n")
	os.Exit(1)
}

func NewScreen(w, h int, font *truetype.Font) Screen {
	fontDPI := float64(*dpi)
	fontSize := *fontSize
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
	fontScale := fixed.Int26_6(fontSize * fontDPI * (64.0 / 72.0))

	// Calculate bounds for the set of ASCII 8-bit characters glyphs, to avoid
	// having double-width (e.g. Chinese?) character glyphs in the measurement.
	b := fixed.Rectangle26_6{}
	for ch := rune(0); ch <= 255; ch++ {
		idx := font.Index(ch)
		glyph := truetype.GlyphBuf{}
		err := glyph.Load(font, fontScale, idx, fontopt.HintingFull)
		if err != nil {
			continue
		}
		b = b.Union(glyph.Bounds)
	}
	cell := image.Rect(
		b.Min.X.Ceil(), b.Min.Y.Ceil(),
		b.Max.X.Ceil(), b.Max.Y.Ceil())
	rect := image.Rect(0, 0, w*cell.Dx(), h*cell.Dy())

	// https://en.wikipedia.org/wiki/ANSI_escape_code#Colors
	pal := make(color.Palette, 256)
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }
	pal[31] = rgb(222, 56, 43)
	pal[32] = rgb(57, 181, 74)
	pal[33] = rgb(255, 199, 6)
	pal[34] = rgb(0, 111, 184)
	pal[35] = rgb(118, 38, 113)
	pal[36] = rgb(44, 181, 233)
	pal[37] = rgb(204, 204, 204)
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
	ctx.SetHinting(fontopt.HintingFull)

	return Screen{
		Image: img,
		Font:  ctx,
		Cell:  cell,
		Grid:  make([]Cell, w*h),
		GridW: w,
		Dirty: rect,
	}
}

type Screen struct {
	Image *image.Paletted
	Font  *freetype.Context
	Cell  image.Rectangle
	Grid  []Cell
	GridW int
	Dirty image.Rectangle
}

type Cell struct {
	Ch     rune
	Fg, Bg int
}

func (s *Screen) SetCell(x, y int, ch rune, fg, bg int) {
	clearCells(*s, x, y, x, y, fg, bg)
	s.Font.SetSrc(image.NewUniform(s.Image.Palette[fg]))
	s.Font.DrawString(string(ch), fixed.P(x*s.Cell.Dx(), y*s.Cell.Dy()+s.Cell.Max.Y-1))
	s.Grid[y*s.GridW+x] = Cell{ch, fg, bg}
	s.Dirty = s.Dirty.Union(image.Rect(
		x*s.Cell.Dx(), y*s.Cell.Dy(),
		(x+1)*s.Cell.Dx(), (y+1)*s.Cell.Dy()))
}

func (s *Screen) GetCell(x, y int) Cell {
	return s.Grid[y*s.GridW+x]
}

func atoi(b []byte, default_ int) int {
	if len(b) == 0 {
		return default_
	}
	i, err := strconv.Atoi(string(b))
	if err != nil {
		panic(err)
	}
	return i
}

func seqMode(seq *ansi.SequenceData, default_ string) string {
	if len(seq.Params) == 0 || (len(seq.Params) == 1 && len(seq.Params[0]) == 0) {
		return default_
	}
	return string(seq.Params[0])
}

func clearCells(scr Screen, x1, y1, x2, y2 int, fg, bg int) {
	rect := image.Rect(
		x1*scr.Cell.Dx(), y1*scr.Cell.Dy(),
		(x2+1)*scr.Cell.Dx(), (y2+1)*scr.Cell.Dy())
	draw.Draw(scr.Image, rect, image.NewUniform(scr.Image.Palette[bg]), image.Pt(0, 0), draw.Src)
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			scr.Grid[y*scr.GridW+x] = Cell{' ', fg, bg}
		}
	}
}

// TODO(akavel): better parser, less ad-hoc
// parseANSISequence parses an ANSI sequence if b starts with an ESC character,
// otherwise returns nil, b
func parseANSISequence(b []byte) (*ansi.SequenceData, []byte) {
	if len(b) < 2 || b[0] != 0x1b {
		return nil, b
	}

	// TODO(akavel): properly handle two-byte sequences; for now, ignoring
	// them; see: http://ascii-table.com/ansi-escape-sequences-vt-100.php
	var ignored = []string{"(B", ">"}
	if b[1] != '[' {
		for _, ign := range ignored {
			if bytes.HasPrefix(b[1:], []byte(ign)) {
				return parseANSISequence(b[len(ign)+1:])
			}
		}
		panic(fmt.Sprintf("unknown ANSI sequence: %q", b))
	}

	// TODO(akavel): would IndexFunc be faster?
	icmd := bytes.IndexAny(b, "@abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if icmd == -1 {
		// log.Printf("cmd not found in %q", b)
		return nil, b
	}
	return &ansi.SequenceData{
		Params:  bytes.Split(b[2:icmd], []byte(";")),
		Command: b[icmd],
	}, b[icmd+1:]
}
