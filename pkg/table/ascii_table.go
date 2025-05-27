package table

import (
	"fmt"
	"strings"
)

const (
	boldWhite = "\033[1;37m"
	reset     = "\033[0m"
)

// Print prints an aligned ASCII table with bold white headers.
func Print(headers []string, rows [][]string) {
	// Calculate max column widths from headers and data
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if l := len(cell); l > colWidths[i] {
				colWidths[i] = l
			}
		}
	}

	printBorder := func() {
		fmt.Print("+")
		for _, w := range colWidths {
			fmt.Print(strings.Repeat("-", w+2) + "+")
		}
		fmt.Println()
	}

	printRow := func(cells []string, styleHeader bool) {
		fmt.Print("|")
		for i, cell := range cells {
			if styleHeader {
				fmt.Printf(" %s%-*s%s |", boldWhite, colWidths[i], cell, reset)
			} else {
				fmt.Printf(" %-*s |", colWidths[i], cell)
			}
		}
		fmt.Println()
	}

	// Render table
	printBorder()
	printRow(headers, true) // styled header row
	printBorder()
	for _, row := range rows {
		printRow(row, false)
	}
	printBorder()
}

// stripANSI removes ANSI escape codes from a string for width calculation.
func stripANSI(s string) string {
	// Remove ANSI escape codes for width calculation
	return strings.ReplaceAll(strings.ReplaceAll(s, boldWhite, ""), reset, "")
}
