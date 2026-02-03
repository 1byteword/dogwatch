package promql

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ItemType identifies the type of lexical token.
type ItemType int

const (
	ItemError ItemType = iota
	ItemEOF

	// Literals
	ItemIdentifier
	ItemMetricIdentifier
	ItemNumber
	ItemString
	ItemDuration

	// Operators
	ItemAdd      // +
	ItemSub      // -
	ItemMul      // *
	ItemDiv      // /
	ItemMod      // %
	ItemPow      // ^
	ItemEql      // ==
	ItemNeq      // !=
	ItemLss      // <
	ItemGtr      // >
	ItemLte      // <=
	ItemGte      // >=
	ItemEqlMatch // =~
	ItemNeqMatch // !~
	ItemAssign   // =

	// Delimiters
	ItemLeftParen    // (
	ItemRightParen   // )
	ItemLeftBracket  // [
	ItemRightBracket // ]
	ItemLeftBrace    // {
	ItemRightBrace   // }
	ItemComma        // ,
	ItemColon        // :
	ItemAt           // @

	// Keywords - Aggregation operators
	ItemSum
	ItemAvg
	ItemMin
	ItemMax
	ItemCount
	ItemStddev
	ItemStdvar
	ItemTopK
	ItemBottomK
	ItemCountValues
	ItemQuantile
	ItemGroup

	// Keywords - Aggregation modifiers
	ItemBy
	ItemWithout

	// Keywords - Binary operators
	ItemAnd
	ItemOr
	ItemUnless
	ItemAtan2

	// Keywords - Vector matching
	ItemOn
	ItemIgnoring
	ItemGroupLeft
	ItemGroupRight

	// Keywords - Other
	ItemBool
	ItemOffset
	ItemStart
	ItemEnd

	// Function names (stored as identifiers, but recognized during parsing)
)

var itemTypeStr = map[ItemType]string{
	ItemError:            "error",
	ItemEOF:              "EOF",
	ItemIdentifier:       "identifier",
	ItemMetricIdentifier: "metric",
	ItemNumber:           "number",
	ItemString:           "string",
	ItemDuration:         "duration",
	ItemAdd:              "+",
	ItemSub:              "-",
	ItemMul:              "*",
	ItemDiv:              "/",
	ItemMod:              "%",
	ItemPow:              "^",
	ItemEql:              "==",
	ItemNeq:              "!=",
	ItemLss:              "<",
	ItemGtr:              ">",
	ItemLte:              "<=",
	ItemGte:              ">=",
	ItemEqlMatch:         "=~",
	ItemNeqMatch:         "!~",
	ItemAssign:           "=",
	ItemLeftParen:        "(",
	ItemRightParen:       ")",
	ItemLeftBracket:      "[",
	ItemRightBracket:     "]",
	ItemLeftBrace:        "{",
	ItemRightBrace:       "}",
	ItemComma:            ",",
	ItemColon:            ":",
	ItemAt:               "@",
	ItemSum:              "sum",
	ItemAvg:              "avg",
	ItemMin:              "min",
	ItemMax:              "max",
	ItemCount:            "count",
	ItemStddev:           "stddev",
	ItemStdvar:           "stdvar",
	ItemTopK:             "topk",
	ItemBottomK:          "bottomk",
	ItemCountValues:      "count_values",
	ItemQuantile:         "quantile",
	ItemGroup:            "group",
	ItemBy:               "by",
	ItemWithout:          "without",
	ItemAnd:              "and",
	ItemOr:               "or",
	ItemUnless:           "unless",
	ItemAtan2:            "atan2",
	ItemOn:               "on",
	ItemIgnoring:         "ignoring",
	ItemGroupLeft:        "group_left",
	ItemGroupRight:       "group_right",
	ItemBool:             "bool",
	ItemOffset:           "offset",
	ItemStart:            "start",
	ItemEnd:              "end",
}

func (t ItemType) String() string {
	if s, ok := itemTypeStr[t]; ok {
		return s
	}
	return fmt.Sprintf("ItemType(%d)", t)
}

// IsOperator returns true if the item is an operator.
func (t ItemType) IsOperator() bool {
	switch t {
	case ItemAdd, ItemSub, ItemMul, ItemDiv, ItemMod, ItemPow,
		ItemEql, ItemNeq, ItemLss, ItemGtr, ItemLte, ItemGte,
		ItemAnd, ItemOr, ItemUnless, ItemAtan2:
		return true
	}
	return false
}

// IsAggregator returns true if the item is an aggregation operator.
func (t ItemType) IsAggregator() bool {
	switch t {
	case ItemSum, ItemAvg, ItemMin, ItemMax, ItemCount,
		ItemStddev, ItemStdvar, ItemTopK, ItemBottomK,
		ItemCountValues, ItemQuantile, ItemGroup:
		return true
	}
	return false
}

var keywords = map[string]ItemType{
	"sum":          ItemSum,
	"avg":          ItemAvg,
	"min":          ItemMin,
	"max":          ItemMax,
	"count":        ItemCount,
	"stddev":       ItemStddev,
	"stdvar":       ItemStdvar,
	"topk":         ItemTopK,
	"bottomk":      ItemBottomK,
	"count_values": ItemCountValues,
	"quantile":     ItemQuantile,
	"group":        ItemGroup,
	"by":           ItemBy,
	"without":      ItemWithout,
	"and":          ItemAnd,
	"or":           ItemOr,
	"unless":       ItemUnless,
	"atan2":        ItemAtan2,
	"on":           ItemOn,
	"ignoring":     ItemIgnoring,
	"group_left":   ItemGroupLeft,
	"group_right":  ItemGroupRight,
	"bool":         ItemBool,
	"offset":       ItemOffset,
	"start":        ItemStart,
	"end":          ItemEnd,
}

// Item represents a token returned by the lexer.
type Item struct {
	Typ ItemType
	Pos Pos
	Val string
}

func (i Item) String() string {
	switch {
	case i.Typ == ItemEOF:
		return "EOF"
	case i.Typ == ItemError:
		return i.Val
	case len(i.Val) > 20:
		return fmt.Sprintf("%.20q...", i.Val)
	}
	return fmt.Sprintf("%q", i.Val)
}

const eof = -1

// Lexer holds the state of the scanner.
type Lexer struct {
	input string
	pos   int
	start int
	width int
	line  int
	col   int
	items chan Item
}

// NewLexer creates a new lexer for the input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input: input,
		line:  1,
		col:   1,
		items: make(chan Item, 2),
	}
	return l
}

// Lex tokenizes the input and returns all items.
func Lex(input string) []Item {
	l := NewLexer(input)
	var items []Item
	for {
		item := l.NextItem()
		items = append(items, item)
		if item.Typ == ItemEOF || item.Typ == ItemError {
			break
		}
	}
	return items
}

// NextItem returns the next item from the input.
func (l *Lexer) NextItem() Item {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Item{Typ: ItemEOF, Pos: l.position()}
	}

	r := l.peek()

	// Handle operators and delimiters
	switch r {
	case '+':
		return l.singleChar(ItemAdd)
	case '-':
		return l.singleChar(ItemSub)
	case '*':
		return l.singleChar(ItemMul)
	case '/':
		return l.singleChar(ItemDiv)
	case '%':
		return l.singleChar(ItemMod)
	case '^':
		return l.singleChar(ItemPow)
	case '(':
		return l.singleChar(ItemLeftParen)
	case ')':
		return l.singleChar(ItemRightParen)
	case '[':
		return l.singleChar(ItemLeftBracket)
	case ']':
		return l.singleChar(ItemRightBracket)
	case '{':
		return l.singleChar(ItemLeftBrace)
	case '}':
		return l.singleChar(ItemRightBrace)
	case ',':
		return l.singleChar(ItemComma)
	case ':':
		return l.singleChar(ItemColon)
	case '@':
		return l.singleChar(ItemAt)
	case '=':
		return l.scanEquals()
	case '!':
		return l.scanBang()
	case '<':
		return l.scanLess()
	case '>':
		return l.scanGreater()
	case '"', '\'', '`':
		return l.scanString()
	}

	// Handle numbers
	if unicode.IsDigit(r) || (r == '.' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1]))) {
		return l.scanNumber()
	}

	// Handle identifiers and keywords
	if isIdentifierStart(r) {
		return l.scanIdentifier()
	}

	return Item{
		Typ: ItemError,
		Pos: l.position(),
		Val: fmt.Sprintf("unexpected character: %q", r),
	}
}

func (l *Lexer) position() Pos {
	return Pos{
		Offset: l.pos,
		Line:   l.line,
		Column: l.col,
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:])
	return r
}

func (l *Lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = w
	l.pos += w
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) backup() {
	l.pos -= l.width
	if l.input[l.pos] == '\n' {
		l.line--
		// Find column
		lineStart := strings.LastIndex(l.input[:l.pos], "\n")
		if lineStart < 0 {
			l.col = l.pos + 1
		} else {
			l.col = l.pos - lineStart
		}
	} else {
		l.col--
	}
}

func (l *Lexer) skipWhitespace() {
	for {
		r := l.peek()
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			l.next()
		} else if r == '#' {
			// Skip comments
			for {
				r = l.next()
				if r == '\n' || r == eof {
					break
				}
			}
		} else {
			break
		}
	}
}

func (l *Lexer) singleChar(typ ItemType) Item {
	pos := l.position()
	l.next()
	return Item{Typ: typ, Pos: pos, Val: itemTypeStr[typ]}
}

func (l *Lexer) scanEquals() Item {
	pos := l.position()
	l.next() // consume '='
	if l.peek() == '=' {
		l.next()
		return Item{Typ: ItemEql, Pos: pos, Val: "=="}
	}
	if l.peek() == '~' {
		l.next()
		return Item{Typ: ItemEqlMatch, Pos: pos, Val: "=~"}
	}
	return Item{Typ: ItemAssign, Pos: pos, Val: "="}
}

func (l *Lexer) scanBang() Item {
	pos := l.position()
	l.next() // consume '!'
	if l.peek() == '=' {
		l.next()
		return Item{Typ: ItemNeq, Pos: pos, Val: "!="}
	}
	if l.peek() == '~' {
		l.next()
		return Item{Typ: ItemNeqMatch, Pos: pos, Val: "!~"}
	}
	return Item{
		Typ: ItemError,
		Pos: pos,
		Val: "unexpected character after '!'",
	}
}

func (l *Lexer) scanLess() Item {
	pos := l.position()
	l.next() // consume '<'
	if l.peek() == '=' {
		l.next()
		return Item{Typ: ItemLte, Pos: pos, Val: "<="}
	}
	return Item{Typ: ItemLss, Pos: pos, Val: "<"}
}

func (l *Lexer) scanGreater() Item {
	pos := l.position()
	l.next() // consume '>'
	if l.peek() == '=' {
		l.next()
		return Item{Typ: ItemGte, Pos: pos, Val: ">="}
	}
	return Item{Typ: ItemGtr, Pos: pos, Val: ">"}
}

func (l *Lexer) scanString() Item {
	pos := l.position()
	quote := l.next() // consume opening quote
	var b strings.Builder

	for {
		r := l.next()
		if r == eof {
			return Item{Typ: ItemError, Pos: pos, Val: "unterminated string"}
		}
		if r == quote {
			break
		}
		if r == '\\' && quote != '`' {
			// Handle escape sequences
			r = l.next()
			switch r {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case '\'':
				b.WriteByte('\'')
			default:
				b.WriteByte('\\')
				b.WriteRune(r)
			}
		} else {
			b.WriteRune(r)
		}
	}

	return Item{Typ: ItemString, Pos: pos, Val: b.String()}
}

func (l *Lexer) scanNumber() Item {
	pos := l.position()
	start := l.pos

	// Handle leading digits
	l.acceptRun("0123456789")

	// Handle decimal point
	if l.peek() == '.' {
		l.next()
		l.acceptRun("0123456789")
	}

	// Handle exponent
	if l.peek() == 'e' || l.peek() == 'E' {
		l.next()
		if l.peek() == '+' || l.peek() == '-' {
			l.next()
		}
		l.acceptRun("0123456789")
	}

	// Check for Inf, NaN
	val := l.input[start:l.pos]
	if strings.EqualFold(val, "inf") || strings.EqualFold(val, "nan") {
		return Item{Typ: ItemNumber, Pos: pos, Val: val}
	}

	// Check if this is a duration (number followed by duration unit)
	r := l.peek()
	if r == 'y' || r == 'w' || r == 'd' || r == 'h' || r == 'm' || r == 's' {
		// This might be a duration
		return l.scanDuration(pos, start)
	}

	return Item{Typ: ItemNumber, Pos: pos, Val: val}
}

func (l *Lexer) scanDuration(pos Pos, start int) Item {
	// We're positioned after the initial digits, now we need to consume the unit
	// Continue scanning duration components: 1d2h30m15s
	for {
		// Accept duration unit (we know there's one waiting)
		r := l.peek()
		if r == 'y' || r == 'w' || r == 'd' || r == 'h' || r == 'm' || r == 's' {
			l.next()
			// Check for 'ms' specifically
			if r == 'm' && l.peek() == 's' {
				l.next()
			}
		} else {
			break
		}
		// Accept more digits for compound durations like 1h30m
		if !l.acceptRun("0123456789") {
			break
		}
	}

	val := l.input[start:l.pos]
	return Item{Typ: ItemDuration, Pos: pos, Val: val}
}

func (l *Lexer) acceptRun(valid string) bool {
	consumed := false
	for strings.ContainsRune(valid, l.peek()) {
		l.next()
		consumed = true
	}
	return consumed
}

func (l *Lexer) scanIdentifier() Item {
	pos := l.position()
	start := l.pos

	for {
		r := l.peek()
		if isIdentifierChar(r) {
			l.next()
		} else {
			break
		}
	}

	val := l.input[start:l.pos]
	lower := strings.ToLower(val)

	// Check for keywords
	if typ, ok := keywords[lower]; ok {
		return Item{Typ: typ, Pos: pos, Val: val}
	}

	// Check for special number values
	if lower == "inf" || lower == "nan" {
		return Item{Typ: ItemNumber, Pos: pos, Val: val}
	}

	return Item{Typ: ItemIdentifier, Pos: pos, Val: val}
}

func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentifierChar(r rune) bool {
	return r == '_' || r == ':' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
