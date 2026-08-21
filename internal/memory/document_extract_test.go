package memory

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDocumentText_PlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "Hello, this is a plain text file for embedding."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	extracted, err := extractDocumentText(path)
	if err != nil {
		t.Fatalf("extractDocumentText plain text failed: %v", err)
	}
	if strings.TrimSpace(extracted) != content {
		t.Fatalf("got %q, want %q", extracted, content)
	}
}

func TestExtractDocumentText_CSVAndTSV(t *testing.T) {
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "data.csv")
	csvContent := "Name,Age,Role\nAlice,30,Developer\nBob,25,Designer\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatal(err)
	}

	extractedCSV, err := extractDocumentText(csvPath)
	if err != nil {
		t.Fatalf("extractDocumentText CSV failed: %v", err)
	}
	if !strings.Contains(extractedCSV, "Alice | 30 | Developer") {
		t.Fatalf("unexpected CSV extracted content: %s", extractedCSV)
	}

	tsvPath := filepath.Join(dir, "data.tsv")
	tsvContent := "Name\tScore\nCharlie\t95\n"
	if err := os.WriteFile(tsvPath, []byte(tsvContent), 0644); err != nil {
		t.Fatal(err)
	}

	extractedTSV, err := extractDocumentText(tsvPath)
	if err != nil {
		t.Fatalf("extractDocumentText TSV failed: %v", err)
	}
	if !strings.Contains(extractedTSV, "Charlie | 95") {
		t.Fatalf("unexpected TSV extracted content: %s", extractedTSV)
	}
}

func TestExtractDocumentText_Docx(t *testing.T) {
	dir := t.TempDir()
	docxPath := filepath.Join(dir, "sample.docx")

	// Create a minimal valid docx zip structure in memory
	f, err := os.Create(docxPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Hello from ActonOS Word document.</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Second paragraph of text.</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`
	if _, err := w.Write([]byte(xmlDoc)); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	_ = f.Close()

	extracted, err := extractDocumentText(docxPath)
	if err != nil {
		t.Fatalf("extractDocumentText docx failed: %v", err)
	}
	if !strings.Contains(extracted, "Hello from ActonOS Word document") || !strings.Contains(extracted, "Second paragraph of text") {
		t.Fatalf("unexpected docx content: %s", extracted)
	}
}

func TestExtractDocumentText_Xlsx(t *testing.T) {
	dir := t.TempDir()
	xlsxPath := filepath.Join(dir, "sample.xlsx")

	// Create a minimal valid xlsx zip structure in memory
	f, err := os.Create(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("xl/sharedStrings.xml")
	if err != nil {
		t.Fatal(err)
	}
	xmlSheet := `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <si><t>Product</t></si>
  <si><t>Revenue</t></si>
  <si><t>ActonOS MiniPC</t></si>
</sst>`
	if _, err := w.Write([]byte(xmlSheet)); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	_ = f.Close()

	extracted, err := extractDocumentText(xlsxPath)
	if err != nil {
		t.Fatalf("extractDocumentText xlsx failed: %v", err)
	}
	if !strings.Contains(extracted, "Product") || !strings.Contains(extracted, "ActonOS MiniPC") {
		t.Fatalf("unexpected xlsx content: %s", extracted)
	}
}

func TestExtractDocumentText_BinaryRejection(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0x03}, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := extractDocumentText(binPath)
	if err == nil {
		t.Fatal("expected binary file to fail extraction")
	}
}
