package main

import (
	"image"
	"image/color"
	"log"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

const windowTitle = "PoE-Overlay-Window"

func main() {
	go func() {
		var w app.Window
		w.Option(
			app.Title(windowTitle),
			app.Size(unit.Dp(400), unit.Dp(300)),
			app.Decorated(false), // Borderless
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
	frameCount := 0

	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			ops.Reset()

			// 1. Let the first frame render completely so Windows allocates
			// the actual screen placement. Then trigger TopMost on frame 2.
			if frameCount == 1 && runtime.GOOS == "windows" {
				go func() {
					// A microscopic pause guarantees Gio finished updating its internal window pipeline
					time.Sleep(10 * time.Millisecond)
					makeWindowTopMost(windowTitle)
				}()
			}
			if frameCount < 2 {
				frameCount++
			}

			// 2. Map out the clickable gesture area over the whole window size
			area := clip.Rect(image.Rectangle{Max: e.Size}).Push(&ops)

			// 3. Direct the OS to handle native window movement on drag
			system.ActionInputOp(system.ActionMove).Add(&ops)

			area.Pop()

			// 4. Render a dark window background
			paint.ColorOp{Color: color.NRGBA{R: 0x2d, G: 0x2d, B: 0x30, A: 0xff}}.Add(&ops)
			paint.PaintOp{}.Add(&ops)

			e.Frame(&ops)
		}
	}
}

func makeWindowTopMost(title string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	setWindowPos := user32.NewProc("SetWindowPos")

	winTitlePtr, _ := syscall.UTF16PtrFromString(title)

	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(winTitlePtr)))
	if hwnd == 0 {
		return
	}

	const (
		HWND_TOPMOST    = ^uintptr(0) // -1
		SWP_NOSIZE      = 0x0001
		SWP_NOMOVE      = 0x0002
		SWP_NOACTIVATE  = 0x0010 // Crucial: Prevents state confusion during composition shifts
	)

	_, _, _ = setWindowPos.Call(
		hwnd,
		HWND_TOPMOST,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
	)
}
