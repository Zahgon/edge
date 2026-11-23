// Go literal helpers for the recorders: JavaScript strings become Go strings in WTF-8.
export function goStr(s) {
  let out = '"'
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i)
    if (c === 0x22) out += '\\"'
    else if (c === 0x5c) out += '\\\\'
    else if (c === 0x0a) out += '\\n'
    else if (c === 0x0d) out += '\\r'
    else if (c === 0x09) out += '\\t'
    else if (c < 0x20 || c === 0x7f) out += '\\x' + c.toString(16).padStart(2, '0')
    else if (c >= 0xd800 && c < 0xdc00 && i + 1 < s.length && s.charCodeAt(i + 1) >= 0xdc00 && s.charCodeAt(i + 1) < 0xe000) { out += s[i] + s[i + 1]; i++ }
    else if (c >= 0xd800 && c < 0xe000) {
      out += '\\x' + (0xe0 | (c >> 12)).toString(16) + '\\x' + (0x80 | ((c >> 6) & 0x3f)).toString(16) + '\\x' + (0x80 | (c & 0x3f)).toString(16)
    } else if (c === 0x2028 || c === 0x2029 || c === 0xfeff || (c >= 0x80 && c < 0xa0)) out += '\\u' + c.toString(16).padStart(4, '0')
    else out += s[i]
  }
  return out + '"'
}
// A raw string literal when the text allows it (no backtick, no carriage return, valid UTF-16).
