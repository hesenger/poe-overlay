package main

import (
	"bufio"
	"fmt"
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
const logFilePath = `C:\Program Files (x86)\Grinding Gear Games\Path of Exile 2\logs\Client.txt`
const guidePath = `data\act1.txt`

var sceneRegex = regexp.MustCompile(`\[SCENE\] Set Source \[(.*?)\]`)

func badgeLayout(gtx layout.Context, th *material.Theme, stepType StepType) layout.Dimensions {
	const badgeWidthDp = unit.Dp(58)
	badgeWidthPx := gtx.Dp(badgeWidthDp)

	badgeText := fmt.Sprintf("[%s]", strings.ToUpper(string(stepType)))
	label := material.Label(th, unit.Sp(12), badgeText)
	label.Color = stepColor(stepType)
	label.Alignment = text.Middle

	// Fix the width so every badge lines up.
	gtx.Constraints.Min.X = badgeWidthPx
	gtx.Constraints.Max.X = badgeWidthPx

	dims := label.Layout(gtx)
	dims.Size.X = badgeWidthPx
	return dims
}

func stepColor(t StepType) color.NRGBA {
	switch t {
	case StepNPC:
		return color.NRGBA{R: 0x4d, G: 0xb8, B: 0xff, A: 0xff} // blue
	case StepLoot:
		return color.NRGBA{R: 0xff, G: 0xd7, B: 0x00, A: 0xff} // gold
	case StepKill:
		return color.NRGBA{R: 0xff, G: 0x4d, B: 0x4d, A: 0xff} // red
	case StepExit:
		return color.NRGBA{R: 0x4d, G: 0xff, B: 0x4d, A: 0xff} // green
	case StepTP:
		return color.NRGBA{R: 0x4d, G: 0xff, B: 0xff, A: 0xff} // cyan
	case StepUse:
		return color.NRGBA{R: 0xc0, G: 0x4d, B: 0xff, A: 0xff} // purple
	case StepTravel:
		return color.NRGBA{R: 0xff, G: 0xa5, B: 0x00, A: 0xff} // orange
	default:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // white
	}
}

func main() {
	go func() {
		var w app.Window
		w.Option(
			app.Title(windowTitle),
			app.Size(unit.Dp(500), unit.Dp(400)),
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

	// Load all guide files from data directory in game progression order.
	guideFiles := []string{
		`data\act1.txt`,
		`data\act2.txt`,
		`data\act3.txt`,
		`data\act4.txt`,
		`data\interlude1.txt`,
		`data\interlude2.txt`,
		`data\interlude3.txt`,
	}
	guide, err := LoadGuides(guideFiles)
	if err != nil {
		log.Printf("Failed to load guides: %v", err)
		guide = &Guide{}
	}

	// Initialize from recent log scenes so we start at the right guide step.
	var currentArea string = "Waiting for scene..."
	var currentSection Section
	var areaFound bool
	var areaMu sync.Mutex

	if recentScenes := getRecentScenes(logFilePath, 2); len(recentScenes) > 0 {
		guide.InitializeFromAreas(recentScenes)
		areaMu.Lock()
		currentArea = recentScenes[len(recentScenes)-1]
		currentSection = guide.CurrentSection()
		areaFound = true
		areaMu.Unlock()
	}

	// Channel to receive area names from the log reader
	areaChan := make(chan string, 10)
	go tailLogFile(areaChan)

	// Goroutine to consume area updates and invalidate the window
	go func() {
		for area := range areaChan {
			areaMu.Lock()
			currentArea = area
			if guide.OnAreaChange(area) {
				currentSection = guide.CurrentSection()
				areaFound = true
			} else {
				// Not the next step (e.g., a quick town refill).
				// Keep the previous guide section visible.
				areaFound = false
			}
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
			section := currentSection
			found := areaFound
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

			// Render a dark window background
			paint.ColorOp{Color: color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1e, A: 0xff}}.Add(&ops)
			paint.PaintOp{}.Add(&ops)

			// 5. Render guide content
			layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Area name header (from the guide section, not the raw log)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									headerArea := section.Area
									if headerArea == "" {
										headerArea = areaName
									}
									label := material.Label(th, unit.Sp(16), headerArea)
									label.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
									return label.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									var mark string
									var markColor color.NRGBA
									if found {
										mark = "\u2713" // checkmark
										markColor = color.NRGBA{R: 0x4d, G: 0xff, B: 0x4d, A: 0xff}
									} else {
										mark = "\u2717" // X
										markColor = color.NRGBA{R: 0xff, G: 0x4d, B: 0x4d, A: 0xff}
									}
									label := material.Label(th, unit.Sp(16), mark)
									label.Color = markColor
									return label.Layout(gtx)
								}),
							)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					// Steps list
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if len(section.Steps) == 0 {
							label := material.Label(th, unit.Sp(14), "No guide steps for this area.")
							label.Color = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
							label.Alignment = text.Middle
							return label.Layout(gtx)
						}

						list := layout.List{Axis: layout.Vertical}
						return list.Layout(gtx, len(section.Steps), func(gtx layout.Context, i int) layout.Dimensions {
							step := section.Steps[i]
							return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return badgeLayout(gtx, th, step.Type)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										textLabel := material.Label(th, unit.Sp(14), step.Text)
										textLabel.Color = color.NRGBA{R: 0xdd, G: 0xdd, B: 0xdd, A: 0xff}
										return textLabel.Layout(gtx)
									}),
								)
							})
						})
					}),
					// Current log area at the bottom
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Label(th, unit.Sp(11), areaName)
							label.Color = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
							return label.Layout(gtx)
						})
					}),
				)
			})

			// Make the entire window draggable by adding a system move action on top of all content.
			area := clip.Rect(image.Rectangle{Max: e.Size}).Push(gtx.Ops)
			system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
			area.Pop()

			e.Frame(&ops)
		}
	}
}

func getRecentScenes(path string, count int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	scenes := make([]string, 0, count)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := sceneRegex.FindStringSubmatch(line); matches != nil {
			scene := matches[1]
			if scene != "(null)" && scene != "" {
				scenes = append(scenes, scene)
				if len(scenes) > count {
					scenes = scenes[1:]
				}
			}
		}
	}
	return scenes
}

func tailLogFile(areaChan chan<- string) {
	// Wait for file to exist and capture its current size so we only
	// process new lines written after the overlay starts.
	var lastPos int64
	for {
		if info, err := os.Stat(logFilePath); err == nil {
			lastPos = info.Size()
			break
		}
		time.Sleep(2 * time.Second)
	}

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
