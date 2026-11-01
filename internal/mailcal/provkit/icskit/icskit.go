// Package icskit reads and writes iCalendar (RFC 5545) as a tree of components and properties.
// It keeps every property it does not understand, so changing an event's title does not drop
// its alarms, time-zone definitions or vendor extensions.
package icskit

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "time/tzdata" // zone names in TZID must resolve inside WebAssembly, where there is no system database
)

// Param is one property parameter, for example TZID=Europe/Paris.
type Param struct {
	Name   string
	Values []string
}

// Prop is one content line.
type Prop struct {
	Name   string // upper-case
	Params []Param
	Value  string // still escaped as on the wire
}

// Component is BEGIN:NAME ... END:NAME.
type Component struct {
	Name     string
	Props    []Prop
	Children []*Component
}

// ErrMalformed is returned for text that is not valid iCalendar.
var ErrMalformed = errors.New("malformed iCalendar")

const (
	maxDepth = 8
	maxLines = 200000
)

// unfold joins continuation lines (a line starting with a space or tab continues the last one).
func unfold(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1] += line[1:]
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitParams(rest string) ([]Param, error) {
	var params []Param
	for rest != "" {
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("%w: parameter without a value", ErrMalformed)
		}
		p := Param{Name: strings.ToUpper(rest[:eq])}
		rest = rest[eq+1:]
		for {
			var v string
			if strings.HasPrefix(rest, `"`) {
				end := strings.IndexByte(rest[1:], '"')
				if end < 0 {
					return nil, fmt.Errorf("%w: unterminated quote", ErrMalformed)
				}
				v, rest = rest[1:1+end], rest[end+2:]
			} else {
				end := strings.IndexAny(rest, ",;")
				if end < 0 {
					end = len(rest)
				}
				v, rest = rest[:end], rest[end:]
			}
			p.Values = append(p.Values, v)
			if strings.HasPrefix(rest, ",") {
				rest = rest[1:]
				continue
			}
			break
		}
		params = append(params, p)
		rest = strings.TrimPrefix(rest, ";")
	}
	return params, nil
}

// splitName finds the first ':' or ';' that is not inside a quoted parameter value.
func parseLine(line string) (Prop, error) {
	inQuote := false
	colon := -1
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon <= 0 {
		return Prop{}, fmt.Errorf("%w: line without a colon", ErrMalformed)
	}
	head, value := line[:colon], line[colon+1:]
	name, rest, _ := strings.Cut(head, ";")
	p := Prop{Name: strings.ToUpper(strings.TrimSpace(name)), Value: value}
	if p.Name == "" {
		return Prop{}, fmt.Errorf("%w: empty property name", ErrMalformed)
	}
	if rest != "" {
		params, err := splitParams(rest)
		if err != nil {
			return Prop{}, err
		}
		p.Params = params
	}
	return p, nil
}

// Parse reads one top-level component (normally VCALENDAR).
func Parse(text string) (*Component, error) {
	lines := unfold(text)
	if len(lines) > maxLines {
		return nil, fmt.Errorf("%w: too many lines", ErrMalformed)
	}
	var stack []*Component
	var root *Component
	for _, line := range lines {
		p, err := parseLine(line)
		if err != nil {
			return nil, err
		}
		switch p.Name {
		case "BEGIN":
			c := &Component{Name: strings.ToUpper(p.Value)}
			if len(stack) >= maxDepth {
				return nil, fmt.Errorf("%w: nesting too deep", ErrMalformed)
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, c)
			} else if root != nil {
				return nil, fmt.Errorf("%w: more than one top-level component", ErrMalformed)
			} else {
				root = c
			}
			stack = append(stack, c)
		case "END":
			if len(stack) == 0 || stack[len(stack)-1].Name != strings.ToUpper(p.Value) {
				return nil, fmt.Errorf("%w: unbalanced END:%s", ErrMalformed, p.Value)
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("%w: property outside a component", ErrMalformed)
			}
			top := stack[len(stack)-1]
			top.Props = append(top.Props, p)
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("%w: missing END", ErrMalformed)
	}
	return root, nil
}

// Find returns the first child component with the given name.
func (c *Component) Find(name string) *Component {
	for _, ch := range c.Children {
		if ch.Name == name {
			return ch
		}
	}
	return nil
}

// All returns every child component with the given name.
func (c *Component) All(name string) []*Component {
	var out []*Component
	for _, ch := range c.Children {
		if ch.Name == name {
			out = append(out, ch)
		}
	}
	return out
}

// Get returns the first property called name.
func (c *Component) Get(name string) (Prop, bool) {
	for _, p := range c.Props {
		if p.Name == name {
			return p, true
		}
	}
	return Prop{}, false
}

// Values returns every property called name.
func (c *Component) Values(name string) []Prop {
	var out []Prop
	for _, p := range c.Props {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// Param returns the first value of a parameter.
func (p Prop) Param(name string) string {
	for _, q := range p.Params {
		if q.Name == name && len(q.Values) > 0 {
			return q.Values[0]
		}
	}
	return ""
}

// Set replaces every property called name by one with the given (already escaped) value.
func (c *Component) Set(name, value string, params ...Param) {
	c.Remove(name)
	c.Props = append(c.Props, Prop{Name: name, Params: params, Value: value})
}

// Remove deletes every property called name.
func (c *Component) Remove(name string) {
	out := c.Props[:0:0]
	for _, p := range c.Props {
		if p.Name != name {
			out = append(out, p)
		}
	}
	c.Props = out
}

// Escape encodes text for a TEXT value.
func Escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// Unescape decodes a TEXT value.
func Unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func writeLine(buf *bytes.Buffer, line string) {
	// Fold at 75 octets without cutting a UTF-8 sequence.
	first := true
	for len(line) > 0 {
		limit := 75
		if !first {
			limit = 74
		}
		if len(line) <= limit {
			if !first {
				buf.WriteByte(' ')
			}
			buf.WriteString(line)
			buf.WriteString("\r\n")
			return
		}
		cut := limit
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if !first {
			buf.WriteByte(' ')
		}
		buf.WriteString(line[:cut])
		buf.WriteString("\r\n")
		line = line[cut:]
		first = false
	}
}

func (p Prop) line() string {
	var b strings.Builder
	b.WriteString(p.Name)
	for _, q := range p.Params {
		b.WriteByte(';')
		b.WriteString(q.Name)
		b.WriteByte('=')
		for i, v := range q.Values {
			if i > 0 {
				b.WriteByte(',')
			}
			if strings.ContainsAny(v, ",;:") {
				b.WriteString(`"` + v + `"`)
			} else {
				b.WriteString(v)
			}
		}
	}
	b.WriteByte(':')
	b.WriteString(p.Value)
	return b.String()
}

// Serialize writes the component with CRLF line endings and folded lines.
func (c *Component) Serialize() []byte {
	var buf bytes.Buffer
	var write func(*Component)
	write = func(c *Component) {
		writeLine(&buf, "BEGIN:"+c.Name)
		for _, p := range c.Props {
			if strings.ContainsAny(p.Value, "\r\n") || strings.ContainsAny(p.Name, "\r\n:;") {
				continue // never emit a value that could start a new line (injection)
			}
			writeLine(&buf, p.line())
		}
		for _, ch := range c.Children {
			write(ch)
		}
		writeLine(&buf, "END:"+c.Name)
	}
	write(c)
	return buf.Bytes()
}

// ParseTime reads a DATE or DATE-TIME property value. A value with a TZID parameter is read in
// that zone; a floating value is read as UTC. allDay is true for DATE values.
func ParseTime(p Prop) (t time.Time, allDay bool, err error) {
	v := strings.TrimSpace(p.Value)
	switch {
	case p.Param("VALUE") == "DATE" || len(v) == 8:
		t, err = time.Parse("20060102", v)
		return t, true, err
	case strings.HasSuffix(v, "Z"):
		t, err = time.Parse("20060102T150405Z", v)
		return t.UTC(), false, err
	}
	loc := time.UTC
	if tz := p.Param("TZID"); tz != "" {
		l, lerr := time.LoadLocation(tz)
		if lerr != nil {
			return time.Time{}, false, fmt.Errorf("%w: unknown time zone %q", ErrMalformed, tz)
		}
		loc = l
	}
	t, err = time.ParseInLocation("20060102T150405", v, loc)
	return t.UTC(), false, err
}

// UTC formats a time as a UTC DATE-TIME value.
func UTC(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// Date formats a DATE value.
func Date(t time.Time) string { return t.Format("20060102") }
