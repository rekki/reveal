package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/rekki/reveal/reveal"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: reveal <pkg>")
	}

	out, err := reveal.Reveal(context.Background(), os.Args[1])
	if err != nil {
		panic(err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("  ", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
