package query

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenType represents the type of token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenError

	// Literals
	TokenIdent
	TokenString
	TokenNumber
	TokenDuration
	TokenRegex

	// Operators
	TokenPipe       // |
	TokenDot        // .
	TokenComma      // ,
	TokenLParen     // (
	TokenRParen     // )
	TokenLBracket   // [
	TokenRBracket   // ]
	TokenEq         // =
	TokenNeq        // !=
	TokenLt         // <
	TokenLte        // <=
	TokenGt         // >
	TokenGte        // >=
	TokenPlus       // +
	TokenMinus      // -
	TokenStar       // *
	TokenSlash      // /
	TokenPercent    // %
	TokenTilde      // ~
	TokenPlusMinus  // ±

	// Keywords
	TokenMetrics
	TokenLogs
	TokenTraces
	TokenEvents
	TokenWhere
	TokenSelect
	TokenAs
	TokenBy
	TokenAnd
	TokenOr
	TokenNot
	TokenIn
	TokenLike
	TokenMatches
	TokenBetween
	TokenLast
	TokenOrder
	TokenAsc
	TokenDesc
	TokenLimit
	TokenOffset
	TokenTop
	TokenWindow
	TokenTumbling
	TokenSliding
	TokenSession
	TokenCorrelate
	TokenOn
	TokenTime
	TokenAnomalies
	TokenAlgorithm
	TokenSensitivity
	TokenExtract
	TokenPattern
	TokenAuto
	TokenHistogram
	TokenBuckets
	TokenDefine
	TokenTrue
	TokenFalse
	TokenNull
	TokenShift
	TokenCompare
	TokenBaseline
	TokenDistinct
	TokenCount
	TokenSum
	TokenAvg
	TokenMin
	TokenMax
	TokenP50
	TokenP90
	TokenP95
	TokenP99
)

var keywords = map[string]TokenType{
	"metrics":     TokenMetrics,
	"logs":        TokenLogs,
	"traces":      TokenTraces,
	"events":      TokenEvents,
	"where":       TokenWhere,
	"select":      TokenSelect,
	"as":          TokenAs,
	"by":          TokenBy,
	"and":         TokenAnd,
	"or":          TokenOr,
	"not":         TokenNot,
	"in":          TokenIn,
	"like":        TokenLike,
	"matches":     TokenMatches,
	"between":     TokenBetween,
	"last":        TokenLast,
	"order":       TokenOrder,
	"asc":         TokenAsc,
	"desc":        TokenDesc,
	"limit":       TokenLimit,
	"offset":      TokenOffset,
	"top":         TokenTop,
	"window":      TokenWindow,
	"tumbling":    TokenTumbling,
	"sliding":     TokenSliding,
	"session":     TokenSession,
	"correlate":   TokenCorrelate,
	"on":          TokenOn,
	"time":        TokenTime,
	"anomalies":   TokenAnomalies,
	"algorithm":   TokenAlgorithm,
	"sensitivity": TokenSensitivity,
	"extract":     TokenExtract,
	"pattern":     TokenPattern,
	"auto":        TokenAuto,
	"histogram":   TokenHistogram,
	"buckets":     TokenBuckets,
	"define":      TokenDefine,
	"true":        TokenTrue,
	"false":       TokenFalse,
	"null":        TokenNull,
	"shift":       TokenShift,
	"compare":     TokenCompare,
	"baseline":    TokenBaseline,
	"distinct":    TokenDistinct,
	"count":       TokenCount,
	"sum":         TokenSum,
	"avg":         TokenAvg,
	"min":         TokenMin,
	"max":         TokenMax,
	"p50":         TokenP50,
	"p90":         TokenP90,
	"p95":         TokenP95,
	"p99":         TokenP99,
}

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Value   string
	Pos     Position
}

func (t Token) String() string {
	if t.Type == TokenEOF {
		return "EOF"
	}
	if t.Type == TokenError {
		return fmt.Sprintf("ERROR(%s)", t.Value)
	}
	if len(t.Value) > 20 {
		return fmt.Sprintf("%v(%.20s...)", t.Type, t.Value)
	}
	return fmt.Sprintf("%v(%s)", t.Type, t.Value)
}

// Lexer tokenizes WatchQL input
type Lexer struct {
	input  string
	pos    int
	line   int
	col    int
	start  int
	tokens []Token
}

// NewLexer creates a new lexer
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: input,
		line:  1,
		col:   1,
	}
}

// Tokenize returns all tokens
func (l *Lexer) Tokenize() ([]Token, error) {
	for {
		tok := l.nextToken()
		l.tokens = append(l.tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
		if tok.Type == TokenError {
			return l.tokens, fmt.Errorf("lexer error at %s: %s", tok.Pos, tok.Value)
		}
	}
	return l.tokens, nil
}

func (l *Lexer) nextToken() Token {
	l.skipWhitespace()
	l.start = l.pos

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Pos: l.position()}
	}

	ch := l.peek()

	// Single character tokens
	switch ch {
	case '|':
		l.advance()
		return Token{Type: TokenPipe, Value: "|", Pos: l.position()}
	case '.':
		l.advance()
		return Token{Type: TokenDot, Value: ".", Pos: l.position()}
	case ',':
		l.advance()
		return Token{Type: TokenComma, Value: ",", Pos: l.position()}
	case '(':
		l.advance()
		return Token{Type: TokenLParen, Value: "(", Pos: l.position()}
	case ')':
		l.advance()
		return Token{Type: TokenRParen, Value: ")", Pos: l.position()}
	case '[':
		l.advance()
		return Token{Type: TokenLBracket, Value: "[", Pos: l.position()}
	case ']':
		l.advance()
		return Token{Type: TokenRBracket, Value: "]", Pos: l.position()}
	case '+':
		l.advance()
		return Token{Type: TokenPlus, Value: "+", Pos: l.position()}
	case '-':
		l.advance()
		// Check if it's a negative number
		if l.pos < len(l.input) && (unicode.IsDigit(rune(l.input[l.pos]))) {
			return l.scanNumber(true)
		}
		return Token{Type: TokenMinus, Value: "-", Pos: l.position()}
	case '*':
		l.advance()
		return Token{Type: TokenStar, Value: "*", Pos: l.position()}
	case '/':
		l.advance()
		// Check for regex
		if l.peekPrev() == '~' || l.start == 0 || l.tokens[len(l.tokens)-1].Type == TokenMatches {
			return l.scanRegex()
		}
		return Token{Type: TokenSlash, Value: "/", Pos: l.position()}
	case '%':
		l.advance()
		return Token{Type: TokenPercent, Value: "%", Pos: l.position()}
	case '~':
		l.advance()
		return Token{Type: TokenTilde, Value: "~", Pos: l.position()}
	case '±':
		l.advance()
		return Token{Type: TokenPlusMinus, Value: "±", Pos: l.position()}
	case '=':
		l.advance()
		return Token{Type: TokenEq, Value: "=", Pos: l.position()}
	case '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenNeq, Value: "!=", Pos: l.position()}
		}
		return Token{Type: TokenError, Value: "unexpected character '!'", Pos: l.position()}
	case '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenLte, Value: "<=", Pos: l.position()}
		}
		return Token{Type: TokenLt, Value: "<", Pos: l.position()}
	case '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenGte, Value: ">=", Pos: l.position()}
		}
		return Token{Type: TokenGt, Value: ">", Pos: l.position()}
	case '"', '\'':
		return l.scanString(ch)
	}

	// Numbers
	if unicode.IsDigit(rune(ch)) {
		return l.scanNumber(false)
	}

	// Identifiers and keywords
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.scanIdentifier()
	}

	l.advance()
	return Token{Type: TokenError, Value: fmt.Sprintf("unexpected character '%c'", ch), Pos: l.position()}
}

func (l *Lexer) scanIdentifier() Token {
	pos := l.position()
	start := l.pos

	for l.pos < len(l.input) {
		ch := l.peek()
		if !unicode.IsLetter(rune(ch)) && !unicode.IsDigit(rune(ch)) && ch != '_' {
			break
		}
		l.advance()
	}

	value := l.input[start:l.pos]
	lower := strings.ToLower(value)

	// Check for keywords
	if tokType, ok := keywords[lower]; ok {
		return Token{Type: tokType, Value: lower, Pos: pos}
	}

	return Token{Type: TokenIdent, Value: value, Pos: pos}
}

func (l *Lexer) scanNumber(negative bool) Token {
	pos := l.position()
	start := l.pos
	if negative {
		start-- // include the minus sign
	}

	hasDot := false
	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == '.' && !hasDot {
			hasDot = true
			l.advance()
			continue
		}
		if !unicode.IsDigit(rune(ch)) {
			break
		}
		l.advance()
	}

	// Check for duration suffix
	if l.pos < len(l.input) {
		suffix := l.peekWord()
		if isDurationSuffix(suffix) {
			l.pos += len(suffix)
			return Token{Type: TokenDuration, Value: l.input[start:l.pos], Pos: pos}
		}
	}

	return Token{Type: TokenNumber, Value: l.input[start:l.pos], Pos: pos}
}

func (l *Lexer) scanString(quote byte) Token {
	pos := l.position()
	l.advance() // consume opening quote
	start := l.pos

	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == quote {
			value := l.input[start:l.pos]
			l.advance() // consume closing quote
			return Token{Type: TokenString, Value: value, Pos: pos}
		}
		if ch == '\\' {
			l.advance() // skip escape
		}
		l.advance()
	}

	return Token{Type: TokenError, Value: "unterminated string", Pos: pos}
}

func (l *Lexer) scanRegex() Token {
	pos := l.position()
	start := l.pos

	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == '/' {
			value := l.input[start:l.pos]
			l.advance() // consume closing /
			return Token{Type: TokenRegex, Value: value, Pos: pos}
		}
		if ch == '\\' {
			l.advance() // skip escape
		}
		l.advance()
	}

	return Token{Type: TokenError, Value: "unterminated regex", Pos: pos}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else if ch == '\n' {
			l.advance()
			l.line++
			l.col = 1
		} else if ch == '-' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '-' {
			// Comment: -- until end of line
			for l.pos < len(l.input) && l.peek() != '\n' {
				l.advance()
			}
		} else {
			break
		}
	}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) peekPrev() byte {
	if l.pos <= 0 {
		return 0
	}
	return l.input[l.pos-1]
}

func (l *Lexer) peekWord() string {
	end := l.pos
	for end < len(l.input) {
		ch := l.input[end]
		if !unicode.IsLetter(rune(ch)) {
			break
		}
		end++
	}
	return l.input[l.pos:end]
}

func (l *Lexer) advance() {
	if l.pos < len(l.input) {
		_, size := utf8.DecodeRuneInString(l.input[l.pos:])
		l.pos += size
		l.col++
	}
}

func (l *Lexer) position() Position {
	return Position{Line: l.line, Column: l.col, Offset: l.start}
}

func isDurationSuffix(s string) bool {
	s = strings.ToLower(s)
	switch s {
	case "ns", "us", "µs", "ms", "s", "m", "h", "d", "w":
		return true
	}
	return false
}

// TokenName returns a human-readable name for a token type
func TokenName(t TokenType) string {
	names := map[TokenType]string{
		TokenEOF:         "end of input",
		TokenError:       "error",
		TokenIdent:       "identifier",
		TokenString:      "string",
		TokenNumber:      "number",
		TokenDuration:    "duration",
		TokenRegex:       "regex",
		TokenPipe:        "'|'",
		TokenDot:         "'.'",
		TokenComma:       "','",
		TokenLParen:      "'('",
		TokenRParen:      "')'",
		TokenLBracket:    "'['",
		TokenRBracket:    "']'",
		TokenEq:          "'='",
		TokenNeq:         "'!='",
		TokenLt:          "'<'",
		TokenLte:         "'<='",
		TokenGt:          "'>'",
		TokenGte:         "'>='",
		TokenPlus:        "'+'",
		TokenMinus:       "'-'",
		TokenStar:        "'*'",
		TokenSlash:       "'/'",
		TokenPercent:     "'%'",
		TokenMetrics:     "'metrics'",
		TokenLogs:        "'logs'",
		TokenTraces:      "'traces'",
		TokenEvents:      "'events'",
		TokenWhere:       "'where'",
		TokenSelect:      "'select'",
		TokenAs:          "'as'",
		TokenBy:          "'by'",
		TokenAnd:         "'and'",
		TokenOr:          "'or'",
		TokenNot:         "'not'",
		TokenIn:          "'in'",
		TokenLike:        "'like'",
		TokenMatches:     "'matches'",
		TokenBetween:     "'between'",
		TokenLast:        "'last'",
		TokenOrder:       "'order'",
		TokenAsc:         "'asc'",
		TokenDesc:        "'desc'",
		TokenLimit:       "'limit'",
		TokenOffset:      "'offset'",
		TokenTop:         "'top'",
		TokenWindow:      "'window'",
		TokenCorrelate:   "'correlate'",
		TokenOn:          "'on'",
		TokenTime:        "'time'",
		TokenAnomalies:   "'anomalies'",
		TokenExtract:     "'extract'",
		TokenHistogram:   "'histogram'",
		TokenDefine:      "'define'",
		TokenTrue:        "'true'",
		TokenFalse:       "'false'",
		TokenNull:        "'null'",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("token(%d)", t)
}
