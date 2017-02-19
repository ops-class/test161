package main

import (
	"errors"
	"fmt"
	color "gopkg.in/fatih/color.v0"
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

type Cell struct {
	Text      string
	CellColor *color.Color
}

type Row []*Cell
type Rows []Row

type PrintConfig struct {
	NumSpaceSep   int
	UnderlineChar string
	BoldHeadings  bool
}

type PrintData struct {
	Headings []*Heading
	Rows     Rows
	Config   PrintConfig
}

////////////////////////////////////////////////////////////////////////////////

var defaultPrintConf = PrintConfig{
	NumSpaceSep:   3,
	UnderlineChar: "-",
	BoldHeadings:  true,
}

// Get the number of columns available on the terminal. We'll try to squeeze
// the results into this width.
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
func (pd *PrintData) calcWidths() {
	// First pass, get the max and min widths of each column. Best case scenerio,
	// we fit within the real estate we have. Worst case scenerio, the min width
	// is smaller than what we have to work with.

	// Start with the headings, and don't break them up
	for _, h := range pd.Headings {
		h.min = h.MinWidth
		if h.min < len(h.Text) {
			h.min = len(h.Text)
		}
		h.max = h.min
	}

	// Now, the rows. The min column width should be the shortest word we find.
	// The max width is the width of the longest cell.
	for _, row := range pd.Rows {
		for col, cell := range row {
			lw := longestWordLen(cell.Text)
			if pd.Headings[col].min < lw {
				pd.Headings[col].min = lw
			}
			if len(cell.Text) > pd.Headings[col].max {
				pd.Headings[col].max = len(cell.Text)
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
	remaining -= (len(pd.Headings) - 1) * pd.Config.NumSpaceSep

	// Start out by setting the widths to the min widths.
	// (OK if remaining goes negative)
	for _, h := range pd.Headings {
		h.width = h.min
		remaining -= h.width
	}

	// Divide up the remaining columns equally
	for remaining > 0 {
		didOne := false
		for _, h := range pd.Headings {
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
func (cell *Cell) split(width int) []*Cell {
	// We currently use the simple (and common) greedy algorithm for
	// filling each row.
	// TODO: Change this to Knuth's algorithm for minumum raggedness

	res := make([]*Cell, 0)
	remaining := width
	line := ""
	words := strings.Split(cell.Text, " ")

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
			res = append(res, &Cell{line, cell.CellColor})
			remaining = width - len(word)
			line = word
		}
	}

	// Make sure the last line gets added
	if len(line) > 0 {
		res = append(res, &Cell{line, cell.CellColor})
	}

	return res
}

// Split this row if it that has any cells that are too long for their column.
func (row Row) split(headings []*Heading) []Row {
	newRows := make([]Row, 0)

	splits := make([][]*Cell, len(row))
	numLines := 0
	for i, cell := range row {
		splits[i] = cell.split(headings[i].width)
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
		newRow := make(Row, len(row))

		for col, _ := range row {
			if i < len(splits[col]) {
				newRow[col] = splits[col][i]
			} else {
				newRow[col] = &Cell{"", nil}
			}
		}
		newRows = append(newRows, newRow)
	}

	return newRows
}

// Split any rows that have cells that are too long for their column.
func (pd *PrintData) splitRows() {
	newRows := make(Rows, 0)

	for _, row := range pd.Rows {
		rows := row.split(pd.Headings)
		newRows = append(newRows, rows...)
	}

	pd.Rows = newRows
}

func (pd *PrintData) Print() error {
	// Do we have the right number of columns in the data? This is a
	// programming error if we don't.
	for _, row := range pd.Rows {
		if len(row) != len(pd.Headings) {
			return errors.New("Wrong number of columns")
		}
	}

	// Calculate min/max column widths and split up cells if needed
	pd.calcWidths()
	pd.splitRows()

	// Next compute the format string for each cell
	fmtStrings := make([]string, 0, len(pd.Headings))
	for i, h := range pd.Headings {
		fmtStr := "%"
		if !h.RightJustified {
			fmtStr += "-"
		}
		fmtStr += fmt.Sprintf("%v", h.width)
		fmtStr += "v"
		if i+1 < len(pd.Headings) && pd.Config.NumSpaceSep > 0 {
			fmtStr += strings.Repeat(" ", pd.Config.NumSpaceSep)
		} else {
			fmtStr += "\n"
		}
		fmtStrings = append(fmtStrings, fmtStr)
	}

	// Print heading
	bold := color.New(color.Bold)

	for i, h := range pd.Headings {
		if pd.Config.BoldHeadings {
			bold.Printf(fmtStrings[i], h.Text)
		} else {
			fmt.Printf(fmtStrings[i], h.Text)
		}
	}

	// Underlines
	if pd.Config.UnderlineChar != "" {
		for i, h := range pd.Headings {
			fmt.Printf(fmtStrings[i], strings.Repeat(pd.Config.UnderlineChar, h.width))
		}
	}

	for _, row := range pd.Rows {
		for col, cell := range row {
			if cell.CellColor != nil {
				cell.CellColor.Printf(fmtStrings[col], cell.Text)
			} else {
				fmt.Printf(fmtStrings[col], cell.Text)
			}
		}
	}

	return nil
}
