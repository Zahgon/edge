package acorn

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// acorn validates the pattern and flags of every regular expression literal
// against the grammar of the ECMAScript version it parses (Annex B included),
// with V8's error messages. This file is that validator for ECMAScript 3 to 2020;
// the v flag, the d flag, modifiers and duplicate group names in alternatives
// come later.

// unicodeProperties are the names a \p{...} escape accepts.
type unicodeProperties struct {
	binary    wordSet
	nonBinary map[string]wordSet
}

// unicodeData holds the property names by ecmaVersion, as acorn lists them from
// the tables of the ECMAScript specifications.
var unicodeData = map[int]*unicodeProperties{}

func init() {
	binary := map[int]string{9: ecma9BinaryProperties}
	binary[10] = binary[9] + " Extended_Pictographic"
	binary[11] = binary[10]
	script := map[int]string{9: ecma9ScriptValues}
	script[10] = script[9] + " Dogra Dogr Gunjala_Gondi Gong Hanifi_Rohingya Rohg Makasar Maka Medefaidrin Medf Old_Sogdian Sogo Sogdian Sogd"
	script[11] = script[10] + " Elymaic Elym Nandinagari Nand Nyiakeng_Puachue_Hmong Hmnp Wancho Wcho"
	for _, v := range []int{9, 10, 11} {
		generalCategory, scripts := words(unicodeGeneralCategoryValues), words(script[v])
		unicodeData[v] = &unicodeProperties{
			binary: words(binary[v] + " " + unicodeGeneralCategoryValues),
			nonBinary: map[string]wordSet{
				"General_Category": generalCategory, "gc": generalCategory,
				"Script": scripts, "sc": scripts,
				"Script_Extensions": scripts, "scx": scripts,
			},
		}
	}
}

const ecma9BinaryProperties = "ASCII ASCII_Hex_Digit AHex Alphabetic Alpha Any Assigned Bidi_Control Bidi_C Bidi_Mirrored Bidi_M Case_Ignorable CI Cased Changes_When_Casefolded CWCF Changes_When_Casemapped CWCM Changes_When_Lowercased CWL Changes_When_NFKC_Casefolded CWKCF Changes_When_Titlecased CWT Changes_When_Uppercased CWU Dash Default_Ignorable_Code_Point DI Deprecated Dep Diacritic Dia Emoji Emoji_Component Emoji_Modifier Emoji_Modifier_Base Emoji_Presentation Extender Ext Grapheme_Base Gr_Base Grapheme_Extend Gr_Ext Hex_Digit Hex IDS_Binary_Operator IDSB IDS_Trinary_Operator IDST ID_Continue IDC ID_Start IDS Ideographic Ideo Join_Control Join_C Logical_Order_Exception LOE Lowercase Lower Math Noncharacter_Code_Point NChar Pattern_Syntax Pat_Syn Pattern_White_Space Pat_WS Quotation_Mark QMark Radical Regional_Indicator RI Sentence_Terminal STerm Soft_Dotted SD Terminal_Punctuation Term Unified_Ideograph UIdeo Uppercase Upper Variation_Selector VS White_Space space XID_Continue XIDC XID_Start XIDS"

const unicodeGeneralCategoryValues = "Cased_Letter LC Close_Punctuation Pe Connector_Punctuation Pc Control Cc cntrl Currency_Symbol Sc Dash_Punctuation Pd Decimal_Number Nd digit Enclosing_Mark Me Final_Punctuation Pf Format Cf Initial_Punctuation Pi Letter L Letter_Number Nl Line_Separator Zl Lowercase_Letter Ll Mark M Combining_Mark Math_Symbol Sm Modifier_Letter Lm Modifier_Symbol Sk Nonspacing_Mark Mn Number N Open_Punctuation Ps Other C Other_Letter Lo Other_Number No Other_Punctuation Po Other_Symbol So Paragraph_Separator Zp Private_Use Co Punctuation P punct Separator Z Space_Separator Zs Spacing_Mark Mc Surrogate Cs Symbol S Titlecase_Letter Lt Unassigned Cn Uppercase_Letter Lu"

const ecma9ScriptValues = "Adlam Adlm Ahom Anatolian_Hieroglyphs Hluw Arabic Arab Armenian Armn Avestan Avst Balinese Bali Bamum Bamu Bassa_Vah Bass Batak Batk Bengali Beng Bhaiksuki Bhks Bopomofo Bopo Brahmi Brah Braille Brai Buginese Bugi Buhid Buhd Canadian_Aboriginal Cans Carian Cari Caucasian_Albanian Aghb Chakma Cakm Cham Cham Cherokee Cher Common Zyyy Coptic Copt Qaac Cuneiform Xsux Cypriot Cprt Cyrillic Cyrl Deseret Dsrt Devanagari Deva Duployan Dupl Egyptian_Hieroglyphs Egyp Elbasan Elba Ethiopic Ethi Georgian Geor Glagolitic Glag Gothic Goth Grantha Gran Greek Grek Gujarati Gujr Gurmukhi Guru Han Hani Hangul Hang Hanunoo Hano Hatran Hatr Hebrew Hebr Hiragana Hira Imperial_Aramaic Armi Inherited Zinh Qaai Inscriptional_Pahlavi Phli Inscriptional_Parthian Prti Javanese Java Kaithi Kthi Kannada Knda Katakana Kana Kayah_Li Kali Kharoshthi Khar Khmer Khmr Khojki Khoj Khudawadi Sind Lao Laoo Latin Latn Lepcha Lepc Limbu Limb Linear_A Lina Linear_B Linb Lisu Lisu Lycian Lyci Lydian Lydi Mahajani Mahj Malayalam Mlym Mandaic Mand Manichaean Mani Marchen Marc Masaram_Gondi Gonm Meetei_Mayek Mtei Mende_Kikakui Mend Meroitic_Cursive Merc Meroitic_Hieroglyphs Mero Miao Plrd Modi Mongolian Mong Mro Mroo Multani Mult Myanmar Mymr Nabataean Nbat New_Tai_Lue Talu Newa Newa Nko Nkoo Nushu Nshu Ogham Ogam Ol_Chiki Olck Old_Hungarian Hung Old_Italic Ital Old_North_Arabian Narb Old_Permic Perm Old_Persian Xpeo Old_South_Arabian Sarb Old_Turkic Orkh Oriya Orya Osage Osge Osmanya Osma Pahawh_Hmong Hmng Palmyrene Palm Pau_Cin_Hau Pauc Phags_Pa Phag Phoenician Phnx Psalter_Pahlavi Phlp Rejang Rjng Runic Runr Samaritan Samr Saurashtra Saur Sharada Shrd Shavian Shaw Siddham Sidd SignWriting Sgnw Sinhala Sinh Sora_Sompeng Sora Soyombo Soyo Sundanese Sund Syloti_Nagri Sylo Syriac Syrc Tagalog Tglg Tagbanwa Tagb Tai_Le Tale Tai_Tham Lana Tai_Viet Tavt Takri Takr Tamil Taml Tangut Tang Telugu Telu Thaana Thaa Thai Thai Tibetan Tibt Tifinagh Tfng Tirhuta Tirh Ugaritic Ugar Vai Vaii Warang_Citi Wara Yi Yiii Zanabazar_Square Zanb"

// regexpState is acorn's RegExpValidationState: the pattern being validated and
// what the recursive descent has found so far.
type regexpState struct {
	parser            *parser
	validFlags        string
	unicodeProperties *unicodeProperties

	source  []uint16
	flags   string
	start   int
	switchU bool
	switchN bool
	pos     int
	// lastIntValue is a double, as acorn's: long digit runs round the way
	// JavaScript numbers do.
	lastIntValue                float64
	lastStringValue             []uint16
	lastAssertionIsQuantifiable bool
	numCapturingParens          float64
	maxBackReference            float64
	groupNames                  map[string]bool
	backReferenceNames          []string
}

func newRegexpState(p *parser) *regexpState {
	valid := "gim"
	if p.ecmaVersion >= 6 {
		valid += "uy"
	}
	if p.ecmaVersion >= 9 {
		valid += "s"
	}
	return &regexpState{parser: p, validFlags: valid, unicodeProperties: unicodeData[p.ecmaVersion]}
}

func (s *regexpState) reset(start int, pattern []uint16, flags string) {
	unicode := strings.Contains(flags, "u")
	s.start = start
	s.source = pattern
	s.flags = flags
	s.switchU = unicode && s.parser.ecmaVersion >= 6
	s.switchN = unicode && s.parser.ecmaVersion >= 9
}

func (s *regexpState) raise(message string) {
	s.parser.raiseRecoverable(s.start, "Invalid regular expression: /"+jsstr.FromUTF16(s.source)+"/: "+message)
}

// at returns the code point at i with the u flag (or forceU), combining a
// surrogate pair, and the code unit at i otherwise; -1 past the end.
func (s *regexpState) at(i int, forceU bool) rune {
	l := len(s.source)
	if i >= l {
		return -1
	}
	c := rune(s.source[i])
	if !(forceU || s.switchU) || c <= 0xd7ff || c >= 0xe000 || i+1 >= l {
		return c
	}
	if next := rune(s.source[i+1]); next >= 0xdc00 && next <= 0xdfff {
		return c<<10 + next - 0x35fdc00
	}
	return c
}

func (s *regexpState) nextIndex(i int, forceU bool) int {
	l := len(s.source)
	if i >= l {
		return l
	}
	c := s.source[i]
	if !(forceU || s.switchU) || c <= 0xd7ff || c >= 0xe000 || i+1 >= l ||
		s.source[i+1] < 0xdc00 || s.source[i+1] > 0xdfff {
		return i + 1
	}
	return i + 2
}

// current is the character at the position; currentForce reads a surrogate
// pair as one code point even without the u flag.
func (s *regexpState) current() rune                 { return s.at(s.pos, false) }
func (s *regexpState) currentForce(forceU bool) rune { return s.at(s.pos, forceU) }

func (s *regexpState) lookahead() rune { return s.at(s.nextIndex(s.pos, false), false) }

func (s *regexpState) advance()                 { s.advanceForce(false) }
func (s *regexpState) advanceForce(forceU bool) { s.pos = s.nextIndex(s.pos, forceU) }

// eat advances past ch if it is the current character.
func (s *regexpState) eat(ch rune) bool { return s.eatForce(ch, false) }

func (s *regexpState) eatForce(ch rune, forceU bool) bool {
	if s.currentForce(forceU) == ch {
		s.advanceForce(forceU)
		return true
	}
	return false
}

// appendLastString adds the code point ch to lastStringValue.
func (s *regexpState) appendLastString(ch rune) {
	s.lastStringValue = jsstr.AppendCodePoint(s.lastStringValue, ch)
}

func (s *regexpState) lastString() string { return jsstr.FromUTF16(s.lastStringValue) }

// validateRegExpFlags checks the flags of a regular expression literal.
func (p *parser) validateRegExpFlags(state *regexpState) {
	flags := state.flags
	for i := 0; i < len(flags); i++ {
		flag := flags[i : i+1]
		if !strings.Contains(state.validFlags, flag) {
			p.raise(state.start, "Invalid regular expression flag")
		}
		if strings.Contains(flags[i+1:], flag) {
			p.raise(state.start, "Duplicate regular expression flag")
		}
	}
}

// validateRegExpPattern checks the pattern of a regular expression literal.
func (p *parser) validateRegExpPattern(state *regexpState) {
	p.regexpPattern(state)

	// The goal symbol is Pattern[~U, ~N]. If the result contains a GroupName,
	// the pattern is parsed again with Pattern[~U, +N], whose result counts.
	if !state.switchN && p.ecmaVersion >= 9 && len(state.groupNames) > 0 {
		state.switchN = true
		p.regexpPattern(state)
	}
}

func (p *parser) regexpPattern(state *regexpState) {
	state.pos = 0
	state.lastIntValue = 0
	state.lastStringValue = nil
	state.lastAssertionIsQuantifiable = false
	state.numCapturingParens = 0
	state.maxBackReference = 0
	state.groupNames = map[string]bool{}
	state.backReferenceNames = state.backReferenceNames[:0]

	p.regexpDisjunction(state)

	if state.pos != len(state.source) {
		// The same messages as V8.
		if state.eat(')') {
			state.raise("Unmatched ')'")
		}
		if state.eat(']') || state.eat('}') {
			state.raise("Lone quantifier brackets")
		}
	}
	if state.maxBackReference > state.numCapturingParens {
		state.raise("Invalid escape")
	}
	for _, name := range state.backReferenceNames {
		if !state.groupNames[name] {
			state.raise("Invalid named capture referenced")
		}
	}
}

func (p *parser) regexpDisjunction(state *regexpState) {
	p.regexpAlternative(state)
	for state.eat('|') {
		p.regexpAlternative(state)
	}
	// The same message as V8.
	if p.regexpEatQuantifier(state, true) {
		state.raise("Nothing to repeat")
	}
	if state.eat('{') {
		state.raise("Lone quantifier brackets")
	}
}

func (p *parser) regexpAlternative(state *regexpState) {
	for state.pos < len(state.source) && p.regexpEatTerm(state) {
	}
}

// regexpEatTerm is Annex B's Term.
func (p *parser) regexpEatTerm(state *regexpState) bool {
	if p.regexpEatAssertion(state) {
		// A QuantifiableAssertion followed by a Quantifier: lastAssertionIsQuantifiable
		// tells whether the assertion just eaten is quantifiable.
		if state.lastAssertionIsQuantifiable && p.regexpEatQuantifier(state, false) {
			// The same message as V8.
			if state.switchU {
				state.raise("Invalid quantifier")
			}
		}
		return true
	}
	var atom bool
	if state.switchU {
		atom = p.regexpEatAtom(state)
	} else {
		atom = p.regexpEatExtendedAtom(state)
	}
	if atom {
		p.regexpEatQuantifier(state, false)
		return true
	}
	return false
}

// regexpEatAssertion is Annex B's Assertion.
func (p *parser) regexpEatAssertion(state *regexpState) bool {
	start := state.pos
	state.lastAssertionIsQuantifiable = false

	if state.eat('^') || state.eat('$') {
		return true
	}

	if state.eat('\\') {
		if state.eat('B') || state.eat('b') {
			return true
		}
		state.pos = start
	}

	// Lookahead and lookbehind.
	if state.eat('(') && state.eat('?') {
		lookbehind := false
		if p.ecmaVersion >= 9 {
			lookbehind = state.eat('<')
		}
		if state.eat('=') || state.eat('!') {
			p.regexpDisjunction(state)
			if !state.eat(')') {
				state.raise("Unterminated group")
			}
			state.lastAssertionIsQuantifiable = !lookbehind
			return true
		}
	}

	state.pos = start
	return false
}

func (p *parser) regexpEatQuantifier(state *regexpState, noError bool) bool {
	if p.regexpEatQuantifierPrefix(state, noError) {
		state.eat('?')
		return true
	}
	return false
}

func (p *parser) regexpEatQuantifierPrefix(state *regexpState, noError bool) bool {
	return state.eat('*') || state.eat('+') || state.eat('?') || p.regexpEatBracedQuantifier(state, noError)
}

func (p *parser) regexpEatBracedQuantifier(state *regexpState, noError bool) bool {
	start := state.pos
	if state.eat('{') {
		min, max := 0.0, -1.0
		if p.regexpEatDecimalDigits(state) {
			min = state.lastIntValue
			if state.eat(',') && p.regexpEatDecimalDigits(state) {
				max = state.lastIntValue
			}
			if state.eat('}') {
				if max != -1 && max < min && !noError {
					state.raise("numbers out of order in {} quantifier")
				}
				return true
			}
		}
		if state.switchU && !noError {
			state.raise("Incomplete quantifier")
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatAtom(state *regexpState) bool {
	return p.regexpEatPatternCharacters(state) ||
		state.eat('.') ||
		p.regexpEatReverseSolidusAtomEscape(state) ||
		p.regexpEatCharacterClass(state) ||
		p.regexpEatUncapturingGroup(state) ||
		p.regexpEatCapturingGroup(state)
}

func (p *parser) regexpEatReverseSolidusAtomEscape(state *regexpState) bool {
	start := state.pos
	if state.eat('\\') {
		if p.regexpEatAtomEscape(state) {
			return true
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatUncapturingGroup(state *regexpState) bool {
	start := state.pos
	if state.eat('(') {
		if state.eat('?') && state.eat(':') {
			p.regexpDisjunction(state)
			if state.eat(')') {
				return true
			}
			state.raise("Unterminated group")
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatCapturingGroup(state *regexpState) bool {
	if state.eat('(') {
		if p.ecmaVersion >= 9 {
			p.regexpGroupSpecifier(state)
		} else if state.current() == '?' {
			state.raise("Invalid group")
		}
		p.regexpDisjunction(state)
		if state.eat(')') {
			state.numCapturingParens++
			return true
		}
		state.raise("Unterminated group")
	}
	return false
}

// regexpEatExtendedAtom is Annex B's ExtendedAtom.
func (p *parser) regexpEatExtendedAtom(state *regexpState) bool {
	return state.eat('.') ||
		p.regexpEatReverseSolidusAtomEscape(state) ||
		p.regexpEatCharacterClass(state) ||
		p.regexpEatUncapturingGroup(state) ||
		p.regexpEatCapturingGroup(state) ||
		p.regexpEatInvalidBracedQuantifier(state) ||
		p.regexpEatExtendedPatternCharacter(state)
}

// regexpEatInvalidBracedQuantifier is Annex B's InvalidBracedQuantifier.
func (p *parser) regexpEatInvalidBracedQuantifier(state *regexpState) bool {
	if p.regexpEatBracedQuantifier(state, true) {
		state.raise("Nothing to repeat")
	}
	return false
}

func (p *parser) regexpEatSyntaxCharacter(state *regexpState) bool {
	ch := state.current()
	if isSyntaxCharacter(ch) {
		state.lastIntValue = float64(ch)
		state.advance()
		return true
	}
	return false
}

func isSyntaxCharacter(ch rune) bool {
	return ch == '$' || ch >= '(' && ch <= '+' || ch == '.' || ch == '?' || ch >= '[' && ch <= '^' || ch >= '{' && ch <= '}'
}

// regexpEatPatternCharacters eats a run of PatternCharacters.
func (p *parser) regexpEatPatternCharacters(state *regexpState) bool {
	start := state.pos
	for ch := state.current(); ch != -1 && !isSyntaxCharacter(ch); ch = state.current() {
		state.advance()
	}
	return state.pos != start
}

// regexpEatExtendedPatternCharacter is Annex B's ExtendedPatternCharacter.
func (p *parser) regexpEatExtendedPatternCharacter(state *regexpState) bool {
	ch := state.current()
	if ch != -1 && ch != '$' && !(ch >= '(' && ch <= '+') && ch != '.' && ch != '?' && ch != '[' && ch != '^' && ch != '|' {
		state.advance()
		return true
	}
	return false
}

// regexpGroupSpecifier reads the optional `?` GroupName of a capturing group.
func (p *parser) regexpGroupSpecifier(state *regexpState) {
	if state.eat('?') {
		if !p.regexpEatGroupName(state) {
			state.raise("Invalid group")
		}
		name := state.lastString()
		if state.groupNames[name] {
			state.raise("Duplicate capture group name")
		}
		state.groupNames[name] = true
	}
}

// regexpEatGroupName reads `<` RegExpIdentifierName `>` into lastStringValue.
func (p *parser) regexpEatGroupName(state *regexpState) bool {
	state.lastStringValue = nil
	if state.eat('<') {
		if p.regexpEatRegExpIdentifierName(state) && state.eat('>') {
			return true
		}
		state.raise("Invalid capture group name")
	}
	return false
}

// regexpEatRegExpIdentifierName reads a RegExpIdentifierName into lastStringValue.
func (p *parser) regexpEatRegExpIdentifierName(state *regexpState) bool {
	state.lastStringValue = nil
	if p.regexpEatRegExpIdentifierChar(state, true) {
		state.appendLastString(rune(state.lastIntValue))
		for p.regexpEatRegExpIdentifierChar(state, false) {
			state.appendLastString(rune(state.lastIntValue))
		}
		return true
	}
	return false
}

// regexpEatRegExpIdentifierChar eats a RegExpIdentifierStart (first) or a
// RegExpIdentifierPart: an identifier character, $, _, a \u escape of one, or, for
// a part, ZWNJ or ZWJ. From ECMAScript 2020 surrogate pairs and \u escapes of them
// count as one code point without the u flag too.
func (p *parser) regexpEatRegExpIdentifierChar(state *regexpState, first bool) bool {
	start := state.pos
	forceU := p.ecmaVersion >= 11
	ch := state.currentForce(forceU)
	state.advanceForce(forceU)

	if ch == '\\' && p.regexpEatRegExpUnicodeEscapeSequence(state, forceU) {
		ch = rune(state.lastIntValue)
	}
	valid := ch == '$' || ch == '_'
	if first {
		valid = valid || isIdentifierStart(ch, true)
	} else {
		valid = valid || isIdentifierChar(ch, true) || ch == 0x200c || ch == 0x200d
	}
	if valid {
		state.lastIntValue = float64(ch)
		return true
	}

	state.pos = start
	return false
}

// regexpEatAtomEscape is Annex B's AtomEscape.
func (p *parser) regexpEatAtomEscape(state *regexpState) bool {
	if p.regexpEatBackReference(state) ||
		p.regexpEatCharacterClassEscape(state) ||
		p.regexpEatCharacterEscape(state) ||
		state.switchN && p.regexpEatKGroupName(state) {
		return true
	}
	if state.switchU {
		// The same message as V8.
		if state.current() == 'c' {
			state.raise("Invalid unicode escape")
		}
		state.raise("Invalid escape")
	}
	return false
}

func (p *parser) regexpEatBackReference(state *regexpState) bool {
	start := state.pos
	if p.regexpEatDecimalEscape(state) {
		n := state.lastIntValue
		if state.switchU {
			// A reference to a group that does not exist is an error with the u
			// flag; regexpPattern checks it at the end.
			if n > state.maxBackReference {
				state.maxBackReference = n
			}
			return true
		}
		if n <= state.numCapturingParens {
			return true
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatKGroupName(state *regexpState) bool {
	if state.eat('k') {
		if p.regexpEatGroupName(state) {
			state.backReferenceNames = append(state.backReferenceNames, state.lastString())
			return true
		}
		state.raise("Invalid named reference")
	}
	return false
}

// regexpEatCharacterEscape is Annex B's CharacterEscape.
func (p *parser) regexpEatCharacterEscape(state *regexpState) bool {
	return p.regexpEatControlEscape(state) ||
		p.regexpEatCControlLetter(state) ||
		p.regexpEatZero(state) ||
		p.regexpEatHexEscapeSequence(state) ||
		p.regexpEatRegExpUnicodeEscapeSequence(state, false) ||
		!state.switchU && p.regexpEatLegacyOctalEscapeSequence(state) ||
		p.regexpEatIdentityEscape(state)
}

func (p *parser) regexpEatCControlLetter(state *regexpState) bool {
	start := state.pos
	if state.eat('c') {
		if p.regexpEatControlLetter(state) {
			return true
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatZero(state *regexpState) bool {
	if state.current() == '0' && !isDecimalDigit(state.lookahead()) {
		state.lastIntValue = 0
		state.advance()
		return true
	}
	return false
}

func (p *parser) regexpEatControlEscape(state *regexpState) bool {
	var value float64
	switch state.current() {
	case 't':
		value = '\t'
	case 'n':
		value = '\n'
	case 'v':
		value = '\v'
	case 'f':
		value = '\f'
	case 'r':
		value = '\r'
	default:
		return false
	}
	state.lastIntValue = value
	state.advance()
	return true
}

func (p *parser) regexpEatControlLetter(state *regexpState) bool {
	ch := state.current()
	if isControlLetter(ch) {
		state.lastIntValue = float64(ch % 0x20)
		state.advance()
		return true
	}
	return false
}

func isControlLetter(ch rune) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func (p *parser) regexpEatRegExpUnicodeEscapeSequence(state *regexpState, forceU bool) bool {
	start := state.pos
	switchU := forceU || state.switchU

	if state.eat('u') {
		if p.regexpEatFixedHexDigits(state, 4) {
			lead := state.lastIntValue
			if switchU && lead >= 0xd800 && lead <= 0xdbff {
				leadSurrogateEnd := state.pos
				if state.eat('\\') && state.eat('u') && p.regexpEatFixedHexDigits(state, 4) {
					trail := state.lastIntValue
					if trail >= 0xdc00 && trail <= 0xdfff {
						state.lastIntValue = (lead-0xd800)*0x400 + (trail - 0xdc00) + 0x10000
						return true
					}
				}
				state.pos = leadSurrogateEnd
				state.lastIntValue = lead
			}
			return true
		}
		if switchU && state.eat('{') && p.regexpEatHexDigits(state) && state.eat('}') &&
			state.lastIntValue >= 0 && state.lastIntValue <= 0x10ffff {
			return true
		}
		if switchU {
			state.raise("Invalid unicode escape")
		}
		state.pos = start
	}
	return false
}

// regexpEatIdentityEscape is Annex B's IdentityEscape.
func (p *parser) regexpEatIdentityEscape(state *regexpState) bool {
	if state.switchU {
		if p.regexpEatSyntaxCharacter(state) {
			return true
		}
		if state.eat('/') {
			state.lastIntValue = '/'
			return true
		}
		return false
	}

	ch := state.current()
	if ch != 'c' && (!state.switchN || ch != 'k') {
		state.lastIntValue = float64(ch)
		state.advance()
		return true
	}
	return false
}

func (p *parser) regexpEatDecimalEscape(state *regexpState) bool {
	state.lastIntValue = 0
	ch := state.current()
	if ch >= '1' && ch <= '9' {
		for isDecimalDigit(ch) {
			state.lastIntValue = float64(10*state.lastIntValue) + float64(ch-'0')
			state.advance()
			ch = state.current()
		}
		return true
	}
	return false
}

// regexpEatCharacterClassEscape eats \d \D \s \S \w \W and, with the u flag,
// \p{...} and \P{...}, setting lastIntValue to -1: they are not single characters.
func (p *parser) regexpEatCharacterClassEscape(state *regexpState) bool {
	ch := state.current()

	switch ch {
	case 'd', 'D', 's', 'S', 'w', 'W':
		state.lastIntValue = -1
		state.advance()
		return true
	}

	if state.switchU && p.ecmaVersion >= 9 && (ch == 'P' || ch == 'p') {
		state.lastIntValue = -1
		state.advance()
		if state.eat('{') && p.regexpEatUnicodePropertyValueExpression(state) && state.eat('}') {
			return true
		}
		state.raise("Invalid property name")
	}
	return false
}

// regexpEatUnicodePropertyValueExpression reads UnicodePropertyName `=`
// UnicodePropertyValue or a LoneUnicodePropertyNameOrValue and checks the names.
func (p *parser) regexpEatUnicodePropertyValueExpression(state *regexpState) bool {
	start := state.pos

	if p.regexpEatUnicodePropertyChars(state, false) && state.eat('=') {
		name := state.lastString()
		if p.regexpEatUnicodePropertyChars(state, true) {
			value := state.lastString()
			values, ok := state.unicodeProperties.nonBinary[name]
			if !ok {
				state.raise("Invalid property name")
			}
			if !values[value] {
				state.raise("Invalid property value")
			}
			return true
		}
	}
	state.pos = start

	if p.regexpEatUnicodePropertyChars(state, true) {
		if state.unicodeProperties.binary[state.lastString()] {
			return true
		}
		state.raise("Invalid property name")
	}
	return false
}

// regexpEatUnicodePropertyChars reads UnicodePropertyNameCharacters (letters and
// _) or, with digits, UnicodePropertyValueCharacters into lastStringValue.
func (p *parser) regexpEatUnicodePropertyChars(state *regexpState, digits bool) bool {
	state.lastStringValue = nil
	for ch := state.current(); isControlLetter(ch) || ch == '_' || digits && isDecimalDigit(ch); ch = state.current() {
		state.appendLastString(ch)
		state.advance()
	}
	return len(state.lastStringValue) > 0
}

func (p *parser) regexpEatCharacterClass(state *regexpState) bool {
	if state.eat('[') {
		state.eat('^')
		if state.current() != ']' {
			p.regexpNonEmptyClassRanges(state)
		}
		if !state.eat(']') {
			state.raise("Unterminated character class")
		}
		return true
	}
	return false
}

// regexpNonEmptyClassRanges reads the atoms and ranges of a character class.
func (p *parser) regexpNonEmptyClassRanges(state *regexpState) {
	for p.regexpEatClassAtom(state) {
		left := state.lastIntValue
		if state.eat('-') && p.regexpEatClassAtom(state) {
			right := state.lastIntValue
			if state.switchU && (left == -1 || right == -1) {
				state.raise("Invalid character class")
			}
			if left != -1 && right != -1 && left > right {
				state.raise("Range out of order in character class")
			}
		}
	}
}

// regexpEatClassAtom is Annex B's ClassAtom.
func (p *parser) regexpEatClassAtom(state *regexpState) bool {
	start := state.pos

	if state.eat('\\') {
		if p.regexpEatClassEscape(state) {
			return true
		}
		if state.switchU {
			// The same messages as V8.
			if ch := state.current(); ch == 'c' || isOctalDigit(ch) {
				state.raise("Invalid class escape")
			}
			state.raise("Invalid escape")
		}
		state.pos = start
	}

	ch := state.current()
	if ch != ']' {
		state.lastIntValue = float64(ch)
		state.advance()
		return true
	}
	return false
}

// regexpEatClassEscape is Annex B's ClassEscape.
func (p *parser) regexpEatClassEscape(state *regexpState) bool {
	start := state.pos

	if state.eat('b') {
		state.lastIntValue = '\b'
		return true
	}

	if state.switchU && state.eat('-') {
		state.lastIntValue = '-'
		return true
	}

	if !state.switchU && state.eat('c') {
		if p.regexpEatClassControlLetter(state) {
			return true
		}
		state.pos = start
	}

	return p.regexpEatCharacterClassEscape(state) || p.regexpEatCharacterEscape(state)
}

// regexpEatClassControlLetter is Annex B's ClassControlLetter.
func (p *parser) regexpEatClassControlLetter(state *regexpState) bool {
	ch := state.current()
	if isDecimalDigit(ch) || ch == '_' {
		state.lastIntValue = float64(ch % 0x20)
		state.advance()
		return true
	}
	return false
}

func (p *parser) regexpEatHexEscapeSequence(state *regexpState) bool {
	start := state.pos
	if state.eat('x') {
		if p.regexpEatFixedHexDigits(state, 2) {
			return true
		}
		if state.switchU {
			state.raise("Invalid escape")
		}
		state.pos = start
	}
	return false
}

func (p *parser) regexpEatDecimalDigits(state *regexpState) bool {
	start := state.pos
	state.lastIntValue = 0
	for ch := state.current(); isDecimalDigit(ch); ch = state.current() {
		state.lastIntValue = float64(10*state.lastIntValue) + float64(ch-'0')
		state.advance()
	}
	return state.pos != start
}

func isDecimalDigit(ch rune) bool { return ch >= '0' && ch <= '9' }

func (p *parser) regexpEatHexDigits(state *regexpState) bool {
	start := state.pos
	state.lastIntValue = 0
	for ch := state.current(); isHexDigit(ch); ch = state.current() {
		state.lastIntValue = float64(16*state.lastIntValue) + float64(hexToInt(ch))
		state.advance()
	}
	return state.pos != start
}

func isHexDigit(ch rune) bool {
	return ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'F' || ch >= 'a' && ch <= 'f'
}

func hexToInt(ch rune) rune {
	switch {
	case ch >= 'A' && ch <= 'F':
		return 10 + ch - 'A'
	case ch >= 'a' && ch <= 'f':
		return 10 + ch - 'a'
	}
	return ch - '0'
}

// regexpEatLegacyOctalEscapeSequence is Annex B's LegacyOctalEscapeSequence.
func (p *parser) regexpEatLegacyOctalEscapeSequence(state *regexpState) bool {
	if p.regexpEatOctalDigit(state) {
		n1 := state.lastIntValue
		if p.regexpEatOctalDigit(state) {
			n2 := state.lastIntValue
			if n1 <= 3 && p.regexpEatOctalDigit(state) {
				state.lastIntValue = n1*64 + n2*8 + state.lastIntValue
			} else {
				state.lastIntValue = n1*8 + n2
			}
		} else {
			state.lastIntValue = n1
		}
		return true
	}
	return false
}

func (p *parser) regexpEatOctalDigit(state *regexpState) bool {
	ch := state.current()
	if isOctalDigit(ch) {
		state.lastIntValue = float64(ch - '0')
		state.advance()
		return true
	}
	state.lastIntValue = 0
	return false
}

func isOctalDigit(ch rune) bool { return ch >= '0' && ch <= '7' }

func (p *parser) regexpEatFixedHexDigits(state *regexpState, length int) bool {
	start := state.pos
	state.lastIntValue = 0
	for i := 0; i < length; i++ {
		ch := state.current()
		if !isHexDigit(ch) {
			state.pos = start
			return false
		}
		state.lastIntValue = float64(16*state.lastIntValue) + float64(hexToInt(ch))
		state.advance()
	}
	return true
}
