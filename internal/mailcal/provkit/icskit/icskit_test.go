package icskit

import (
	"strings"
	"testing"
	"time"
)

const sample = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//\r\nBEGIN:VTIMEZONE\r\nTZID:Europe/Paris\r\nEND:VTIMEZONE\r\n" +
	"BEGIN:VEVENT\r\nUID:abc-123\r\nSUMMARY:Review\\, with notes\\nline two\r\nDTSTART;TZID=Europe/Paris:20261103T100000\r\n" +
	"DTEND;TZID=Europe/Paris:20261103T110000\r\nX-VENDOR-THING;X-P=\"a:b\":keep me\r\n" +
	"ATTENDEE;CN=\"Doe, Jane\";PARTSTAT=NEEDS-ACTION:mailto:jane@partner.com\r\n" +
	"DESCRIPTION:A very long line that must be folded because it is much longer than seventy-five octets and\r\n  continues here\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestParseKeepsEverythingAndRoundTrips(t *testing.T) {
	cal, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	ev := cal.Find("VEVENT")
	if ev == nil {
		t.Fatal("no VEVENT")
	}
	if s, _ := ev.Get("SUMMARY"); Unescape(s.Value) != "Review, with notes\nline two" {
		t.Fatalf("summary %q", s.Value)
	}
	att, _ := ev.Get("ATTENDEE")
	if att.Param("CN") != "Doe, Jane" || att.Param("PARTSTAT") != "NEEDS-ACTION" {
		t.Fatalf("params: %+v", att.Params)
	}
	if d, _ := ev.Get("DESCRIPTION"); !strings.Contains(d.Value, "octets and continues here") && !strings.Contains(d.Value, "octets and  continues here") {
		t.Fatalf("unfolding lost text: %q", d.Value)
	}
	if ev.Find("VALARM") == nil || cal.Find("VTIMEZONE") == nil {
		t.Fatal("alarms and time zones must survive")
	}
	again, err := Parse(string(cal.Serialize()))
	if err != nil {
		t.Fatalf("our own output must parse: %v", err)
	}
	if string(again.Serialize()) != string(cal.Serialize()) {
		t.Fatal("serialization is not stable")
	}
	if v, ok := again.Find("VEVENT").Get("X-VENDOR-THING"); !ok || v.Value != "keep me" || v.Param("X-P") != "a:b" {
		t.Fatalf("an unknown property with a quoted parameter was damaged: %+v", v)
	}
}

func TestTimesHonourTZIDAndKinds(t *testing.T) {
	cal, _ := Parse(sample)
	p, _ := cal.Find("VEVENT").Get("DTSTART")
	got, allDay, err := ParseTime(p)
	if err != nil || allDay || got.Hour() != 9 { // Paris is UTC+1 in November
		t.Fatalf("%v allDay=%v err=%v", got, allDay, err)
	}
	for text, want := range map[string]string{"20261103T030000Z": "03:00", "20261103": "00:00"} {
		v, ad, err := ParseTime(Prop{Name: "DTSTART", Value: text})
		if err != nil || v.Format("15:04") != want || ad != (len(text) == 8) {
			t.Errorf("%s -> %v %v %v", text, v, ad, err)
		}
	}
	if _, _, err := ParseTime(Prop{Value: "20261103T100000", Params: []Param{{Name: "TZID", Values: []string{"Mars/Olympus"}}}}); err == nil {
		t.Fatal("an unknown zone must be an error, not silently UTC")
	}
	if UTC(time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("ICT", 7*3600))) != "20261103T030000Z" {
		t.Fatal("UTC formatting")
	}
}

func TestSetReplacesAndSerializeNeverEmitsInjectedLines(t *testing.T) {
	cal, _ := Parse(sample)
	ev := cal.Find("VEVENT")
	ev.Set("SUMMARY", Escape("Kickoff; \"v2\"\nBEGIN:VEVENT"))
	ev.Props = append(ev.Props, Prop{Name: "X-BAD", Value: "x\r\nATTENDEE:mailto:evil@evil.test"})
	out := string(cal.Serialize())
	if strings.Contains(out, "evil@evil.test") {
		t.Fatalf("a raw line break in a value must never reach the wire:\n%s", out)
	}
	opens := 0
	for _, line := range strings.Split(out, "\r\n") {
		if line == "BEGIN:VEVENT" {
			opens++
		}
	}
	if opens != 1 {
		t.Fatalf("escaped newlines must not open a component:\n%s", out)
	}
	if !strings.Contains(out, `SUMMARY:Kickoff\; "v2"\nBEGIN:VEVENT`) {
		t.Fatalf("escaping: %s", out)
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Fatalf("unfolded line of %d octets", len(line))
		}
	}
}

func TestMalformedInputIsRejected(t *testing.T) {
	for name, in := range map[string]string{
		"empty":          "",
		"no end":         "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n",
		"wrong end":      "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nEND:VCALENDAR\r\nEND:VEVENT\r\n",
		"prop outside":   "SUMMARY:x\r\n",
		"no colon":       "BEGIN:VCALENDAR\r\nBROKEN\r\nEND:VCALENDAR\r\n",
		"two roots":      "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\nBEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
		"unterminated q": "BEGIN:VCALENDAR\r\nX;P=\"oops:v\r\nEND:VCALENDAR\r\n",
		"too deep":       strings.Repeat("BEGIN:A\r\n", 20),
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add(sample)
	f.Add("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	f.Fuzz(func(t *testing.T, s string) {
		if c, err := Parse(s); err == nil {
			_ = c.Serialize()
		}
	})
}
