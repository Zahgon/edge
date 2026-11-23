package jsstr

import "strings"

// QuoteJSON is the specification's QuoteJSONString, the text JSON.stringify writes
// for the string s: s in double quotes with '"', '\\' and the control characters
// escaped (the short escapes \b \f \n \r \t where they exist, \u00xx otherwise),
// and every unpaired surrogate written as a \uxxxx escape. Hex digits are lower
// case.
func QuoteJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	u := ToUTF16(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case c < 0x20:
				writeUnicodeEscape(&b, c)
			case c >= 0xD800 && c < 0xE000:
				cp, n := CodePointAt(u, i)
				if n == 2 {
					b.WriteRune(cp)
					i++
					continue
				}
				writeUnicodeEscape(&b, c)
			default:
				b.WriteRune(rune(c))
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// writeUnicodeEscape writes the code unit c as \u and four lower-case hex digits.
func writeUnicodeEscape(b *strings.Builder, c uint16) {
	const hex = "0123456789abcdef"
	b.WriteString(`\u`)
	b.WriteByte(hex[c>>12])
	b.WriteByte(hex[c>>8&15])
	b.WriteByte(hex[c>>4&15])
	b.WriteByte(hex[c&15])
}
