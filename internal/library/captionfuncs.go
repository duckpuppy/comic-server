package library

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Port of the 13 functions under ComicRackCE's
// cYo.Common/Text/FunctionParser/Functions. Results are rendered the way C#'s
// ToString() renders them ("True"/"False" for booleans, etc.).

// runCaptionFunction runs one $name<args> call. Wrong argument counts, bad
// dates, unknown functions and the like panic with a captionError, which
// formatCaption turns into ComicRack's "#ERROR - ..." output.
func runCaptionFunction(name string, args []string) string {
	need := func(n int) {
		if len(args) != n {
			captionFail("Expected %d parameters, but got %d.", n, len(args))
		}
	}

	switch name {
	case "if":
		need(3)
		b, known := captionBoolEval(args[0])
		if !known {
			return ""
		}
		if b {
			return args[1]
		}
		return args[2]

	case "not":
		need(1)
		b, known := captionBoolEval(args[0])
		if !known {
			captionFail("Nullable object must have a value.")
		}
		return dotNetBool(!b)

	case "year", "month", "day":
		need(1)
		if args[0] == "" {
			return ""
		}
		t, ok := parseDotNetDate(args[0])
		if !ok {
			captionFail("Can't parse date")
		}
		switch name {
		case "year":
			return strconv.Itoa(t.Year())
		case "month":
			return fmt.Sprintf("%04d", int(t.Month()))
		default:
			return fmt.Sprintf("%04d", t.Day())
		}

	case "date":
		need(2)
		if args[0] == "" {
			return ""
		}
		t, ok := parseDotNetDate(args[0])
		if !ok {
			captionFail("Can't parse date")
		}
		return formatDotNetDate(t, args[1])

	case "double":
		need(1)
		f, ok := parseCaptionNumber(args[0])
		if strings.TrimSpace(args[0]) == "" || !ok {
			return "-1"
		}
		return formatCaptionFloat(f)

	case "int":
		need(1)
		n, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 32)
		if err != nil {
			return "-1"
		}
		return strconv.FormatInt(n, 10)

	case "expr":
		need(1)
		v, err := evalCaptionExpr(args[0])
		if err != nil {
			captionFail("%s", err.Error())
		}
		if v == nil {
			captionFail("Object reference not set to an instance of an object.")
		}
		return captionValueString(v)

	case "escape":
		need(1)
		return captionEscape(args[0])

	case "substring":
		need(3)
		return captionSubstring(args[0], args[1], args[2])

	case "regexmatch":
		need(2)
		re := captionRegex(args[1])
		return dotNetBool(re.MatchString(args[0]))

	case "regexreplace":
		need(3)
		re := captionRegex(args[1])
		return re.ReplaceAllString(args[0], dotNetReplacement(args[2]))
	}

	captionFail("%s Function not implemented.", name)
	return ""
}

func dotNetBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// captionBoolEval mirrors FunctionParametersEval.BoolEval: the literal text
// true/false (any case), else the value of a C#-style boolean expression.
// known=false is ComicRack's null (empty text, or a non-boolean result).
func captionBoolEval(text string) (value, known bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true":
		return true, true
	case "false":
		return false, true
	case "":
		return false, false
	}
	v, err := evalCaptionExpr(text)
	if err != nil {
		captionFail("%s", err.Error())
	}
	b, ok := v.(bool)
	return b, ok
}

func captionSubstring(text, startText, lengthText string) string {
	start, _ := strconv.Atoi(strings.TrimSpace(startText))
	length, _ := strconv.Atoi(strings.TrimSpace(lengthText))
	n := len([]rune(text))
	if start < 0 || start+1 >= n {
		captionFail("Object reference not set to an instance of an object.")
	}
	if length < 0 || length-start > n-start {
		length = n - start
	}
	if start+length > n {
		captionFail("Index and length must refer to a location within the string.")
	}
	r := []rune(text)
	return string(r[start : start+length])
}

// captionEscape mirrors EscapeFunction: backslash-escape quote/control
// characters plus the comma, angle bracket and dollar characters that the
// template parser treats specially.
func captionEscape(s string) string {
	var sb strings.Builder
	for _, c := range s {
		switch c {
		case '\'':
			sb.WriteString(`\'`)
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case 0:
			sb.WriteString(`\0`)
		case '\a':
			sb.WriteString(`\a`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\v':
			sb.WriteString(`\v`)
		case ',':
			sb.WriteString(`\,`)
		case '<':
			sb.WriteString(`\<`)
		case '>':
			sb.WriteString(`\>`)
		case '$':
			sb.WriteString(`\$`)
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// captionRegex compiles a pattern case-insensitively (RegexOptions.IgnoreCase).
// Go's RE2 lacks some .NET constructs (lookaround, backreferences); a pattern
// that doesn't compile is reported as a template error.
func captionRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		captionFail("Invalid regular expression: %s", err.Error())
	}
	return re
}

var dotNetGroupRefRe = regexp.MustCompile(`\$(\d+)`)

// dotNetReplacement rewrites .NET replacement syntax ($1) into Go's (${1}),
// since Go would otherwise read "$1x" as a reference to a group named "1x".
func dotNetReplacement(s string) string {
	return dotNetGroupRefRe.ReplaceAllString(s, "$${$1}")
}

func captionValueString(v any) string {
	switch x := v.(type) {
	case bool:
		return dotNetBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return formatCaptionFloat(x)
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func formatCaptionFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func parseCaptionNumber(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil
}

// ---- tiny C#-style expression evaluator --------------------------------
//
// ComicRack evaluates $if<> conditions and $expr<> with DynamicExpresso, a
// C# expression interpreter. This supports the practical subset: int/double/
// string/bool literals, parentheses, unary ! and -, * / % + -, < > <= >=,
// == !=, && ||, and the ?: ternary. Integer division truncates like C#.
// Method calls, member access and the rest of C# are not supported.

type exprParser struct {
	toks []exprTok
	pos  int
}

type exprTok struct {
	kind string // num, str, id, op, end
	text string
}

func evalCaptionExpr(src string) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(captionError); ok {
				v, err = nil, ce
				return
			}
			panic(r)
		}
	}()
	toks := lexCaptionExpr(src)
	p := &exprParser{toks: toks}
	v = p.ternary()
	if p.peek().kind != "end" {
		captionFail("Unexpected token '%s'", p.peek().text)
	}
	return v, nil
}

func lexCaptionExpr(s string) []exprTok {
	var toks []exprTok
	r := []rune(s)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(r) && r[i+1] >= '0' && r[i+1] <= '9'):
			j := i
			for j < len(r) && (r[j] >= '0' && r[j] <= '9' || r[j] == '.') {
				j++
			}
			toks = append(toks, exprTok{"num", string(r[i:j])})
			i = j
		case c == '"':
			var sb strings.Builder
			j := i + 1
			for ; j < len(r) && r[j] != '"'; j++ {
				if r[j] == '\\' && j+1 < len(r) {
					j++
				}
				sb.WriteRune(r[j])
			}
			if j >= len(r) {
				captionFail("Unterminated string literal")
			}
			toks = append(toks, exprTok{"str", sb.String()})
			i = j + 1
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(r) && (r[j] == '_' || r[j] >= 'a' && r[j] <= 'z' || r[j] >= 'A' && r[j] <= 'Z' || r[j] >= '0' && r[j] <= '9') {
				j++
			}
			toks = append(toks, exprTok{"id", string(r[i:j])})
			i = j
		default:
			if i+1 < len(r) {
				two := string(r[i : i+2])
				switch two {
				case "&&", "||", "==", "!=", "<=", ">=":
					toks = append(toks, exprTok{"op", two})
					i += 2
					continue
				}
			}
			if strings.ContainsRune("+-*/%<>!()?:", c) {
				toks = append(toks, exprTok{"op", string(c)})
				i++
				continue
			}
			captionFail("Invalid character '%c' in expression", c)
		}
	}
	return append(toks, exprTok{kind: "end"})
}

func (p *exprParser) peek() exprTok { return p.toks[p.pos] }

func (p *exprParser) acceptOp(ops ...string) (string, bool) {
	t := p.peek()
	if t.kind == "op" {
		for _, o := range ops {
			if t.text == o {
				p.pos++
				return o, true
			}
		}
	}
	return "", false
}

func (p *exprParser) ternary() any {
	cond := p.or()
	if _, ok := p.acceptOp("?"); !ok {
		return cond
	}
	b, isBool := cond.(bool)
	if !isBool {
		captionFail("The ternary condition must be a boolean")
	}
	a := p.ternary()
	if _, ok := p.acceptOp(":"); !ok {
		captionFail("Expected ':' in conditional expression")
	}
	c := p.ternary()
	if b {
		return a
	}
	return c
}

func (p *exprParser) or() any {
	l := p.and()
	for {
		if _, ok := p.acceptOp("||"); !ok {
			return l
		}
		r := p.and()
		l = boolOp(l, r, func(a, b bool) bool { return a || b })
	}
}

func (p *exprParser) and() any {
	l := p.equality()
	for {
		if _, ok := p.acceptOp("&&"); !ok {
			return l
		}
		r := p.equality()
		l = boolOp(l, r, func(a, b bool) bool { return a && b })
	}
}

func boolOp(l, r any, f func(a, b bool) bool) any {
	a, ok1 := l.(bool)
	b, ok2 := r.(bool)
	if !ok1 || !ok2 {
		captionFail("Operator requires boolean operands")
	}
	return f(a, b)
}

func (p *exprParser) equality() any {
	l := p.relational()
	for {
		op, ok := p.acceptOp("==", "!=")
		if !ok {
			return l
		}
		r := p.relational()
		eq := exprEqual(l, r)
		if op == "!=" {
			eq = !eq
		}
		l = eq
	}
}

func exprEqual(l, r any) bool {
	if lf, ok := exprNumber(l); ok {
		if rf, ok := exprNumber(r); ok {
			return lf == rf
		}
		captionFail("Cannot compare a number with a non-number")
	}
	switch a := l.(type) {
	case string:
		b, ok := r.(string)
		if !ok {
			captionFail("Cannot compare a string with a non-string")
		}
		return a == b
	case bool:
		b, ok := r.(bool)
		if !ok {
			captionFail("Cannot compare a boolean with a non-boolean")
		}
		return a == b
	}
	return l == r
}

func exprNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func (p *exprParser) relational() any {
	l := p.additive()
	for {
		op, ok := p.acceptOp("<", ">", "<=", ">=")
		if !ok {
			return l
		}
		r := p.additive()
		a, ok1 := exprNumber(l)
		b, ok2 := exprNumber(r)
		if !ok1 || !ok2 {
			captionFail("Operator '%s' requires numeric operands", op)
		}
		switch op {
		case "<":
			l = a < b
		case ">":
			l = a > b
		case "<=":
			l = a <= b
		default:
			l = a >= b
		}
	}
}

func (p *exprParser) additive() any {
	l := p.multiplicative()
	for {
		op, ok := p.acceptOp("+", "-")
		if !ok {
			return l
		}
		r := p.multiplicative()
		l = exprArith(op, l, r)
	}
}

func (p *exprParser) multiplicative() any {
	l := p.unary()
	for {
		op, ok := p.acceptOp("*", "/", "%")
		if !ok {
			return l
		}
		r := p.unary()
		l = exprArith(op, l, r)
	}
}

func exprArith(op string, l, r any) any {
	if op == "+" {
		_, ls := l.(string)
		_, rs := r.(string)
		if ls || rs {
			return captionValueString(l) + captionValueString(r)
		}
	}
	li, lInt := l.(int64)
	ri, rInt := r.(int64)
	if lInt && rInt {
		switch op {
		case "+":
			return li + ri
		case "-":
			return li - ri
		case "*":
			return li * ri
		case "/":
			if ri == 0 {
				captionFail("Attempted to divide by zero.")
			}
			return li / ri
		case "%":
			if ri == 0 {
				captionFail("Attempted to divide by zero.")
			}
			return li % ri
		}
	}
	a, ok1 := exprNumber(l)
	b, ok2 := exprNumber(r)
	if !ok1 || !ok2 {
		captionFail("Operator '%s' requires numeric operands", op)
	}
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "/":
		return a / b
	default:
		return float64Mod(a, b)
	}
}

func float64Mod(a, b float64) float64 {
	if b == 0 {
		return a
	}
	return a - b*float64(int64(a/b))
}

func (p *exprParser) unary() any {
	if op, ok := p.acceptOp("!", "-"); ok {
		v := p.unary()
		if op == "!" {
			b, isBool := v.(bool)
			if !isBool {
				captionFail("Operator '!' requires a boolean operand")
			}
			return !b
		}
		switch x := v.(type) {
		case int64:
			return -x
		case float64:
			return -x
		}
		captionFail("Operator '-' requires a numeric operand")
	}
	return p.primary()
}

func (p *exprParser) primary() any {
	t := p.peek()
	switch t.kind {
	case "num":
		p.pos++
		if strings.Contains(t.text, ".") {
			f, err := strconv.ParseFloat(t.text, 64)
			if err != nil {
				captionFail("Invalid number '%s'", t.text)
			}
			return f
		}
		n, err := strconv.ParseInt(t.text, 10, 64)
		if err != nil {
			captionFail("Invalid number '%s'", t.text)
		}
		return n
	case "str":
		p.pos++
		return t.text
	case "id":
		p.pos++
		switch strings.ToLower(t.text) {
		case "true":
			return true
		case "false":
			return false
		case "null":
			return nil
		}
		captionFail("Unknown identifier '%s'", t.text)
	case "op":
		if t.text == "(" {
			p.pos++
			v := p.ternary()
			if _, ok := p.acceptOp(")"); !ok {
				captionFail("Expected ')'")
			}
			return v
		}
	}
	if t.kind == "end" {
		captionFail("Unexpected end of expression")
	}
	captionFail("Unexpected token '%s'", t.text)
	return nil
}

// ---- .NET date parsing / formatting ------------------------------------

var dotNetDateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006/01/02",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 15:04:05",
	"1/2/2006",
	"January 2, 2006",
	"Jan 2, 2006",
	"2 January 2006",
	"January 2006",
	"Jan 2006",
}

// parseDotNetDate approximates DateTime.TryParse for the date shapes that show
// up in comic metadata and in the string form of ComicRack's DateTime fields.
func parseDotNetDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range dotNetDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// formatDotNetDate implements DateTime.ToString(format) for the custom
// specifiers comic captions realistically use, plus a few standard formats.
func formatDotNetDate(t time.Time, format string) string {
	if len(format) == 1 {
		switch format {
		case "d":
			return formatDotNetDate(t, "M/d/yyyy")
		case "D":
			return formatDotNetDate(t, "dddd, MMMM d, yyyy")
		case "g":
			return formatDotNetDate(t, "M/d/yyyy h:mm tt")
		case "G":
			return formatDotNetDate(t, "M/d/yyyy h:mm:ss tt")
		case "s":
			return formatDotNetDate(t, "yyyy'-'MM'-'dd'T'HH':'mm':'ss")
		case "t":
			return formatDotNetDate(t, "h:mm tt")
		case "T":
			return formatDotNetDate(t, "h:mm:ss tt")
		case "M", "m":
			return formatDotNetDate(t, "MMMM d")
		case "Y", "y":
			return formatDotNetDate(t, "MMMM yyyy")
		}
	}

	r := []rune(format)
	var sb strings.Builder
	for i := 0; i < len(r); {
		c := r[i]
		run := 1
		for i+run < len(r) && r[i+run] == c {
			run++
		}
		switch c {
		case 'y':
			switch {
			case run == 1:
				sb.WriteString(strconv.Itoa(t.Year() % 100))
			case run == 2:
				sb.WriteString(fmt.Sprintf("%02d", t.Year()%100))
			default:
				sb.WriteString(fmt.Sprintf("%0*d", run, t.Year()))
			}
		case 'M':
			switch {
			case run == 1:
				sb.WriteString(strconv.Itoa(int(t.Month())))
			case run == 2:
				sb.WriteString(fmt.Sprintf("%02d", int(t.Month())))
			case run == 3:
				sb.WriteString(t.Month().String()[:3])
			default:
				sb.WriteString(t.Month().String())
			}
		case 'd':
			switch {
			case run == 1:
				sb.WriteString(strconv.Itoa(t.Day()))
			case run == 2:
				sb.WriteString(fmt.Sprintf("%02d", t.Day()))
			case run == 3:
				sb.WriteString(t.Weekday().String()[:3])
			default:
				sb.WriteString(t.Weekday().String())
			}
		case 'H':
			sb.WriteString(padRun(t.Hour(), run))
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			sb.WriteString(padRun(h, run))
		case 'm':
			sb.WriteString(padRun(t.Minute(), run))
		case 's':
			sb.WriteString(padRun(t.Second(), run))
		case 't':
			ampm := "AM"
			if t.Hour() >= 12 {
				ampm = "PM"
			}
			if run == 1 {
				ampm = ampm[:1]
			}
			sb.WriteString(ampm)
		case '\'', '"':
			j := i + 1
			for j < len(r) && r[j] != c {
				sb.WriteRune(r[j])
				j++
			}
			i = j + 1
			continue
		case '\\':
			if i+1 < len(r) {
				sb.WriteRune(r[i+1])
			}
			i += 2
			continue
		default:
			for k := 0; k < run; k++ {
				sb.WriteRune(c)
			}
		}
		i += run
	}
	return sb.String()
}

func padRun(v, run int) string {
	if run >= 2 {
		return fmt.Sprintf("%02d", v)
	}
	return strconv.Itoa(v)
}

// ---- .NET numeric format strings ---------------------------------------

// formatDotNetNumber implements the practical subset of string.Format
// ("{0:spec}", number): custom specs built from 0 # . , % and literals, plus
// the standard D/F/N/P specifiers. ok=false means "unsupported spec".
func formatDotNetNumber(f float64, spec string) (string, bool) {
	if spec == "" {
		return formatCaptionFloat(f), true
	}

	if len(spec) >= 1 && strings.ContainsRune("DdFfNnPp", rune(spec[0])) {
		prec := -1
		rest := spec[1:]
		if rest != "" {
			n, err := strconv.Atoi(rest)
			if err != nil {
				goto custom
			}
			prec = n
		}
		switch spec[0] {
		case 'D', 'd':
			if f != float64(int64(f)) {
				return "", false
			}
			s := strconv.FormatInt(absInt64(int64(f)), 10)
			if prec > len(s) {
				s = strings.Repeat("0", prec-len(s)) + s
			}
			if f < 0 {
				s = "-" + s
			}
			return s, true
		case 'F', 'f':
			if prec < 0 {
				prec = 2
			}
			return strconv.FormatFloat(f, 'f', prec, 64), true
		case 'N', 'n':
			if prec < 0 {
				prec = 2
			}
			return groupThousands(strconv.FormatFloat(f, 'f', prec, 64)), true
		case 'P', 'p':
			if prec < 0 {
				prec = 2
			}
			return groupThousands(strconv.FormatFloat(f*100, 'f', prec, 64)) + " %", true
		}
	}

custom:
	return formatCustomNumber(f, spec)
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func groupThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var sb strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(c)
	}
	out := sb.String()
	if hasFrac {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// formatCustomNumber handles specs like "0000", "0.00", "#,##0.0", "0%".
// Only the first (positive) section of a ;-separated spec is honored.
func formatCustomNumber(f float64, spec string) (string, bool) {
	if sections := strings.Split(spec, ";"); len(sections) > 1 {
		spec = sections[0]
	}

	var prefix, suffix strings.Builder
	var numPart strings.Builder
	seenNum := false
	doneNum := false
	percent := 0
	r := []rune(spec)
	for i := 0; i < len(r); i++ {
		c := r[i]
		isNum := c == '0' || c == '#' || c == '.' || c == ','
		switch {
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(r) && r[j] != c {
				if seenNum {
					suffix.WriteRune(r[j])
				} else {
					prefix.WriteRune(r[j])
				}
				j++
			}
			i = j
			if seenNum {
				doneNum = true
			}
		case c == '%':
			percent++
			if seenNum {
				suffix.WriteRune(c)
				doneNum = true
			} else {
				prefix.WriteRune(c)
			}
		case isNum && !doneNum:
			seenNum = true
			numPart.WriteRune(c)
		default:
			if seenNum {
				suffix.WriteRune(c)
				doneNum = true
			} else {
				prefix.WriteRune(c)
			}
		}
	}
	if !seenNum {
		return "", false
	}

	for i := 0; i < percent; i++ {
		f *= 100
	}

	num := numPart.String()
	intSpec, fracSpec, _ := strings.Cut(num, ".")
	group := strings.Contains(intSpec, ",")
	intSpec = strings.ReplaceAll(intSpec, ",", "")
	minInt := strings.Count(intSpec, "0")
	minFrac := strings.Count(fracSpec, "0")
	maxFrac := len(fracSpec)

	s := strconv.FormatFloat(absFloat(f), 'f', maxFrac, 64)
	ip, fp, _ := strings.Cut(s, ".")
	for len(fp) > minFrac && strings.HasSuffix(fp, "0") {
		fp = fp[:len(fp)-1]
	}
	ip = strings.TrimLeft(ip, "0")
	if len(ip) < minInt {
		ip = strings.Repeat("0", minInt-len(ip)) + ip
	}
	if group {
		ip = strings.TrimPrefix(groupThousands(ip), "-")
	}

	out := ip
	if fp != "" {
		out += "." + fp
	}
	if out == "" {
		out = "0"
	}
	if f < 0 && strings.Trim(out, "0.,") != "" {
		out = "-" + out
	}
	return prefix.String() + out + suffix.String(), true
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
