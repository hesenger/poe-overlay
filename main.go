package main

import (
	"image"
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func main() {
	go func() {
		var w app.Window
		w.Option(
			app.Title("Borderless Drag Window"),
			app.Size(unit.Dp(400), unit.Dp(300)),
			app.Decorated(false), // Strip native borders and title bar
			app.TopMost(true),
		)

		if err := run(&w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	var ops op.Ops

	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			ops.Reset()

			// 1. Map out the clickable gesture area over the whole window size
			area := clip.Rect(image.Rectangle{Max: e.Size}).Push(&ops)

			// 2. Cast the Action type directly to ActionInputOp and append to the operations stack.
			// This tells the OS to handle window movement natively when the user drags inside this zone.
			system.ActionInputOp(system.ActionMove).Add(&ops)

			area.Pop()

			// 3. Render a dark window background
			paint.ColorOp{Color: color.NRGBA{R: 0x2d, G: 0x2d, B: 0x30, A: 0xff}}.Add(&ops)
			paint.PaintOp{}.Add(&ops)

			e.Frame(&ops)
		}
	}
}
