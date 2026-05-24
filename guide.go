package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type StepType string

const (
	StepNPC    StepType = "npc"
	StepLoot   StepType = "loot"
	StepKill   StepType = "kill"
	StepExit   StepType = "exit"
	StepTP     StepType = "tp"
	StepUse    StepType = "use"
	StepTravel StepType = "travel"
)

type Step struct {
	Type StepType
	Text string
}

type Section struct {
	Act   string
	Area  string
	Steps []Step
}

type Guide struct {
	Sections     []Section
	CurrentIndex int
}

func LoadGuide(path, act string) (*Guide, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	guide := &Guide{
		Sections: []Section{},
	}

	var currentSection *Section

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "Step type: ") {
			rest := strings.TrimPrefix(line, "Step type: ")
			if len(rest) < 3 {
				continue
			}

			var stepType StepType
			var text string
			found := false

			for _, t := range []StepType{StepNPC, StepLoot, StepKill, StepExit, StepTP, StepUse, StepTravel} {
				if strings.HasPrefix(rest, string(t)) {
					stepType = t
					text = strings.TrimSpace(strings.TrimPrefix(rest, string(t)))
					found = true
					break
				}
			}

			if !found {
				stepType = StepType("unknown")
				text = rest
			}

			if currentSection != nil {
				currentSection.Steps = append(currentSection.Steps, Step{
					Type: stepType,
					Text: text,
				})
			}
		} else {
			if currentSection != nil {
				guide.Sections = append(guide.Sections, *currentSection)
			}
			currentSection = &Section{
					Act:   act,
					Area:  line,
					Steps: []Step{},
				}
		}
	}

	if currentSection != nil {
		guide.Sections = append(guide.Sections, *currentSection)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return guide, nil
}

func LoadGuides(paths []string) (*Guide, error) {
	combined := &Guide{
		Sections: []Section{},
	}
	for _, path := range paths {
		act := filepath.Base(path)
		act = strings.TrimSuffix(act, filepath.Ext(act))
		g, err := LoadGuide(path, act)
		if err != nil {
			return nil, err
		}
		combined.Sections = append(combined.Sections, g.Sections...)
	}
	return combined, nil
}

func (g *Guide) OnAreaChange(area string) bool {
	if g.CurrentIndex < len(g.Sections) && g.Sections[g.CurrentIndex].Area == area {
		return true
	}
	if g.CurrentIndex+1 < len(g.Sections) && g.Sections[g.CurrentIndex+1].Area == area {
		g.CurrentIndex++
		return true
	}
	return false
}

func (g *Guide) SetIndex(idx int) {
	if idx >= 0 && idx < len(g.Sections) {
		g.CurrentIndex = idx
	}
}

func (g *Guide) CurrentSection() Section {
	if g.CurrentIndex < 0 || g.CurrentIndex >= len(g.Sections) {
		return Section{}
	}
	return g.Sections[g.CurrentIndex]
}

// InitializeFromAreas sets CurrentIndex by looking for a place in the guide
// where the given areas appear consecutively. This disambiguates repeated
// scene names. The last element of areas is the current scene.
func (g *Guide) InitializeFromAreas(areas []string) {
	if len(areas) == 0 || len(g.Sections) == 0 {
		return
	}

	bestIdx := -1
	for i := 0; i < len(g.Sections); i++ {
		if g.Sections[i].Area != areas[0] {
			continue
		}
		match := true
		for j := 1; j < len(areas); j++ {
			if i+j >= len(g.Sections) || g.Sections[i+j].Area != areas[j] {
				match = false
				break
			}
		}
		if match {
			bestIdx = i + len(areas) - 1
		}
	}

	if bestIdx != -1 {
		g.CurrentIndex = bestIdx
	} else {
		// Fall back to the last area as a single match.
		last := areas[len(areas)-1]
		for i, sec := range g.Sections {
			if sec.Area == last {
				bestIdx = i
			}
		}
		if bestIdx != -1 {
			g.CurrentIndex = bestIdx
		}
	}
}
