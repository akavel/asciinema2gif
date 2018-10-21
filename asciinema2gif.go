package main

import (
	"fmt"
	"os"

	"github.com/cirocosta/asciinema-edit/cast"
	"github.com/golang/freetype/truetype"
	ansi "github.com/icecrime/ansi/internals"
	fontdata "golang.org/x/image/font/gofont/gomono"
)

func main() {
	c, err := cast.Decode(os.Stdin)
	if err != nil {
		die(err.Error())
	}
	_ = c

	font, err := truetype.Parse(fontdata.TTF)
	if err != nil {
		die(err.Error())
	}
	_ = font

	for _, ev := range c.EventStream {
		if ev.Type != "o" {
			continue
		}
		lex := ansi.NewLexer([]byte(ev.Data))
		for tok := lex.NextItem(); tok.T != ansi.EOF; tok = lex.NextItem() {
			fmt.Printf("%s %q\n", tok.T.String(), string(tok.Value))
		}
	}

}

func die(msg string) {
	os.Stdout.WriteString("error: " + msg + "\n")
	os.Exit(1)
}
