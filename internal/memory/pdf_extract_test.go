package memory

import (
	"testing"
)

func TestParseCMapContent_Bfchar(t *testing.T) {
	cmapData := `/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Custom-ToUnicode def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
6 beginbfchar
<0001> <004B>
<0002> <0049>
<0003> <0045>
<0004> <0055>
<0020> <0020>
<0024> <1EBF>
endbfchar
endcmap`

	cm, err := parseCMapContent([]byte(cmapData))
	if err != nil {
		t.Fatalf("parseCMapContent failed: %v", err)
	}

	if cm.charCodeLen != 2 {
		t.Fatalf("expected charCodeLen=2, got %d", cm.charCodeLen)
	}

	if cm.mappings[0x0001] != "K" {
		t.Fatalf("expected 'K', got %q", cm.mappings[0x0001])
	}
	if cm.mappings[0x0002] != "I" {
		t.Fatalf("expected 'I', got %q", cm.mappings[0x0002])
	}
	if cm.mappings[0x0003] != "E" {
		t.Fatalf("expected 'E', got %q", cm.mappings[0x0003])
	}
	if cm.mappings[0x0004] != "U" {
		t.Fatalf("expected 'U', got %q", cm.mappings[0x0004])
	}
	// Vietnamese character U+1EBF = 'ế'
	if cm.mappings[0x0024] != "ế" {
		t.Fatalf("expected 'ế', got %q", cm.mappings[0x0024])
	}
}

func TestParseCMapContent_Bfrange(t *testing.T) {
	cmapData := `begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfrange
<0041> <0043> <0041>
endbfrange
1 beginbfrange
<0050> <0052> [ <1EA1> <0111> <1ED9> ]
endbfrange
endcmap`

	cm, err := parseCMapContent([]byte(cmapData))
	if err != nil {
		t.Fatalf("parseCMapContent bfrange failed: %v", err)
	}

	// Range Form A: <0041> <0043> <0041> -> 'A', 'B', 'C'
	if cm.mappings[0x0041] != "A" || cm.mappings[0x0042] != "B" || cm.mappings[0x0043] != "C" {
		t.Fatalf("unexpected Form A mapping: %v", cm.mappings)
	}

	// Range Form B: <0050> -> U+1EA1 ('ạ'), <0051> -> U+0111 ('đ'), <0052> -> U+1ED9 ('ộ')
	if cm.mappings[0x0050] != "ạ" {
		t.Fatalf("expected 'ạ', got %q", cm.mappings[0x0050])
	}
	if cm.mappings[0x0051] != "đ" {
		t.Fatalf("expected 'đ', got %q", cm.mappings[0x0051])
	}
	if cm.mappings[0x0052] != "ộ" {
		t.Fatalf("expected 'ộ', got %q", cm.mappings[0x0052])
	}
}

func TestDecodeStringWithFont_CMapAndDifferences(t *testing.T) {
	// 1. CMap 2-byte Vietnamese string
	cm := newCMap()
	cm.charCodeLen = 2
	cm.mappings[0x0001] = "K"
	cm.mappings[0x0002] = "i"
	cm.mappings[0x0003] = "ề"
	cm.mappings[0x0004] = "u"

	fontCtx := &PDFFontContext{
		Name:        "F1",
		Subtype:     "Type0",
		CMap:        cm,
		IsComposite: true,
	}

	rawBytes := []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04}
	decoded := decodeStringWithFont(rawBytes, fontCtx)
	if decoded != "Kiều" {
		t.Fatalf("expected 'Kiều', got %q", decoded)
	}

	// 2. Differences Encoding Map for 8-bit font
	diffFont := &PDFFontContext{
		Name:    "F2",
		Subtype: "TrueType",
		EncodingMap: map[byte]rune{
			0x80: 'đ',
			0x81: 'ồ',
			0x82: 'n',
			0x83: 'g',
		},
	}
	decodedDiff := decodeStringWithFont([]byte{0x80, 0x81, 0x82, 0x83}, diffFont)
	if decodedDiff != "đồng" {
		t.Fatalf("expected 'đồng', got %q", decodedDiff)
	}
}

func TestResolveGlyphName(t *testing.T) {
	tests := []struct {
		name     string
		expected rune
	}{
		{"dcroat", 'đ'},
		{"Dcroat", 'Đ'},
		{"aacute", 'á'},
		{"acircumflexacute", 'ấ'},
		{"ecircumflexacute", 'ế'},
		{"ohorndotbelow", 'ợ'},
		{"uhornhookabove", 'ử'},
		{"uni1EBF", 'ế'},
		{"uni0111", 'đ'},
	}

	for _, tt := range tests {
		r := resolveGlyphName(tt.name)
		if r != tt.expected {
			t.Errorf("resolveGlyphName(%q) = %q (%U), want %q (%U)", tt.name, r, r, tt.expected, tt.expected)
		}
	}
}

func TestDecodeStringWithFont_MultiLanguage(t *testing.T) {
	// 1. Japanese (Hiragana & Kanji) + Chinese (Hanzi) via CMap
	cjkCMap := newCMap()
	cjkCMap.charCodeLen = 2
	cjkCMap.mappings[0x0001] = "東" // U+6771 (Tokyo East)
	cjkCMap.mappings[0x0002] = "京" // U+4EAC (Tokyo Capital)
	cjkCMap.mappings[0x0003] = "へ" // U+3078 (Hiragana He)
	cjkCMap.mappings[0x0004] = "よ" // U+3088 (Hiragana Yo)
	cjkCMap.mappings[0x0005] = "う" // U+3046 (Hiragana U)
	cjkCMap.mappings[0x0006] = "こ" // U+3053 (Hiragana Ko)
	cjkCMap.mappings[0x0007] = "そ" // U+305D (Hiragana So)

	cjkFont := &PDFFontContext{
		Name:        "F_CJK",
		Subtype:     "Type0",
		CMap:        cjkCMap,
		IsComposite: true,
	}

	rawCJK := []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00, 0x05, 0x00, 0x06, 0x00, 0x07}
	decodedCJK := decodeStringWithFont(rawCJK, cjkFont)
	if decodedCJK != "東京へようこそ" {
		t.Fatalf("expected '東京へようこそ', got %q", decodedCJK)
	}

	// 2. German, French, Spanish, Polish via Glyph resolution
	europeanTests := []struct {
		name     string
		expected rune
	}{
		{"adieresis", 'ä'},
		{"odieresis", 'ö'},
		{"udieresis", 'ü'},
		{"germandbls", 'ß'},
		{"eacute", 'é'},
		{"egrave", 'è'},
		{"ccedilla", 'ç'},
		{"ntilde", 'ñ'},
		{"lslash", 'ł'},
		{"zdotaccent", 'ż'},
		{"scommaaccent", 'ș'},
		{"gbreve", 'ğ'},
	}

	for _, tt := range europeanTests {
		r := resolveGlyphName(tt.name)
		if r != tt.expected {
			t.Errorf("resolveGlyphName(%q) = %q, want %q", tt.name, r, tt.expected)
		}
	}
}

func TestCleanExtractedText(t *testing.T) {
	raw := "  KIEU TRONG THIEN \x00\x01 \n\n\n FULL STACK ENGINEER \n\n"
	cleaned := cleanExtractedText(raw)
	expected := "KIEU TRONG THIEN\n\nFULL STACK ENGINEER"
	if cleaned != expected {
		t.Fatalf("got %q, want %q", cleaned, expected)
	}
}
