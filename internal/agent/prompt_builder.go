package agent

import (
	"fmt"
	"strings"
)

// PromptSection represents an isolated cognitive layer capable of rendering structured XML.
type PromptSection interface {
	Name() string
	Render() string
	IsEmpty() bool
}

// PromptBuilder orchestrates section composition, token budgeting, and delimiter formatting.
type PromptBuilder struct {
	sections []PromptSection
}

// RawTextSection wraps arbitrary raw text as a PromptSection.
type RawTextSection struct {
	Content string
}

func (s *RawTextSection) Name() string   { return "raw" }
func (s *RawTextSection) IsEmpty() bool  { return strings.TrimSpace(s.Content) == "" }
func (s *RawTextSection) Render() string { return strings.TrimSpace(s.Content) }

// NewPromptBuilder creates an empty fluent prompt builder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		sections: make([]PromptSection, 0, 8),
	}
}

// WithSection appends a section if it is non-nil and not empty.
func (b *PromptBuilder) WithSection(section PromptSection) *PromptBuilder {
	if section != nil && !section.IsEmpty() {
		b.sections = append(b.sections, section)
	}
	return b
}

// Build renders all non-empty sections into a coherent, structured prompt.
func (b *PromptBuilder) Build() string {
	var sb strings.Builder
	for i, s := range b.sections {
		if s == nil || s.IsEmpty() {
			continue
		}
		rendered := strings.TrimSpace(s.Render())
		if rendered == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(rendered)
	}
	return sb.String()
}

// XMLTag wraps content within an opening and closing XML tag with optional attributes.
func XMLTag(tag string, attributes map[string]string, content string) string {
	var attrStr string
	if len(attributes) > 0 {
		var attrs []string
		for k, v := range attributes {
			if v != "" {
				attrs = append(attrs, fmt.Sprintf("%s=\"%s\"", k, EscapeXMLAttribute(v)))
			}
		}
		if len(attrs) > 0 {
			attrStr = " " + strings.Join(attrs, " ")
		}
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Sprintf("<%s%s />", tag, attrStr)
	}

	return fmt.Sprintf("<%s%s>\n%s\n</%s>", tag, attrStr, indentContent(trimmed, "  "), tag)
}

// SimpleTag wraps content in a simple XML tag with no attributes.
func SimpleTag(tag string, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("<%s>\n%s\n</%s>", tag, indentContent(trimmed, "  "), tag)
}

// EscapeXMLAttribute escapes special characters in XML attributes.
func EscapeXMLAttribute(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"\n", " ",
		"\r", "",
	)
	return r.Replace(s)
}

func indentContent(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n")
}
