package memory

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// CMap represents a parsed PDF Character Map for mapping character codes to Unicode text.
type CMap struct {
	mappings    map[uint32]string
	charCodeLen int // 1, 2, or 4 bytes
}

func newCMap() *CMap {
	return &CMap{
		mappings:    make(map[uint32]string),
		charCodeLen: 2, // default for CID / Type0 fonts
	}
}

// decodeUTF16Hex decodes a hex string representing UTF-16BE code points into a UTF-8 string.
func decodeUTF16Hex(hexStr string) string {
	hexStr = strings.TrimSpace(strings.Trim(hexStr, "<>"))
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) == 0 {
		return ""
	}

	// If length is 1 byte, map ASCII directly
	if len(b) == 1 {
		return string(b)
	}

	// Decode 2-byte UTF-16BE units
	var u16 []uint16
	for i := 0; i+1 < len(b); i += 2 {
		u16 = append(u16, binary.BigEndian.Uint16(b[i:i+2]))
	}

	runes := utf16.Decode(u16)
	return string(runes)
}

// parseCMapContent parses a ToUnicode CMap stream.
func parseCMapContent(data []byte) (*CMap, error) {
	cm := newCMap()
	content := string(data)
	tokens := strings.Fields(content)

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		// Code space range: determines character code byte length
		if tok == "begincodespacerange" {
			i++
			for i < len(tokens) && tokens[i] != "endcodespacerange" {
				if strings.HasPrefix(tokens[i], "<") && strings.HasSuffix(tokens[i], ">") {
					hexCode := strings.Trim(tokens[i], "<>")
					if len(hexCode) <= 2 {
						cm.charCodeLen = 1
					} else if len(hexCode) <= 4 {
						cm.charCodeLen = 2
					} else {
						cm.charCodeLen = 4
					}
				}
				i++
			}
			continue
		}

		// Single character mappings: <srcHex> <dstHex>
		if tok == "beginbfchar" {
			i++
			for i < len(tokens) && tokens[i] != "endbfchar" {
				srcTok := tokens[i]
				if strings.HasPrefix(srcTok, "<") && strings.HasSuffix(srcTok, ">") && i+1 < len(tokens) {
					dstTok := tokens[i+1]
					if strings.HasPrefix(dstTok, "<") && strings.HasSuffix(dstTok, ">") {
						srcHex := strings.Trim(srcTok, "<>")
						srcCode, err := strconv.ParseUint(srcHex, 16, 32)
						if err == nil {
							decodedDst := decodeUTF16Hex(dstTok)
							if decodedDst != "" {
								cm.mappings[uint32(srcCode)] = decodedDst
							}
						}
						i++
					}
				}
				i++
			}
			continue
		}

		// Range mappings: <startHex> <endHex> <dstStartHex> OR <startHex> <endHex> [ <dst1> <dst2> ... ]
		if tok == "beginbfrange" {
			i++
			for i < len(tokens) && tokens[i] != "endbfrange" {
				if i+2 >= len(tokens) {
					break
				}
				startTok := tokens[i]
				endTok := tokens[i+1]

				if !strings.HasPrefix(startTok, "<") || !strings.HasPrefix(endTok, "<") {
					i++
					continue
				}

				startHex := strings.Trim(startTok, "<>")
				endHex := strings.Trim(endTok, "<>")
				startCode, err1 := strconv.ParseUint(startHex, 16, 32)
				endCode, err2 := strconv.ParseUint(endHex, 16, 32)
				if err1 != nil || err2 != nil || endCode < startCode {
					i += 2
					continue
				}

				dstTok := tokens[i+2]
				if dstTok == "[" {
					// Form B: array of destinations
					i += 3
					currCode := startCode
					for i < len(tokens) && tokens[i] != "]" && currCode <= endCode {
						item := tokens[i]
						if strings.HasPrefix(item, "<") && strings.HasSuffix(item, ">") {
							decodedDst := decodeUTF16Hex(item)
							if decodedDst != "" {
								cm.mappings[uint32(currCode)] = decodedDst
							}
						}
						currCode++
						i++
					}
				} else if strings.HasPrefix(dstTok, "<") && strings.HasSuffix(dstTok, ">") {
					// Form A: contiguous destination range
					dstHex := strings.Trim(dstTok, "<>")
					dstStartCode, err3 := strconv.ParseUint(dstHex, 16, 32)
					if err3 == nil {
						count := endCode - startCode
						for offset := uint64(0); offset <= count; offset++ {
							srcCode := startCode + offset
							targetCode := dstStartCode + offset
							targetHex := fmt.Sprintf("%04X", targetCode)
							decodedDst := decodeUTF16Hex(targetHex)
							if decodedDst != "" {
								cm.mappings[uint32(srcCode)] = decodedDst
							}
						}
					}
					i += 3
				} else {
					i += 2
				}
			}
			continue
		}
	}

	return cm, nil
}

// AdobeGlyphList maps standard Adobe Glyph names (including Vietnamese and Latin accents) to runes.
var adobeGlyphList = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#', "dollar": '$',
	"percent": '%', "ampersand": '&', "quotesingle": '\'', "parenleft": '(', "parenright": ')',
	"asterisk": '*', "plus": '+', "comma": ',', "hyphen": '-', "period": '.', "slash": '/',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4', "five": '5',
	"six": '6', "seven": '7', "eight": '8', "nine": '9', "colon": ':', "semicolon": ';',
	"less": '<', "equal": '=', "greater": '>', "question": '?', "at": '@',
	"A": 'A', "B": 'B', "C": 'C', "D": 'D', "E": 'E', "F": 'F', "G": 'G', "H": 'H',
	"I": 'I', "J": 'J', "K": 'K', "L": 'L', "M": 'M', "N": 'N', "O": 'O', "P": 'P',
	"Q": 'Q', "R": 'R', "S": 'S', "T": 'T', "U": 'U', "V": 'V', "W": 'W', "X": 'X',
	"Y": 'Y', "Z": 'Z', "bracketleft": '[', "backslash": '\\', "bracketright": ']',
	"asciicircum": '^', "underscore": '_', "grave": '`',
	"a": 'a', "b": 'b', "c": 'c', "d": 'd', "e": 'e', "f": 'f', "g": 'g', "h": 'h',
	"i": 'i', "j": 'j', "k": 'k', "l": 'l', "m": 'm', "n": 'n', "o": 'o', "p": 'p',
	"q": 'q', "r": 'r', "s": 's', "t": 't', "u": 'u', "v": 'v', "w": 'w', "x": 'x',
	"y": 'y', "z": 'z', "braceleft": '{', "bar": '|', "braceright": '}', "asciitilde": '~',
	// Latin-1 & Accents
	"aacute": 'á', "agrave": 'à', "atilde": 'ã', "acircumflex": 'â', "adieresis": 'ä', "aring": 'å', "ae": 'æ',
	"eacute": 'é', "egrave": 'è', "ecircumflex": 'ê', "edieresis": 'ë',
	"iacute": 'í', "igrave": 'ì', "itilde": 'ĩ', "idieresis": 'ï',
	"oacute": 'ó', "ograve": 'ò', "otilde": 'õ', "ocircumflex": 'ô', "odieresis": 'ö', "oslash": 'ø',
	"uacute": 'ú', "ugrave": 'ù', "utilde": 'ũ', "udieresis": 'ü',
	"yacute": 'ý', "ydieresis": 'ÿ', "ccedilla": 'ç', "ntilde": 'ñ', "germandbls": 'ß',
	"eth": 'ð', "thorn": 'þ', "questiondown": '¿', "exclamdown": '¡',
	"Aacute": 'Á', "Agrave": 'À', "Atilde": 'Ã', "Acircumflex": 'Â', "Adieresis": 'Ä', "Aring": 'Å', "AE": 'Æ',
	"Eacute": 'É', "Egrave": 'È', "Ecircumflex": 'Ê', "Edieresis": 'Ë',
	"Iacute": 'Í', "Igrave": 'Ì', "Itilde": 'Ĩ', "Idieresis": 'Ï',
	"Oacute": 'Ó', "Ograve": 'Ò', "Otilde": 'Õ', "Ocircumflex": 'Ô', "Odieresis": 'Ö', "Oslash": 'Ø',
	"Uacute": 'Ú', "Ugrave": 'Ù', "Utilde": 'Ũ', "Udieresis": 'Ü',
	"Yacute": 'Ý', "Ydieresis": 'Ÿ', "Ccedilla": 'Ç', "Ntilde": 'Ñ',
	"Eth": 'Ð', "Thorn": 'Þ',
	// Central / Eastern European & Baltic
	"cacute": 'ć', "Cacute": 'Ć', "ccaron": 'č', "Ccaron": 'Č', "czaron": 'č',
	"dcaron": 'ď', "Dcaron": 'Ď', "dcroat": 'đ', "dstroke": 'đ', "Dcroat": 'Đ', "Dstroke": 'Đ',
	"eogonek": 'ę', "Eogonek": 'Ę', "ecaron": 'ě', "Ecaron": 'Ě',
	"lacute": 'ĺ', "Lacute": 'Ĺ', "lcaron": 'ľ', "Lcaron": 'Ľ', "lslash": 'ł', "Lslash": 'Ł',
	"nacute": 'ń', "Nacute": 'Ń', "ncaron": 'ň', "Ncaron": 'Ň',
	"racute": 'ŕ', "Racute": 'Ŕ', "rcaron": 'ř', "Rcaron": 'Ř',
	"sacute": 'ś', "Sacute": 'Ś', "scaron": 'š', "Scaron": 'Š', "scedilla": 'ş', "Scedilla": 'Ş',
	"scommaaccent": 'ș', "Scommaaccent": 'Ș', "tcommaaccent": 'ț', "Tcommaaccent": 'Ț', "tcaron": 'ť', "Tcaron": 'Ť',
	"zacute": 'ź', "Zacute": 'Ź', "zdotaccent": 'ż', "Zdotaccent": 'Ż', "zcaron": 'ž', "Zcaron": 'Ž',
	"aogonek": 'ą', "Aogonek": 'Ą', "iogonek": 'į', "Iogonek": 'Į', "uogonek": 'ų', "Uogonek": 'Ų',
	"gbreve": 'ğ', "Gbreve": 'Ğ', "dotlessi": 'ı', "Idotaccent": 'İ',
	"hungarumlaut": '˝', "odblacute": 'ő', "Odblacute": 'Ő', "udblacute": 'ű', "Udblacute": 'Ű',
	// Typographic & Math symbols
	"endash": '–', "emdash": '—', "quoteleft": '‘', "quoteright": '’', "quotesinglbase": '‚',
	"quotedblleft": '“', "quotedblright": '”', "quotedblbase": '„', "bullet": '•', "ellipsis": '…',
	"perthousand": '‰', "guilsinglleft": '‹', "guilsinglright": '›', "guillemotleft": '«', "guillemotright": '»',
	"euro": '€', "sterling": '£', "yen": '¥', "cent": '¢', "currency": '¤',
	"section": '§', "paragraph": '¶', "dagger": '†', "daggerdbl": '‡',
	"degree": '°', "plusminus": '±', "twosuperior": '²', "threesuperior": '³', "onesuperior": '¹',
	"micro": 'µ', "multiply": '×', "divide": '÷', "onequarter": '¼', "onehalf": '½', "threequarters": '¾',
	"copyright": '©', "registered": '®', "trademark": '™',
	// Vietnamese Specific Diacritics
	"abreve": 'ă', "Abreve": 'Ă',
	"abreveacute": 'ắ', "Abreveacute": 'Ắ',
	"abrevegrave": 'ằ', "Abrevegrave": 'Ằ',
	"abrevetilde": 'ẵ', "Abrevetilde": 'Ẵ',
	"abrevehookabove": 'ẳ', "Abrevehookabove": 'Ẳ',
	"abrevedotbelow": 'ặ', "Abrevedotbelow": 'Ặ',
	"adotbelow": 'ạ', "Adotbelow": 'Ạ',
	"ahookabove": 'ả', "Ahookabove": 'Ả',
	"acircumflexacute": 'ấ', "Acircumflexacute": 'Ấ',
	"acircumflexgrave": 'ầ', "Acircumflexgrave": 'Ầ',
	"acircumflextilde": 'ẫ', "Acircumflextilde": 'Ẫ',
	"acircumflexhookabove": 'ẩ', "Acircumflexhookabove": 'Ẩ',
	"acircumflexdotbelow": 'ậ', "Acircumflexdotbelow": 'Ậ',
	"edotbelow": 'ẹ', "Edotbelow": 'Ẹ',
	"ehookabove": 'ẻ', "Ehookabove": 'Ẻ',
	"etilde": 'ẽ', "Etilde": 'Ẽ',
	"ecircumflexacute": 'ế', "Ecircumflexacute": 'Ế',
	"ecircumflexgrave": 'ề', "Ecircumflexgrave": 'Ề',
	"ecircumflextilde": 'ễ', "Ecircumflextilde": 'Ễ',
	"ecircumflexhookabove": 'ể', "Ecircumflexhookabove": 'Ể',
	"ecircumflexdotbelow": 'ệ', "Ecircumflexdotbelow": 'Ệ',
	"idotbelow": 'ị', "Idotbelow": 'Ị',
	"ihookabove": 'ỉ', "Ihookabove": 'Ỉ',
	"odotbelow": 'ọ', "Odotbelow": 'Ọ',
	"ohookabove": 'ỏ', "Ohookabove": 'Ỏ',
	"ocircumflexacute": 'ố', "Ocircumflexacute": 'Ố',
	"ocircumflexgrave": 'ồ', "Ocircumflexgrave": 'Ồ',
	"ocircumflextilde": 'ỗ', "Ocircumflextilde": 'Ỗ',
	"ocircumflexhookabove": 'ổ', "Ocircumflexhookabove": 'Ổ',
	"ocircumflexdotbelow": 'ộ', "Ocircumflexdotbelow": 'Ộ',
	"ohorn": 'ơ', "Ohorn": 'Ơ',
	"ohornacute": 'ớ', "Ohornacute": 'Ớ',
	"ohorngrave": 'ờ', "Ohorngrave": 'Ờ',
	"ohorntilde": 'ỡ', "Ohorntilde": 'Ỡ',
	"ohornhookabove": 'ở', "Ohornhookabove": 'Ở',
	"ohorndotbelow": 'ợ', "Ohorndotbelow": 'Ợ',
	"udotbelow": 'ụ', "Udotbelow": 'Ụ',
	"uhookabove": 'ủ', "Uhookabove": 'Ủ',
	"uhorn": 'ư', "Uhorn": 'Ư',
	"uhornacute": 'ứ', "Uhornacute": 'Ứ',
	"uhorngrave": 'ừ', "Uhorngrave": 'Ừ',
	"uhorntilde": 'ữ', "Uhorntilde": 'Ữ',
	"uhornhookabove": 'ử', "Uhornhookabove": 'Ử',
	"uhorndotbelow": 'ự', "Uhorndotbelow": 'Ự',
	"ydotbelow": 'ỵ', "Ydotbelow": 'Ỵ',
	"yhookabove": 'ỷ', "Yhookabove": 'Ỷ',
	"ytilde": 'ỹ', "Ytilde": 'Ỹ',
	"ygrave": 'ỳ', "Ygrave": 'Ỳ',
}

// resolveGlyphName resolves a glyph name (e.g., "aacute", "uni1EBF", "u1EBF") to a rune.
func resolveGlyphName(name string) rune {
	if r, ok := adobeGlyphList[name]; ok {
		return r
	}
	// Check for uniXXXX or uXXXX patterns
	if strings.HasPrefix(name, "uni") && len(name) == 7 {
		if val, err := strconv.ParseUint(name[3:], 16, 32); err == nil {
			return rune(val)
		}
	}
	if strings.HasPrefix(name, "u") && len(name) >= 5 && len(name) <= 7 {
		if val, err := strconv.ParseUint(name[1:], 16, 32); err == nil {
			return rune(val)
		}
	}
	return 0
}

// PDFFontContext holds font decoding information for a PDF font.
type PDFFontContext struct {
	Name        string
	Subtype     string
	CMap        *CMap
	EncodingMap map[byte]rune
	IsComposite bool
}

// extractFontsFromPage collects all Font objects and their CMap/Encoding resources from a Page.
func extractFontsFromPage(p pdf.Page) map[string]*PDFFontContext {
	fontMap := make(map[string]*PDFFontContext)
	res := p.V.Key("Resources")
	if res.Kind() != pdf.Dict {
		return fontMap
	}

	fontsDict := res.Key("Font")
	if fontsDict.Kind() != pdf.Dict {
		return fontMap
	}

	for _, fontKey := range fontsDict.Keys() {
		fontVal := fontsDict.Key(fontKey)
		if fontVal.Kind() != pdf.Dict {
			continue
		}

		ctx := &PDFFontContext{
			Name:        fontKey,
			Subtype:     fontVal.Key("Subtype").Name(),
			EncodingMap: make(map[byte]rune),
		}
		if ctx.Subtype == "Type0" {
			ctx.IsComposite = true
		}

		// 1. Try parsing ToUnicode CMap stream from Font or DescendantFonts
		toUnicodeVal := fontVal.Key("ToUnicode")
		if toUnicodeVal.Kind() == pdf.Stream {
			rc := toUnicodeVal.Reader()
			if rc != nil {
				data, err := io.ReadAll(rc)
				_ = rc.Close()
				if err == nil && len(data) > 0 {
					cm, err := parseCMapContent(data)
					if err == nil && len(cm.mappings) > 0 {
						ctx.CMap = cm
					}
				}
			}
		}

		// If ToUnicode was not on Type0 directly, check descendant CIDFonts
		if ctx.CMap == nil && ctx.IsComposite {
			descFonts := fontVal.Key("DescendantFonts")
			if descFonts.Kind() == pdf.Array && descFonts.Len() > 0 {
				descFont := descFonts.Index(0)
				descToUnicode := descFont.Key("ToUnicode")
				if descToUnicode.Kind() == pdf.Stream {
					rc := descToUnicode.Reader()
					if rc != nil {
						data, err := io.ReadAll(rc)
						_ = rc.Close()
						if err == nil && len(data) > 0 {
							cm, err := parseCMapContent(data)
							if err == nil && len(cm.mappings) > 0 {
								ctx.CMap = cm
							}
						}
					}
				}
			}
		}

		// 2. Parse /Encoding & /Differences for 8-bit single-byte fonts
		encVal := fontVal.Key("Encoding")
		if encVal.Kind() == pdf.Dict {
			diffs := encVal.Key("Differences")
			if diffs.Kind() == pdf.Array {
				var curCode byte = 0
				for idx := 0; idx < diffs.Len(); idx++ {
					item := diffs.Index(idx)
					if item.Kind() == pdf.Integer {
						curCode = byte(item.Int64())
					} else if item.Kind() == pdf.Name {
						glyphName := item.Name()
						r := resolveGlyphName(glyphName)
						if r != 0 {
							ctx.EncodingMap[curCode] = r
						}
						curCode++
					}
				}
			}
		}

		fontMap[fontKey] = ctx
		fontMap["/"+fontKey] = ctx
	}

	return fontMap
}

// decodeStringWithFont decodes raw string/hex bytes using the active font's CMap or Encoding.
func decodeStringWithFont(raw []byte, font *PDFFontContext) string {
	if len(raw) == 0 {
		return ""
	}

	// 1. If font has a valid CMap mapping
	if font != nil && font.CMap != nil && len(font.CMap.mappings) > 0 {
		var sb strings.Builder
		codeLen := font.CMap.charCodeLen
		if font.IsComposite && codeLen < 2 {
			codeLen = 2
		}

		if codeLen == 2 {
			for i := 0; i < len(raw); {
				if i+1 < len(raw) {
					code := uint32(raw[i])<<8 | uint32(raw[i+1])
					if mapped, ok := font.CMap.mappings[code]; ok {
						sb.WriteString(mapped)
						i += 2
						continue
					}
				}
				// Fallback single byte in CMap
				code := uint32(raw[i])
				if mapped, ok := font.CMap.mappings[code]; ok {
					sb.WriteString(mapped)
				} else if raw[i] >= 32 && raw[i] < 127 {
					sb.WriteByte(raw[i])
				}
				i++
			}
			return sb.String()
		} else if codeLen == 1 {
			for _, b := range raw {
				code := uint32(b)
				if mapped, ok := font.CMap.mappings[code]; ok {
					sb.WriteString(mapped)
				} else if b >= 32 && b < 127 {
					sb.WriteByte(b)
				}
			}
			return sb.String()
		}
	}

	// 2. If font has custom Differences / EncodingMap
	if font != nil && len(font.EncodingMap) > 0 {
		var sb strings.Builder
		for _, b := range raw {
			if r, ok := font.EncodingMap[b]; ok {
				sb.WriteRune(r)
			} else if b >= 32 && b < 127 {
				sb.WriteByte(b)
			}
		}
		return sb.String()
	}

	// 3. UTF-16BE BOM detection (\xFE\xFF)
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		var u16 []uint16
		for i := 2; i+1 < len(raw); i += 2 {
			u16 = append(u16, binary.BigEndian.Uint16(raw[i:i+2]))
		}
		return string(utf16.Decode(u16))
	}

	// 4. Valid UTF-8 direct pass-through
	if utf8.Valid(raw) {
		return string(raw)
	}

	// 5. Fallback Latin-1 / ASCII
	var sb strings.Builder
	for _, b := range raw {
		sb.WriteRune(rune(b))
	}
	return sb.String()
}

// extractPageContentText interprets the PDF content stream operators (BT, ET, Tf, Tj, TJ, TD, T*, etc.)
func extractPageContentText(p pdf.Page, fonts map[string]*PDFFontContext) (string, error) {
	contentsVal := p.V.Key("Contents")
	if contentsVal.Kind() == pdf.Null {
		return "", errors.New("empty contents")
	}

	var streamData bytes.Buffer
	if contentsVal.Kind() == pdf.Stream {
		rc := contentsVal.Reader()
		if rc == nil {
			return "", errors.New("cannot open contents stream")
		}
		_, err := io.Copy(&streamData, rc)
		_ = rc.Close()
		if err != nil {
			return "", err
		}
	} else if contentsVal.Kind() == pdf.Array {
		for i := 0; i < contentsVal.Len(); i++ {
			st := contentsVal.Index(i)
			if st.Kind() == pdf.Stream {
				rc := st.Reader()
				if rc != nil {
					_, _ = io.Copy(&streamData, rc)
					_ = rc.Close()
					streamData.WriteByte('\n')
				}
			}
		}
	}

	rawContent := streamData.Bytes()
	if len(rawContent) == 0 {
		return "", errors.New("no stream data")
	}

	var out strings.Builder
	var activeFont *PDFFontContext
	var tjAccumulator strings.Builder
	var lastStringDecoded string
	var args []float64
	var lastX, lastY float64
	var hasLastPos bool

	inTextObject := false
	inString := false
	inHex := false
	escapeNext := false
	parenDepth := 0
	var stringBuf bytes.Buffer

	i := 0
	for i < len(rawContent) {
		b := rawContent[i]

		// 1. Inside PDF string literal (...)
		if inString {
			if escapeNext {
				switch b {
				case 'n':
					stringBuf.WriteByte('\n')
				case 'r':
					stringBuf.WriteByte('\r')
				case 't':
					stringBuf.WriteByte('\t')
				case 'b':
					stringBuf.WriteByte('\b')
				case 'f':
					stringBuf.WriteByte('\f')
				case '(', ')', '\\':
					stringBuf.WriteByte(b)
				default:
					if b >= '0' && b <= '7' {
						octal := string(b)
						if i+1 < len(rawContent) && rawContent[i+1] >= '0' && rawContent[i+1] <= '7' {
							octal += string(rawContent[i+1])
							i++
							if i+1 < len(rawContent) && rawContent[i+1] >= '0' && rawContent[i+1] <= '7' {
								octal += string(rawContent[i+1])
								i++
							}
						}
						val, _ := strconv.ParseUint(octal, 8, 8)
						stringBuf.WriteByte(byte(val))
					} else {
						stringBuf.WriteByte(b)
					}
				}
				escapeNext = false
			} else if b == '\\' {
				escapeNext = true
			} else if b == '(' {
				parenDepth++
				stringBuf.WriteByte(b)
			} else if b == ')' {
				if parenDepth > 0 {
					parenDepth--
					stringBuf.WriteByte(b)
				} else {
					inString = false
					decoded := decodeStringWithFont(stringBuf.Bytes(), activeFont)
					lastStringDecoded = decoded
					tjAccumulator.WriteString(decoded)
					stringBuf.Reset()
				}
			} else {
				stringBuf.WriteByte(b)
			}
			i++
			continue
		}

		// 2. Inside hex string <...>
		if inHex {
			if b == '>' {
				inHex = false
				hexStr := stringBuf.String()
				if len(hexStr)%2 != 0 {
					hexStr += "0"
				}
				if bBytes, err := hex.DecodeString(hexStr); err == nil {
					decoded := decodeStringWithFont(bBytes, activeFont)
					lastStringDecoded = decoded
					tjAccumulator.WriteString(decoded)
				}
				stringBuf.Reset()
			} else if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') {
				stringBuf.WriteByte(b)
			}
			i++
			continue
		}

		// 3. String literal start
		if b == '(' {
			inString = true
			parenDepth = 0
			escapeNext = false
			stringBuf.Reset()
			i++
			continue
		}

		// 4. Hex string start (excluding << dict start)
		if b == '<' {
			if i+1 < len(rawContent) && rawContent[i+1] == '<' {
				// Dictionary start <<
				i += 2
				continue
			}
			inHex = true
			stringBuf.Reset()
			i++
			continue
		}

		// 5. Dictionary end >>
		if b == '>' && i+1 < len(rawContent) && rawContent[i+1] == '>' {
			i += 2
			continue
		}

		// 6. Array delimiters [ and ]
		if b == '[' {
			tjAccumulator.Reset()
			i++
			continue
		}
		if b == ']' {
			i++
			continue
		}

		// 7. Whitespace
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			i++
			continue
		}

		// 8. Word token (names, numbers, operators)
		start := i
		for i < len(rawContent) && rawContent[i] != ' ' && rawContent[i] != '\t' &&
			rawContent[i] != '\r' && rawContent[i] != '\n' && rawContent[i] != '(' &&
			rawContent[i] != ')' && rawContent[i] != '<' && rawContent[i] != '>' &&
			rawContent[i] != '[' && rawContent[i] != ']' {
			i++
		}
		if start == i {
			i++
			continue
		}
		op := string(rawContent[start:i])

		// Numeric argument for graphics/text matrix operators
		if num, err := strconv.ParseFloat(op, 64); err == nil {
			args = append(args, num)
			continue
		}

		switch op {
		case "BT":
			inTextObject = true
			tjAccumulator.Reset()
			lastStringDecoded = ""
			args = args[:0]
		case "ET":
			inTextObject = false
			args = args[:0]
		case "Tm":
			// a b c d e f Tm
			if len(args) >= 6 {
				curX := args[4]
				curY := args[5]
				if hasLastPos {
					if math.Abs(curY-lastY) > 2.0 {
						if !strings.HasSuffix(out.String(), "\n") && out.Len() > 0 {
							out.WriteString("\n")
						}
					} else if curX > lastX+5.0 {
						if !strings.HasSuffix(out.String(), " ") && !strings.HasSuffix(out.String(), "\n") && out.Len() > 0 {
							out.WriteString(" ")
						}
					}
				}
				lastX = curX
				lastY = curY
				hasLastPos = true
			}
			args = args[:0]
		case "Td", "TD":
			// tx ty Td
			if len(args) >= 2 {
				ty := args[len(args)-1]
				if math.Abs(ty) > 2.0 {
					if !strings.HasSuffix(out.String(), "\n") && out.Len() > 0 {
						out.WriteString("\n")
					}
				}
			}
			args = args[:0]
		case "T*":
			if !strings.HasSuffix(out.String(), "\n") && out.Len() > 0 {
				out.WriteString("\n")
			}
			args = args[:0]
		case "Tf":
			// Font selection: look backwards for /FontName
			prefix := strings.TrimSpace(string(rawContent[max(0, start-32) : start]))
			parts := strings.Fields(prefix)
			for pIdx := len(parts) - 1; pIdx >= 0; pIdx-- {
				if strings.HasPrefix(parts[pIdx], "/") {
					fontKey := strings.TrimPrefix(parts[pIdx], "/")
					if ctx, ok := fonts[fontKey]; ok {
						activeFont = ctx
					} else if ctx, ok := fonts["/"+fontKey]; ok {
						activeFont = ctx
					}
					break
				}
			}
			args = args[:0]
		case "Tj":
			if inTextObject {
				out.WriteString(lastStringDecoded)
				lastStringDecoded = ""
			}
			args = args[:0]
		case "'", "\"":
			if inTextObject {
				out.WriteString(lastStringDecoded)
				out.WriteString("\n")
				lastStringDecoded = ""
			}
			args = args[:0]
		case "TJ":
			if inTextObject {
				out.WriteString(tjAccumulator.String())
				tjAccumulator.Reset()
				lastStringDecoded = ""
			}
			args = args[:0]
		default:
			args = args[:0]
		}
	}

	return out.String(), nil
}

func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// cleanExtractedText removes null bytes, unprintable artifacts, and normalizes blank lines.
func cleanExtractedText(text string) string {
	var sb strings.Builder
	lines := strings.Split(text, "\n")
	consecutiveBlank := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			consecutiveBlank++
			if consecutiveBlank <= 1 && sb.Len() > 0 {
				sb.WriteString("\n")
			}
			continue
		}
		consecutiveBlank = 0

		// Clean non-printable characters
		var cleanLine strings.Builder
		for _, r := range trimmed {
			if r == '\t' || (r >= 32 && r != 0xFFFD) {
				cleanLine.WriteRune(r)
			}
		}
		res := strings.TrimSpace(cleanLine.String())
		if len(res) > 0 {
			sb.WriteString(res)
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// extractPDFText extracts clean, accurately decoded Unicode & Vietnamese text from a PDF file.
func extractPDFText(filePath string) (string, error) {
	file, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: opening pdf: %v", errUnsupportedEmbeddingSource, err)
	}
	defer file.Close()

	numPages := r.NumPage()
	if numPages <= 0 {
		return "", fmt.Errorf("%w: pdf has 0 pages", errUnsupportedEmbeddingSource)
	}

	var fullText strings.Builder

	for pageIdx := 1; pageIdx <= numPages; pageIdx++ {
		p := r.Page(pageIdx)
		fonts := extractFontsFromPage(p)

		// Try our advanced CMap & Encoding stream interpreter
		pageText, err := extractPageContentText(p, fonts)
		cleaned := cleanExtractedText(pageText)

		// If our advanced interpreter successfully extracted meaningful text, use it
		if err == nil && len(cleaned) > 0 {
			fullText.WriteString(cleaned)
			fullText.WriteString("\n\n")
			continue
		}

		// Fallback: use built-in GetTextByRow or Content()
		var fallbackText strings.Builder
		rows, errRow := p.GetTextByRow()
		if errRow == nil && len(rows) > 0 {
			for _, row := range rows {
				for _, word := range row.Content {
					fallbackText.WriteString(word.S)
					fallbackText.WriteString(" ")
				}
				fallbackText.WriteString("\n")
			}
		} else {
			for _, item := range p.Content().Text {
				fallbackText.WriteString(item.S)
			}
		}

		cleanFallback := cleanExtractedText(fallbackText.String())
		if len(cleanFallback) > 0 {
			fullText.WriteString(cleanFallback)
			fullText.WriteString("\n\n")
		}
	}

	result := strings.TrimSpace(fullText.String())
	if len(result) == 0 {
		return "", fmt.Errorf("%w: no readable text found in pdf", errUnsupportedEmbeddingSource)
	}

	return result, nil
}
