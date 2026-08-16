package xlsxexport

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

var thinBorder = []excelize.Border{
	{Type: "left", Color: "000000", Style: 1},
	{Type: "top", Color: "000000", Style: 1},
	{Type: "bottom", Color: "000000", Style: 1},
	{Type: "right", Color: "000000", Style: 1},
}

// RowStyle colors a data row (index 0 is the first row under the header).
// Empty FillRGB and FontRGB keep the default cell style.
type RowStyle struct {
	FillRGB string
	FontRGB string
}

// Build writes a single-sheet XLSX: bold centered header, AutoFilter, column
// widths from the longest cell, and borders on the used range.
func Build(sheetName string, headers []string, rows [][]string) ([]byte, error) {
	return BuildStyled(sheetName, headers, rows, nil)
}

func BuildStyled(sheetName string, headers []string, rows [][]string, rowStyles []RowStyle) ([]byte, error) {
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if name := f.GetSheetName(0); name != sheetName {
		if err := f.SetSheetName(name, sheetName); err != nil {
			return nil, err
		}
	}

	cols := len(headers)
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return writeFile(f)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder,
	})
	if err != nil {
		return nil, err
	}
	cellStyle, err := f.NewStyle(&excelize.Style{Border: thinBorder})
	if err != nil {
		return nil, err
	}
	styleIDs := map[string]int{"": cellStyle}

	widths := make([]int, cols)
	for i := 0; i < cols; i++ {
		val := ""
		if i < len(headers) {
			val = headers[i]
		}
		widths[i] = utf8.RuneCountInString(val)
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetCellValue(sheetName, cell, val); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(sheetName, cell, cell, headerStyle); err != nil {
			return nil, err
		}
	}
	for r, row := range rows {
		styleID := cellStyle
		if r < len(rowStyles) {
			id, err := rowStyleID(f, styleIDs, rowStyles[r])
			if err != nil {
				return nil, err
			}
			styleID = id
		}
		for c := 0; c < cols; c++ {
			val := ""
			if c < len(row) {
				val = row[c]
			}
			if n := utf8.RuneCountInString(val); n > widths[c] {
				widths[c] = n
			}
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return nil, err
			}
			if err := f.SetCellValue(sheetName, cell, val); err != nil {
				return nil, err
			}
			if err := f.SetCellStyle(sheetName, cell, cell, styleID); err != nil {
				return nil, err
			}
		}
	}

	lastCol, err := excelize.ColumnNumberToName(cols)
	if err != nil {
		return nil, err
	}
	lastRow := 1 + len(rows)
	if err := f.AutoFilter(sheetName, fmt.Sprintf("A1:%s%d", lastCol, lastRow), nil); err != nil {
		return nil, err
	}
	for i, w := range widths {
		name, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		width := float64(w) + 2
		if width < 8 {
			width = 8
		}
		if width > 255 {
			width = 255
		}
		if err := f.SetColWidth(sheetName, name, name, width); err != nil {
			return nil, err
		}
	}
	return writeFile(f)
}

func rowStyleID(f *excelize.File, cache map[string]int, style RowStyle) (int, error) {
	fill := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(style.FillRGB)), "#")
	font := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(style.FontRGB)), "#")
	key := fill + "/" + font
	if id, ok := cache[key]; ok {
		return id, nil
	}
	st := excelize.Style{Border: thinBorder}
	if fill != "" {
		st.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}}
	}
	if font != "" {
		st.Font = &excelize.Font{Color: font}
	}
	id, err := f.NewStyle(&st)
	if err != nil {
		return 0, err
	}
	cache[key] = id
	return id, nil
}

func writeFile(f *excelize.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
