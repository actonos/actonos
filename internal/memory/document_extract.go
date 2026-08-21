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
)

// extractDocumentBytes extracts indexable text from database-backed workspace
// bytes. Complex readers that require a filename receive a private temporary
// file which is removed before this function returns and is never exposed to a
// tool, API response, semantic source, or audit event.
func extractDocumentBytes(name, mimeType string, data []byte) (string, error) {
	if len(data) > maxEmbeddingFileSize {
		return "", fmt.Errorf("%w: file exceeds limit of %d bytes", errUnsupportedEmbeddingSource, maxEmbeddingFileSize)
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "application/pdf":
		ext = ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		ext = ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		ext = ".xlsx"
	case "text/csv":
		ext = ".csv"
	case "text/tab-separated-values":
		ext = ".tsv"
	}
	if ext != ".pdf" && ext != ".docx" && ext != ".xlsx" && ext != ".csv" && ext != ".tsv" {
		return extractPlainTextBytes(data)
	}
	temp, err := os.CreateTemp("", "actonos-workspace-*"+ext)
	if err != nil {
		return "", fmt.Errorf("creating private extraction file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("writing private extraction file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("closing private extraction file: %w", err)
	}
	return extractDocumentText(tempPath)
}

func extractPlainTextBytes(data []byte) (string, error) {
	// Check for UTF-16 BOMs.
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			runes := make([]rune, 0, len(data)/2)
			for i := 2; i+1 < len(data); i += 2 {
				runes = append(runes, rune(uint16(data[i])|uint16(data[i+1])<<8))
			}
			return string(runes), nil
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			runes := make([]rune, 0, len(data)/2)
			for i := 2; i+1 < len(data); i += 2 {
				runes = append(runes, rune(uint16(data[i])<<8|uint16(data[i+1])))
			}
			return string(runes), nil
		}
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("%w: binary file", errUnsupportedEmbeddingSource)
	}
	return string(data), nil
}

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
	return extractPlainTextBytes(data)
}
