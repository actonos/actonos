package memory

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// extractDocumentText extracts readable plain text from various document formats (PDF, DOCX, XLSX, CSV, TSV, TXT, etc.).
func extractDocumentText(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		return extractPDFText(filePath)
	case ".docx":
		return extractDocxText(filePath)
	case ".xlsx":
		return extractXlsxText(filePath)
	case ".csv":
		return extractCSVText(filePath, ',')
	case ".tsv":
		return extractCSVText(filePath, '\t')
	default:
		return extractPlainText(filePath)
	}
}

func extractPDFText(filePath string) (string, error) {
	file, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: opening pdf: %v", errUnsupportedEmbeddingSource, err)
	}
	defer file.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("%w: extracting pdf plain text: %v", errUnsupportedEmbeddingSource, err)
	}

	var sb strings.Builder
	if _, err := io.Copy(&sb, reader); err != nil {
		return "", fmt.Errorf("%w: reading pdf stream: %v", errUnsupportedEmbeddingSource, err)
	}
	return sb.String(), nil
}

func extractDocxText(filePath string) (string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: opening docx: %v", errUnsupportedEmbeddingSource, err)
	}
	defer r.Close()

	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("%w: invalid docx (missing word/document.xml)", errUnsupportedEmbeddingSource)
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var sb strings.Builder
	decoder := xml.NewDecoder(rc)
	inText := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			} else if t.Name.Local == "br" || t.Name.Local == "tab" {
				sb.WriteString(" ")
			} else if t.Name.Local == "p" {
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			} else if t.Name.Local == "p" {
				if !strings.HasSuffix(sb.String(), "\n") {
					sb.WriteString("\n")
				}
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return sb.String(), nil
}

func extractXlsxText(filePath string) (string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: opening xlsx: %v", errUnsupportedEmbeddingSource, err)
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		if f.Name == "xl/sharedStrings.xml" || (strings.HasPrefix(f.Name, "xl/worksheets/sheet") && strings.HasSuffix(f.Name, ".xml")) {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			dec := xml.NewDecoder(rc)
			inText := false
			for {
				tok, err := dec.Token()
				if err != nil {
					break
				}
				switch t := tok.(type) {
				case xml.StartElement:
					if t.Name.Local == "t" || t.Name.Local == "v" {
						inText = true
					} else if t.Name.Local == "row" {
						if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
							sb.WriteString("\n")
						}
					}
				case xml.EndElement:
					if t.Name.Local == "t" || t.Name.Local == "v" {
						inText = false
						sb.WriteString(" ")
					} else if t.Name.Local == "row" {
						sb.WriteString("\n")
					}
				case xml.CharData:
					if inText {
						sb.Write(t)
					}
				}
			}
			rc.Close()
		}
	}
	return sb.String(), nil
}

func extractCSVText(filePath string, comma rune) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var sb strings.Builder
	for {
		record, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// If CSV parser fails, fallback to reading as raw plain text
			_, _ = f.Seek(0, io.SeekStart)
			data, readErr := io.ReadAll(f)
			if readErr == nil {
				return string(data), nil
			}
			return "", err
		}
		sb.WriteString(strings.Join(record, " | "))
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func extractPlainText(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	// Check for UTF-16 BOMs
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			// UTF-16 LE
			runes := make([]rune, 0, len(data)/2)
			for i := 2; i+1 < len(data); i += 2 {
				runes = append(runes, rune(uint16(data[i])|uint16(data[i+1])<<8))
			}
			return string(runes), nil
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			// UTF-16 BE
			runes := make([]rune, 0, len(data)/2)
			for i := 2; i+1 < len(data); i += 2 {
				runes = append(runes, rune(uint16(data[i])<<8|uint16(data[i+1])))
			}
			return string(runes), nil
		}
	}
	// Check for binary null byte
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("%w: binary file", errUnsupportedEmbeddingSource)
	}
	return string(data), nil
}
