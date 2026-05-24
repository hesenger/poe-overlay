package main

import (
	"bufio"
	"os"
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
	Area  string
	Steps []Step
}

type Guide struct {
	Sections     []Section
	CurrentIndex int
}

func LoadGuide(path string) (*Guide, error) {
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
		g, err := LoadGuide(path)
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
	// Look behind for the most recent occurrence within a small window
	for i := g.CurrentIndex - 1; i >= 0 && i >= g.CurrentIndex-5; i-- {
		if g.Sections[i].Area == area {
			g.CurrentIndex = i
			return true
		}
	}
	// Look ahead for the nearest occurrence within a small window
	for i := g.CurrentIndex + 2; i < len(g.Sections) && i <= g.CurrentIndex+5; i++ {
		if g.Sections[i].Area == area {
			g.CurrentIndex = i
			return true
		}
	}
	// Not found nearby — scan the entire guide for the closest match.
	bestIdx := -1
	bestDist := len(g.Sections)
	for i, sec := range g.Sections {
		if sec.Area == area {
			dist := i - g.CurrentIndex
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
	}
	if bestIdx != -1 {
		g.CurrentIndex = bestIdx
		return true
	}
	return false
}

func (g *Guide) CurrentSection() Section {
	if g.CurrentIndex < 0 || g.CurrentIndex >= len(g.Sections) {
		return Section{}
	}
	return g.Sections[g.CurrentIndex]
}
