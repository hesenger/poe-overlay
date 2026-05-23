package main

import (
	"bufio"
	"image"
	"image/color"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const windowTitle = "PoE-Overlay-Window"
const logFilePath = `C:\Program Files (x86)\Grinding Gear Games\Path of Exile 2\logs\LatestClient.txt`

var sceneRegex = regexp.MustCompile(`\[SCENE\] Set Source \[(.*?)\]`)

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

	// Channel to receive area names from the log reader
	areaChan := make(chan string, 10)
	go tailLogFile(areaChan)

	// Current area name to display - protected by mutex since it's updated
	// from a different goroutine.
	var currentArea string = "Waiting for scene..."
	var areaMu sync.Mutex

	// Goroutine to consume area updates and invalidate the window,
	// forcing an immediate redraw.
	go func() {
		for area := range areaChan {
			areaMu.Lock()
			currentArea = area
			areaMu.Unlock()
			w.Invalidate()
		}
	}()

	// Theme for text rendering
	th := material.NewTheme()

	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			ops.Reset()

			areaMu.Lock()
			areaName := currentArea
			areaMu.Unlock()

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

			gtx := layout.Context{
				Ops:         &ops,
				Now:         e.Now,
				Metric:      e.Metric,
				Source:      e.Source,
				Constraints: layout.Exact(e.Size),
			}

			// 2. Map out the clickable gesture area over the whole window size
			area := clip.Rect(image.Rectangle{Max: e.Size}).Push(&ops)

			// 3. Direct the OS to handle native window movement on drag
			system.ActionInputOp(system.ActionMove).Add(&ops)

			area.Pop()

			// 4. Render a dark window background
			paint.ColorOp{Color: color.NRGBA{R: 0x2d, G: 0x2d, B: 0x30, A: 0xff}}.Add(&ops)
			paint.PaintOp{}.Add(&ops)

			// 5. Render the current area name centered in the window
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Label(th, unit.Sp(24), areaName)
				label.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
				label.Alignment = text.Middle
				return label.Layout(gtx)
			})

			e.Frame(&ops)
		}
	}
}

func tailLogFile(areaChan chan<- string) {
	// Wait for file to exist
	for {
		if _, err := os.Stat(logFilePath); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// On first read, scan the entire file to find the most recent scene
	// so the UI shows the current area immediately.
	firstRead := true
	lastPos := int64(0)

	for {
		file, err := os.Open(logFilePath)
		if err != nil {
			log.Printf("Failed to open log file: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		info, err := file.Stat()
		if err != nil {
			file.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		currentSize := info.Size()

		if currentSize < lastPos {
			// File was truncated or rotated
			lastPos = 0
		}

		_, err = file.Seek(lastPos, io.SeekStart)
		if err != nil {
			file.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		reader := bufio.NewReader(file)
		var lineBuf strings.Builder
		var latestScene string

		for {
			data, err := reader.ReadBytes('\n')
			if len(data) > 0 {
				lineBuf.Write(data)
				if err == nil {
					// Complete line
					line := strings.TrimSpace(lineBuf.String())
					lineBuf.Reset()

					if matches := sceneRegex.FindStringSubmatch(line); matches != nil {
						scene := matches[1]
						if scene != "(null)" {
							latestScene = scene
						}
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading log file: %v", err)
				}
				break
			}
		}

		if latestScene != "" {
			log.Printf("Scene change detected: %s", latestScene)
			areaChan <- latestScene
		}

		// If we have a partial line (no newline yet), keep lastPos at the
		// start of that line so we re-read it next iteration.
		if lineBuf.Len() == 0 {
			lastPos = currentSize
		}

		file.Close()

		// After the first read, switch to tailing from the end
		if firstRead {
			firstRead = false
			lastPos = currentSize
		}

		time.Sleep(500 * time.Millisecond)
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
		HWND_TOPMOST   = ^uintptr(0) // -1
		SWP_NOSIZE     = 0x0001
		SWP_NOMOVE     = 0x0002
		SWP_NOACTIVATE = 0x0010 // Crucial: Prevents state confusion during composition shifts
	)

	_, _, _ = setWindowPos.Call(
		hwnd,
		HWND_TOPMOST,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
	)
}
