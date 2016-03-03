package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Heading struct {
	Text           string
	RightJustified bool
	MinWidth       int

	// Calculated/working values
	width  int
	format string
	min    int
	max    int
}

type Rows [][]string

type PrintConfig struct {
	NumSpaceSep   int
	UnderlineChar string
}

var defaultPrintConf = PrintConfig{
	NumSpaceSep:   3,
	UnderlineChar: "-",
}

// Get the number of columns available on the terminal. We'll try to squeeze the results
// into this width.
func numTTYColumns() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	res, err := cmd.Output()
	if err == nil {
		tokens := strings.Split(strings.TrimSpace(string(res)), " ")
		if len(tokens) == 2 {
			cols, _ := strconv.Atoi(tokens[1])
			return cols
		}
	}

	return -1
}

// Get the length of the longest word in a line
func longestWordLen(line string) int {
	start, longest, pos := 0, 0, 0
	for pos <= len(line) {
		// Break char or EOL
		if pos == len(line) || line[pos] == ' ' {
			// (pos-1) - start + 1
			wordlen := pos - start
			start = pos + 1
			pos = start

			if wordlen > longest {
				longest = wordlen
			}
		} else {
			pos += 1
		}
	}
	return longest
}

// Calculate the final widths of each column.
func calcWidths(headings []*Heading, rows Rows, config PrintConfig) {

	// First pass, get the max and min widths of each column. Best case scenerio,
	// we fit within the real estate we have. Worst case scenerio, the min width
	// is smaller than what we have to work with.

	// Start with the headings, and don't break them up
	for _, h := range headings {
		h.min = h.MinWidth
		if h.min < len(h.Text) {
			h.min = len(h.Text)
		}
		h.max = h.min
	}

	// Now, the rows. The min column width should be the shortest word we find.
	// The max width is the width of the longest cell.
	for _, row := range rows {
		for col, cell := range row {
			lw := longestWordLen(cell)
			if headings[col].min < lw {
				headings[col].min = lw
			}
			if len(cell) > headings[col].max {
				headings[col].max = len(cell)
			}
		}
	}

	// Figure out how many columns we have to work with. But, at least give
	// us something to work with.
	remaining := numTTYColumns()
	if remaining < 80 {
		remaining = 80
	}

	// Deduct the column spacers
	remaining -= (len(headings) - 1) * config.NumSpaceSep

	// Start out by setting the widths to the min widths.
	// (OK if remaining goes negative)
	for _, h := range headings {
		h.width = h.min
		remaining -= h.width
	}

	// Divide up the remaining columns equally
	for remaining > 0 {
		didOne := false
		for _, h := range headings {
			if h.width < h.max && remaining > 0 {
				remaining -= 1
				h.width += 1
				didOne = true
			}
		}
		if !didOne {
			break
		}
	}

}

// Split a single-line cell into (possibly) mutiple cells, with one line per cell.
func splitCell(cell string, width int) []string {
	// We currently use the simple (and common) greedy algorithm for
	// filling each row.
	// TODO: Change this to Knuth's algorithm for minumum raggedness

	res := make([]string, 0)
	remaining := width
	line := ""
	words := strings.Split(cell, " ")

	for _, word := range words {
		if len(line) == 0 {
			// First word
			line = word
			remaining = width - len(word)
		} else if remaining >= len(word)+1 {
			// The word + space fits in the current line
			remaining -= (len(word) + 1)
			line += " " + word
		} else {
			// The word doesn't fit; finish the old line and start a new one.
			res = append(res, line)
			remaining = width - len(word)
			line = word
		}
	}

	// Make sure the last line gets added
	if len(line) > 0 {
		res = append(res, line)
	}

	return res
}

// Split any rows that have cells that are too long for their column.
func splitRows(headings []*Heading, rows Rows) Rows {

	newRows := make([][]string, 0)

	for _, row := range rows {
		splits := make([][]string, len(row))
		numLines := 0
		for i, cell := range row {
			splits[i] = splitCell(cell, headings[i].width)
			if len(splits[i]) > numLines {
				numLines = len(splits[i])
			}
		}

		// At this point we have something like this:
		// [    ]  [      ]  [        ]
		// [    ]            [        ]
		//                   [        ]
		//                   [        ]
		//
		// Each cell has been broken up into columnar slice, and we now have
		// to piece back together rows. We need to make sure each new row
		// has the right number of columns, even if we have blank cells.
		// We iterate over the rows, and if the cell split has that row,
		// we add it, otherwise we just add a placeholder.
		for i := 0; i < numLines; i++ {
			// The new row has to have the same number of columns
			newRow := make([]string, len(row))

			for col, _ := range row {
				if i < len(splits[col]) {
					newRow[col] = splits[col][i]
				} else {
					newRow[col] = ""
				}
			}
			newRows = append(newRows, newRow)
		}
	}
	return newRows
}

func printColumns(headings []*Heading, rows Rows, config PrintConfig) error {
	// Do we have the right number of columns in the data? This is a
	// programming error if we don't.
	for _, row := range rows {
		if len(row) != len(headings) {
			return errors.New("Wrong number of columns")
		}
	}

	// Calculate min/max column widths and split up cells if needed
	calcWidths(headings, rows, config)
	rows = splitRows(headings, rows)

	// Next compute the format string for each row
	fmtStr := ""
	for i, h := range headings {
		fmtStr += "%"
		if !h.RightJustified {
			fmtStr += "-"
		}
		fmtStr += fmt.Sprintf("%v", h.width)
		fmtStr += "v"
		if i+1 < len(headings) && config.NumSpaceSep > 0 {
			fmtStr += strings.Repeat(" ", config.NumSpaceSep)
		}
	}
	fmtStr += "\n"

	// Print heading
	row := make([]interface{}, len(headings))
	for i, h := range headings {
		row[i] = h.Text
	}
	fmt.Printf(fmtStr, row...)

	// Underlines
	if config.UnderlineChar != "" {
		for i, h := range headings {
			row[i] = strings.Repeat(config.UnderlineChar, h.width)
		}
		fmt.Printf(fmtStr, row...)
	}

	for _, stringRow := range rows {
		for col, text := range stringRow {
			row[col] = text
		}
		fmt.Printf(fmtStr, row...)
	}

	return nil
}
