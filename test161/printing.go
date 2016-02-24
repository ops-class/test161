package main

import (
	"errors"
	"fmt"
	"strings"
)

type Heading struct {
	Text          string
	LeftJustified bool
	MinWidth      int
	width         int
	format        string
}

type PrintConfig struct {
	NumSpaceSep   int
	UnderlineChar string
}

var defaultPrintConf = PrintConfig{
	NumSpaceSep:   3,
	UnderlineChar: "-",
}

func printColumns(headings []*Heading, rows [][]string, config PrintConfig) error {
	// Do we have the right number of columns in the data?
	// While we're at it, compute the max with of each.
	expected := len(headings)
	for _, h := range headings {
		h.width = len(h.Text)
	}
	for _, row := range rows {
		if len(row) != expected {
			return errors.New("Wrong number of columns")
		}
		for i, cell := range row {
			if headings[i].width < len(cell) {
				headings[i].width = len(cell)
			}
			if headings[i].width < headings[i].MinWidth {
				headings[i].width = headings[i].MinWidth
			}
		}
	}

	// Next compute the format string for each row
	fmtStr := ""
	for i, h := range headings {
		fmtStr += "%"
		if h.LeftJustified {
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
