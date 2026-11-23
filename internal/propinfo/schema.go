package propinfo

import "github.com/edge-js/edge-go/internal/jsval"

// The definitions of property-information 7.2.0's schemas, lib/aria.js,
// lib/html.js, lib/xlink.js, lib/xmlns.js and lib/xml.js, in their order: the
// properties with their types, the space, the special attribute names and the
// transform from a property name to its attribute name.

// The property types of lib/util/types.js, powers of two combined into masks.
const (
	boolean = 1 << (iota + 1)
	booleanish
	overloadedBoolean
	number
	spaceSeparated
	commaSeparated
	commaOrSpaceSeparated
)

// untyped is a property defined with the type null: none of the flags.
const untyped = 0

// property is one entry of a definition's properties: a property name and its
// type mask.
type property struct {
	name string
	mask int
}

// definition is the argument of create() in lib/util/create.js.
type definition struct {
	space           string
	attributes      map[string]string
	mustUseProperty []string
	properties      []property
	transform       func(attributes map[string]string, property string) string
}

// caseSensitiveTransform is lib/util/case-sensitive-transform.js: the special
// attribute name of attribute, else attribute itself.
func caseSensitiveTransform(attributes map[string]string, attribute string) string {
	if special, ok := attributes[attribute]; ok {
		return special
	}
	return attribute
}

// caseInsensitiveTransform is lib/util/case-insensitive-transform.js: the
// lowercased property, through caseSensitiveTransform.
func caseInsensitiveTransform(attributes map[string]string, property string) string {
	return caseSensitiveTransform(attributes, jsval.ToLowerCase(property))
}

// prefixTransform returns the transform of aria.js, xlink.js and xml.js:
// prefix followed by the lowercased property name after its first skip
// characters.
func prefixTransform(prefix string, skip int) func(map[string]string, string) string {
	return func(_ map[string]string, property string) string {
		return prefix + jsval.ToLowerCase(property[skip:])
	}
}

var aria = definition{
	properties: []property{

		{"ariaActiveDescendant", untyped},
		{"ariaAtomic", booleanish},
		{"ariaAutoComplete", untyped},
		{"ariaBusy", booleanish},
		{"ariaChecked", booleanish},
		{"ariaColCount", number},
		{"ariaColIndex", number},
		{"ariaColSpan", number},
		{"ariaControls", spaceSeparated},
		{"ariaCurrent", untyped},
		{"ariaDescribedBy", spaceSeparated},
		{"ariaDetails", untyped},
		{"ariaDisabled", booleanish},
		{"ariaDropEffect", spaceSeparated},
		{"ariaErrorMessage", untyped},
		{"ariaExpanded", booleanish},
		{"ariaFlowTo", spaceSeparated},
		{"ariaGrabbed", booleanish},
		{"ariaHasPopup", untyped},
		{"ariaHidden", booleanish},
		{"ariaInvalid", untyped},
		{"ariaKeyShortcuts", untyped},
		{"ariaLabel", untyped},
		{"ariaLabelledBy", spaceSeparated},
		{"ariaLevel", number},
		{"ariaLive", untyped},
		{"ariaModal", booleanish},
		{"ariaMultiLine", booleanish},
		{"ariaMultiSelectable", booleanish},
		{"ariaOrientation", untyped},
		{"ariaOwns", spaceSeparated},
		{"ariaPlaceholder", untyped},
		{"ariaPosInSet", number},
		{"ariaPressed", booleanish},
		{"ariaReadOnly", booleanish},
		{"ariaRelevant", untyped},
		{"ariaRequired", booleanish},
		{"ariaRoleDescription", spaceSeparated},
		{"ariaRowCount", number},
		{"ariaRowIndex", number},
		{"ariaRowSpan", number},
		{"ariaSelected", booleanish},
		{"ariaSetSize", number},
		{"ariaSort", untyped},
		{"ariaValueMax", number},
		{"ariaValueMin", number},
		{"ariaValueNow", number},
		{"ariaValueText", untyped},
		{"role", untyped},
	},
	transform: func(attributes map[string]string, property string) string {
		if property == "role" {
			return property
		}
		return prefixTransform("aria-", 4)(attributes, property)
	},
}

var html = definition{
	space: "html",
	attributes: map[string]string{
		"acceptcharset": "accept-charset",
		"classname":     "class",
		"htmlfor":       "for",
		"httpequiv":     "http-equiv",
	},
	mustUseProperty: []string{"checked", "multiple", "muted", "selected"},
	properties: []property{

		// Standard Properties.
		{"abbr", untyped},
		{"accept", commaSeparated},
		{"acceptCharset", spaceSeparated},
		{"accessKey", spaceSeparated},
		{"action", untyped},
		{"allow", untyped},
		{"allowFullScreen", boolean},
		{"allowPaymentRequest", boolean},
		{"allowUserMedia", boolean},
		{"alpha", boolean},
		{"alt", untyped},
		{"as", untyped},
		{"async", boolean},
		{"autoCapitalize", untyped},
		{"autoComplete", spaceSeparated},
		{"autoFocus", boolean},
		{"autoPlay", boolean},
		{"blocking", spaceSeparated},
		{"capture", untyped},
		{"charSet", untyped},
		{"checked", boolean},
		{"cite", untyped},
		{"className", spaceSeparated},
		{"closedBy", untyped},
		{"colorSpace", untyped},
		{"cols", number},
		{"colSpan", number},
		{"command", untyped},
		{"commandFor", untyped},
		{"content", untyped},
		{"contentEditable", booleanish},
		{"controls", boolean},
		{"controlsList", spaceSeparated},
		{"coords", number | commaSeparated},
		{"crossOrigin", untyped},
		{"data", untyped},
		{"dateTime", untyped},
		{"decoding", untyped},
		{"default", boolean},
		{"defer", boolean},
		{"dir", untyped},
		{"dirName", untyped},
		{"disabled", boolean},
		{"download", overloadedBoolean},
		{"draggable", booleanish},
		{"encType", untyped},
		{"enterKeyHint", untyped},
		{"fetchPriority", untyped},
		{"form", untyped},
		{"formAction", untyped},
		{"formEncType", untyped},
		{"formMethod", untyped},
		{"formNoValidate", boolean},
		{"formTarget", untyped},
		{"headers", spaceSeparated},
		{"height", number},
		{"hidden", overloadedBoolean},
		{"high", number},
		{"href", untyped},
		{"hrefLang", untyped},
		{"htmlFor", spaceSeparated},
		{"httpEquiv", spaceSeparated},
		{"id", untyped},
		{"imageSizes", untyped},
		{"imageSrcSet", untyped},
		{"inert", boolean},
		{"inputMode", untyped},
		{"integrity", untyped},
		{"is", untyped},
		{"isMap", boolean},
		{"itemId", untyped},
		{"itemProp", spaceSeparated},
		{"itemRef", spaceSeparated},
		{"itemScope", boolean},
		{"itemType", spaceSeparated},
		{"kind", untyped},
		{"label", untyped},
		{"lang", untyped},
		{"language", untyped},
		{"list", untyped},
		{"loading", untyped},
		{"loop", boolean},
		{"low", number},
		{"manifest", untyped},
		{"max", untyped},
		{"maxLength", number},
		{"media", untyped},
		{"method", untyped},
		{"min", untyped},
		{"minLength", number},
		{"multiple", boolean},
		{"muted", boolean},
		{"name", untyped},
		{"nonce", untyped},
		{"noModule", boolean},
		{"noValidate", boolean},
		{"onAbort", untyped},
		{"onAfterPrint", untyped},
		{"onAuxClick", untyped},
		{"onBeforeMatch", untyped},
		{"onBeforePrint", untyped},
		{"onBeforeToggle", untyped},
		{"onBeforeUnload", untyped},
		{"onBlur", untyped},
		{"onCancel", untyped},
		{"onCanPlay", untyped},
		{"onCanPlayThrough", untyped},
		{"onChange", untyped},
		{"onClick", untyped},
		{"onClose", untyped},
		{"onContextLost", untyped},
		{"onContextMenu", untyped},
		{"onContextRestored", untyped},
		{"onCopy", untyped},
		{"onCueChange", untyped},
		{"onCut", untyped},
		{"onDblClick", untyped},
		{"onDrag", untyped},
		{"onDragEnd", untyped},
		{"onDragEnter", untyped},
		{"onDragExit", untyped},
		{"onDragLeave", untyped},
		{"onDragOver", untyped},
		{"onDragStart", untyped},
		{"onDrop", untyped},
		{"onDurationChange", untyped},
		{"onEmptied", untyped},
		{"onEnded", untyped},
		{"onError", untyped},
		{"onFocus", untyped},
		{"onFormData", untyped},
		{"onHashChange", untyped},
		{"onInput", untyped},
		{"onInvalid", untyped},
		{"onKeyDown", untyped},
		{"onKeyPress", untyped},
		{"onKeyUp", untyped},
		{"onLanguageChange", untyped},
		{"onLoad", untyped},
		{"onLoadedData", untyped},
		{"onLoadedMetadata", untyped},
		{"onLoadEnd", untyped},
		{"onLoadStart", untyped},
		{"onMessage", untyped},
		{"onMessageError", untyped},
		{"onMouseDown", untyped},
		{"onMouseEnter", untyped},
		{"onMouseLeave", untyped},
		{"onMouseMove", untyped},
		{"onMouseOut", untyped},
		{"onMouseOver", untyped},
		{"onMouseUp", untyped},
		{"onOffline", untyped},
		{"onOnline", untyped},
		{"onPageHide", untyped},
		{"onPageShow", untyped},
		{"onPaste", untyped},
		{"onPause", untyped},
		{"onPlay", untyped},
		{"onPlaying", untyped},
		{"onPopState", untyped},
		{"onProgress", untyped},
		{"onRateChange", untyped},
		{"onRejectionHandled", untyped},
		{"onReset", untyped},
		{"onResize", untyped},
		{"onScroll", untyped},
		{"onScrollEnd", untyped},
		{"onSecurityPolicyViolation", untyped},
		{"onSeeked", untyped},
		{"onSeeking", untyped},
		{"onSelect", untyped},
		{"onSlotChange", untyped},
		{"onStalled", untyped},
		{"onStorage", untyped},
		{"onSubmit", untyped},
		{"onSuspend", untyped},
		{"onTimeUpdate", untyped},
		{"onToggle", untyped},
		{"onUnhandledRejection", untyped},
		{"onUnload", untyped},
		{"onVolumeChange", untyped},
		{"onWaiting", untyped},
		{"onWheel", untyped},
		{"open", boolean},
		{"optimum", number},
		{"pattern", untyped},
		{"ping", spaceSeparated},
		{"placeholder", untyped},
		{"playsInline", boolean},
		{"popover", untyped},
		{"popoverTarget", untyped},
		{"popoverTargetAction", untyped},
		{"poster", untyped},
		{"preload", untyped},
		{"readOnly", boolean},
		{"referrerPolicy", untyped},
		{"rel", spaceSeparated},
		{"required", boolean},
		{"reversed", boolean},
		{"rows", number},
		{"rowSpan", number},
		{"sandbox", spaceSeparated},
		{"scope", untyped},
		{"scoped", boolean},
		{"seamless", boolean},
		{"selected", boolean},
		{"shadowRootClonable", boolean},
		{"shadowRootCustomElementRegistry", boolean},
		{"shadowRootDelegatesFocus", boolean},
		{"shadowRootMode", untyped},
		{"shadowRootSerializable", boolean},
		{"shape", untyped},
		{"size", number},
		{"sizes", untyped},
		{"slot", untyped},
		{"span", number},
		{"spellCheck", booleanish},
		{"src", untyped},
		{"srcDoc", untyped},
		{"srcLang", untyped},
		{"srcSet", untyped},
		{"start", number},
		{"step", untyped},
		{"style", untyped},
		{"tabIndex", number},
		{"target", untyped},
		{"title", untyped},
		{"translate", untyped},
		{"type", untyped},
		{"typeMustMatch", boolean},
		{"useMap", untyped},
		{"value", booleanish},
		{"width", number},
		{"wrap", untyped},
		{"writingSuggestions", untyped},

		// Legacy.
		// See: https://html.spec.whatwg.org/#other-elements,-attributes-and-apis
		{"align", untyped},          // Several. Use CSS `text-align` instead,
		{"aLink", untyped},          // `<body>`. Use CSS `a:active {color}` instead
		{"archive", spaceSeparated}, // `<object>`. List of URIs to archives
		{"axis", untyped},           // `<td>` and `<th>`. Use `scope` on `<th>`
		{"background", untyped},     // `<body>`. Use CSS `background-image` instead
		{"bgColor", untyped},        // `<body>` and table elements. Use CSS `background-color` instead
		{"border", number},          // `<table>`. Use CSS `border-width` instead,
		{"borderColor", untyped},    // `<table>`. Use CSS `border-color` instead,
		{"bottomMargin", number},    // `<body>`
		{"cellPadding", untyped},    // `<table>`
		{"cellSpacing", untyped},    // `<table>`
		{"char", untyped},           // Several table elements. When `align=char`, sets the character to align on
		{"charOff", untyped},        // Several table elements. When `char`, offsets the alignment
		{"classId", untyped},        // `<object>`
		{"clear", untyped},          // `<br>`. Use CSS `clear` instead
		{"code", untyped},           // `<object>`
		{"codeBase", untyped},       // `<object>`
		{"codeType", untyped},       // `<object>`
		{"color", untyped},          // `<font>` and `<hr>`. Use CSS instead
		{"compact", boolean},        // Lists. Use CSS to reduce space between items instead
		{"declare", boolean},        // `<object>`
		{"event", untyped},          // `<script>`
		{"face", untyped},           // `<font>`. Use CSS instead
		{"frame", untyped},          // `<table>`
		{"frameBorder", untyped},    // `<iframe>`. Use CSS `border` instead
		{"hSpace", number},          // `<img>` and `<object>`
		{"leftMargin", number},      // `<body>`
		{"link", untyped},           // `<body>`. Use CSS `a:link {color: *}` instead
		{"longDesc", untyped},       // `<frame>`, `<iframe>`, and `<img>`. Use an `<a>`
		{"lowSrc", untyped},         // `<img>`. Use a `<picture>`
		{"marginHeight", number},    // `<body>`
		{"marginWidth", number},     // `<body>`
		{"noResize", boolean},       // `<frame>`
		{"noHref", boolean},         // `<area>`. Use no href instead of an explicit `nohref`
		{"noShade", boolean},        // `<hr>`. Use background-color and height instead of borders
		{"noWrap", boolean},         // `<td>` and `<th>`
		{"object", untyped},         // `<applet>`
		{"profile", untyped},        // `<head>`
		{"prompt", untyped},         // `<isindex>`
		{"rev", untyped},            // `<link>`
		{"rightMargin", number},     // `<body>`
		{"rules", untyped},          // `<table>`
		{"scheme", untyped},         // `<meta>`
		{"scrolling", booleanish},   // `<frame>`. Use overflow in the child context
		{"standby", untyped},        // `<object>`
		{"summary", untyped},        // `<table>`
		{"text", untyped},           // `<body>`. Use CSS `color` instead
		{"topMargin", number},       // `<body>`
		{"valueType", untyped},      // `<param>`
		{"version", untyped},        // `<html>`. Use a doctype.
		{"vAlign", untyped},         // Several. Use CSS `vertical-align` instead
		{"vLink", untyped},          // `<body>`. Use CSS `a:visited {color}` instead
		{"vSpace", number},          // `<img>` and `<object>`

		// Non-standard Properties.
		{"allowTransparency", untyped},
		{"autoCorrect", untyped},
		{"autoSave", untyped},
		{"credentialless", boolean},
		{"disablePictureInPicture", boolean},
		{"disableRemotePlayback", boolean},
		{"exportParts", commaSeparated},
		{"part", spaceSeparated},
		{"prefix", untyped},
		{"property", untyped},
		{"results", number},
		{"security", untyped},
		{"unselectable", untyped},
	},
	transform: caseInsensitiveTransform,
}

var xlink = definition{
	space: "xlink",
	properties: []property{
		{"xLinkActuate", untyped},
		{"xLinkArcRole", untyped},
		{"xLinkHref", untyped},
		{"xLinkRole", untyped},
		{"xLinkShow", untyped},
		{"xLinkTitle", untyped},
		{"xLinkType", untyped},
	},
	transform: prefixTransform("xlink:", 5),
}

var xmlns = definition{
	space:      "xmlns",
	attributes: map[string]string{"xmlnsxlink": "xmlns:xlink"},
	properties: []property{
		{"xmlnsXLink", untyped},
		{"xmlns", untyped},
	},
	transform: caseInsensitiveTransform,
}

var xml = definition{
	space: "xml",
	properties: []property{
		{"xmlBase", untyped},
		{"xmlLang", untyped},
		{"xmlSpace", untyped},
	},
	transform: prefixTransform("xml:", 3),
}
