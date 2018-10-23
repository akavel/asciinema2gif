// +build none

package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math"
	"os"

	"github.com/cirocosta/asciinema-edit/cast"
)

func main() {
	c, err := cast.Decode(os.Stdin)
	if err != nil {
		die(err.Error())
	}

	maxt := int(math.Ceil(c.EventStream[len(c.EventStream)-1].Time))

	side := int(math.Ceil(math.Sqrt(float64(len(c.EventStream)))))
	rect := image.Rect(0, 0, side+maxt/side+1, side+5)

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
	drawRect(img, rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y, 90)

	anim := &gif.GIF{
		Config: image.Config{
			Width:  img.Bounds().Max.X,
			Height: img.Bounds().Max.Y,
		},
	}

	const blink = 0.5

	tprev := 0.0
	for iev, ev := range c.EventStream {
		if iev%100 == 99 {
			os.Stderr.WriteString(".")
		}

		dt := int(ev.Time*100) - int(tprev*100)
		if dt > 0 {
			// fmt.Println(int(ev.Time/blink), int(tprev/blink), ev.Time)
			for tprev < ev.Time {
				t := float64(int(tprev/blink)+1)*blink + 0.00001
				// fmt.Println("\t", tprev, int(tprev/blink), cursor, dbg)
				if t > ev.Time {
					t = ev.Time
				}
				dt = int(t*100) - int(tprev*100)
				// fmt.Printf("dt= % 6d  t= %v\n", dt, t)
				if int(tprev/blink)&1 == 1 {
					drawRect(img, 0, side+1, side, side+5, 31)
					// if !dbg {
					// 	dbg = true
					// 	// fmt.Println("#", t)
					// }
				} else {
					drawRect(img, 0, side+1, side, side+5, 96)
					// if dbg {
					// 	dbg = false
					// 	// fmt.Println("_", t)
					// }
				}
				tprev = t
				frame := image.NewPaletted(img.Bounds(), img.Palette)
				draw.Draw(frame, img.Bounds(), img, image.Pt(0, 0), draw.Src)
				anim.Image = append(anim.Image, frame)
				// if int(*maxPause*100) > 0 && dt > int(*maxPause*100) {
				// 	dt = int(*maxPause * 100)
				// }
				anim.Delay = append(anim.Delay, dt)
			}
		}

		// image inside, marking # of events
		y := (iev + 1) / side
		if y > 0 {
			drawRect(img, 0, 0, side, y-1, 97)
		}
		drawRect(img, 0, y, (iev+1)%side, y, 97)
		// fmt.Println(y, (iev+1)%side)

		// 2nd image, marking T of events
		dx := int(math.Ceil(ev.Time)) / side
		if dx > 0 {
			drawRect(img, side+1, 0, side+dx, side, 94)
		}
		drawRect(img, side+1+dx, 0, side+1+dx, int(math.Ceil(ev.Time))%side, 94)
	}

	f, err := os.Create("blink.gif")
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

func drawRect(img *image.Paletted, x0, y0, x1, y1, color int) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), image.NewUniform(img.Palette[color]), image.Pt(0, 0), draw.Src)
}
