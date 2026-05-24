package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
)

var sceneRegex = regexp.MustCompile(`\[SCENE\] Set Source \[(.*?)\]`)

func main() {
	const path = `C:\Program Files (x86)\Grinding Gear Games\Path of Exile 2\logs\Client.txt`

	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open Client.txt: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	scenes := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := sceneRegex.FindStringSubmatch(line); matches != nil {
			scene := matches[1]
			if scene != "" && scene != "(null)" {
				scenes[scene]++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var list []string
	for s := range scenes {
		list = append(list, s)
	}
	sort.Strings(list)

	fmt.Printf("Found %d unique scene(s):\n\n", len(list))
	for _, s := range list {
		fmt.Printf("  %-40s (%d occurrence(s))\n", s, scenes[s])
	}
}
