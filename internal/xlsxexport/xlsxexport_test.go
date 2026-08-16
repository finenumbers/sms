package xlsxexport

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBuildHeaderFilterWidthBorder(t *testing.T) {
	raw, err := Build("items", []string{"Номер", "Ошибка"}, [][]string{
		{"+79001234567", "коротко"},
		{"+79001111111", "очень длинное сообщение ошибки проверки"},
	})
	if err != nil || len(raw) < 100 {
		t.Fatalf("build %v %d", err, len(raw))
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if f.GetSheetName(0) != "items" {
		t.Fatalf("sheet %s", f.GetSheetName(0))
	}
	got, err := f.GetCellValue("items", "A1")
	if err != nil || got != "Номер" {
		t.Fatalf("A1 %q %v", got, err)
	}
	got, err = f.GetCellValue("items", "B2")
	if err != nil || got != "коротко" {
		t.Fatalf("B2 %q %v", got, err)
	}

	styleID, err := f.GetCellStyle("items", "A1")
	if err != nil {
		t.Fatal(err)
	}
	style, err := f.GetStyle(styleID)
	if err != nil || style == nil || style.Font == nil || !style.Font.Bold {
		t.Fatalf("header bold %#v %v", style, err)
	}
	if style.Alignment == nil || style.Alignment.Horizontal != "center" {
		t.Fatalf("header align %#v", style.Alignment)
	}
	if len(style.Border) < 4 {
		t.Fatalf("header border %#v", style.Border)
	}

	cellStyleID, err := f.GetCellStyle("items", "A2")
	if err != nil {
		t.Fatal(err)
	}
	cellStyle, err := f.GetStyle(cellStyleID)
	if err != nil || cellStyle == nil || len(cellStyle.Border) < 4 {
		t.Fatalf("cell border %#v %v", cellStyle, err)
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	foundFilter := false
	for _, file := range zr.File {
		if !strings.Contains(file.Name, "sheet") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("autoFilter")) {
			foundFilter = true
			break
		}
	}
	if !foundFilter {
		t.Fatal("expected autoFilter in worksheet")
	}

	w, err := f.GetColWidth("items", "B")
	if err != nil {
		t.Fatal(err)
	}
	if w < 20 {
		t.Fatalf("col B width %v, want longest error text", w)
	}
}
