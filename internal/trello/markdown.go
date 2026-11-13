// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package trello

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Node is a ProseMirror JSON node. The node and mark names match the card
// description editor (web/src/board/Description.tsx): TipTap's StarterKit
// (with headings limited to levels 2 and 3, and its link mark) plus images.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Limits that keep a hostile description from costing much: Trello itself
// caps descriptions at 16,384 characters.
const (
	maxMarkdownBytes = 64 << 10
	maxBlockDepth    = 12
	maxLinkScan      = 4096
)

// Markdown converts Trello's Markdown to a ProseMirror document. It covers
// what Trello's editor writes: paragraphs, headings, bold, italic,
// strikethrough, inline code, code blocks, bullet and numbered lists,
// links, blockquotes, horizontal rules and line breaks. Anything else stays
// as plain text, so nothing is lost. Images become links: descriptions only
// show this app's own attachments, never pictures from elsewhere. It returns
// nil for an empty description.
func Markdown(src string) *Node {
	if len(src) > maxMarkdownBytes {
		src = truncateBytes(src, maxMarkdownBytes)
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = strings.ReplaceAll(src, "\x00", "�")
	blocks := parseBlocks(strings.Split(src, "\n"), 0)
	if len(blocks) == 0 {
		return nil
	}
	return &Node{Type: "doc", Content: blocks}
}

// truncateBytes cuts s to at most n bytes without splitting a character.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	thematic   = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	fenceOpen  = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})[ \t]*([^`\\s]*)")
	listItem   = regexp.MustCompile(`^( {0,3})([-*+]|(\d{1,9})[.)])([ \t]+|$)`)
	setext     = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
	quoteLine  = regexp.MustCompile(`^ {0,3}> ?`)
)

func blank(line string) bool { return strings.TrimSpace(line) == "" }

// indentOf counts leading columns, with tabs to the next multiple of 4.
func indentOf(line string) int {
	n := 0
	for _, r := range line {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4 - n%4
		default:
			return n
		}
	}
	return n
}

// stripIndent removes up to n columns of leading whitespace.
func stripIndent(line string, n int) string {
	col := 0
	for i, r := range line {
		if col >= n {
			return line[i:]
		}
		switch r {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
			if col > n {
				return strings.Repeat(" ", col-n) + line[i+1:]
			}
		default:
			return line[i:]
		}
	}
	return ""
}

// startsBlock reports whether line begins something other than paragraph
// text, which ends a paragraph in progress.
func startsBlock(line string) bool {
	return atxHeading.MatchString(line) || thematic.MatchString(line) || fenceOpen.MatchString(line) ||
		quoteLine.MatchString(line) || (listItem.MatchString(line) && !blank(listItem.ReplaceAllString(line, "")))
}

func parseBlocks(lines []string, depth int) []Node {
	var out []Node
	for i := 0; i < len(lines); {
		line := lines[i]
		switch {
		case blank(line):
			i++

		case depth > maxBlockDepth:
			// Too deeply nested to be real: keep the rest as plain text.
			var text []string
			for ; i < len(lines); i++ {
				text = append(text, strings.TrimSpace(lines[i]))
			}
			out = append(out, paragraph(strings.Join(text, "\n")))

		case fenceOpen.MatchString(line):
			m := fenceOpen.FindStringSubmatch(line)
			indent, fence, lang := len(m[1]), m[2], m[3]
			i++
			var code []string
			for ; i < len(lines); i++ {
				t := strings.TrimSpace(lines[i])
				if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" && indentOf(lines[i]) < 4 {
					i++
					break
				}
				code = append(code, stripIndent(lines[i], indent))
			}
			out = append(out, codeBlock(strings.Join(code, "\n"), lang))

		case indentOf(line) >= 4:
			var code []string
			for ; i < len(lines) && (blank(lines[i]) || indentOf(lines[i]) >= 4); i++ {
				code = append(code, stripIndent(lines[i], 4))
			}
			for len(code) > 0 && blank(code[len(code)-1]) {
				code = code[:len(code)-1]
			}
			out = append(out, codeBlock(strings.Join(code, "\n"), ""))

		case atxHeading.MatchString(line):
			m := atxHeading.FindStringSubmatch(line)
			out = append(out, heading(len(m[1]), strings.TrimSpace(m[2])))
			i++

		case thematic.MatchString(line):
			out = append(out, Node{Type: "horizontalRule"})
			i++

		case quoteLine.MatchString(line):
			var inner []string
			for ; i < len(lines) && quoteLine.MatchString(lines[i]); i++ {
				inner = append(inner, quoteLine.ReplaceAllString(lines[i], ""))
			}
			content := parseBlocks(inner, depth+1)
			if len(content) == 0 {
				content = []Node{{Type: "paragraph"}}
			}
			out = append(out, Node{Type: "blockquote", Content: content})

		case listItem.MatchString(line):
			var n Node
			n, i = parseList(lines, i, depth)
			out = append(out, n)

		default:
			var text []string
			level := 0
			for ; i < len(lines) && !blank(lines[i]); i++ {
				if len(text) > 0 {
					if m := setext.FindStringSubmatch(lines[i]); m != nil {
						level = 1
						if m[1][0] == '-' {
							level = 2
						}
						i++
						break
					}
					if startsBlock(lines[i]) {
						break
					}
				}
				text = append(text, strings.TrimSpace(lines[i]))
			}
			joined := strings.Join(text, "\n")
			if level > 0 {
				out = append(out, heading(level, strings.ReplaceAll(joined, "\n", " ")))
			} else {
				out = append(out, paragraph(joined))
			}
		}
	}
	return out
}

func parseList(lines []string, i, depth int) (Node, int) {
	m := listItem.FindStringSubmatch(lines[i])
	ordered := m[3] != ""
	list := Node{Type: "bulletList"}
	if ordered {
		list.Type = "orderedList"
		start, _ := strconv.Atoi(m[3])
		list.Attrs = map[string]any{"start": start}
	}
	for i < len(lines) {
		m := listItem.FindStringSubmatch(lines[i])
		if m == nil || (m[3] != "") != ordered {
			break
		}
		// Content lines line up with the text after the marker.
		marker := len(m[1]) + len(m[2])
		rest := lines[i][marker:]
		spaces := indentOf(rest)
		width := marker + 1
		var first string
		switch {
		case blank(rest):
		case spaces <= 4:
			width = marker + spaces
			first = stripIndent(rest, spaces)
		default: // indented code inside the item
			first = stripIndent(rest, 1)
		}
		item := []string{first}
		i++
		for i < len(lines) {
			line := lines[i]
			switch {
			case blank(line):
				// A blank line continues the item only if indented content follows.
				j := i
				for j < len(lines) && blank(lines[j]) {
					j++
				}
				if j < len(lines) && indentOf(lines[j]) >= width {
					for ; i < j; i++ {
						item = append(item, "")
					}
					continue
				}
				i = j
				goto itemDone
			case indentOf(line) >= width:
				item = append(item, stripIndent(line, width))
				i++
			case listItem.MatchString(line) || startsBlock(line):
				goto itemDone
			default:
				// A lazy continuation of the item's paragraph.
				item = append(item, strings.TrimSpace(line))
				i++
			}
		}
	itemDone:
		content := parseBlocks(item, depth+1)
		if len(content) == 0 || content[0].Type != "paragraph" {
			// The editor's list items always start with a paragraph.
			content = append([]Node{{Type: "paragraph"}}, content...)
		}
		list.Content = append(list.Content, Node{Type: "listItem", Content: content})
	}
	return list, i
}

func codeBlock(text, lang string) Node {
	n := Node{Type: "codeBlock", Attrs: map[string]any{"language": nil}}
	if lang != "" {
		n.Attrs["language"] = lang
	}
	if text != "" {
		n.Content = []Node{{Type: "text", Text: text}}
	}
	return n
}

// heading maps Markdown's six levels onto the editor's two: # and ## become
// level 2, anything smaller level 3.
func heading(level int, text string) Node {
	l := 2
	if level > 2 {
		l = 3
	}
	return Node{Type: "heading", Attrs: map[string]any{"level": l}, Content: inline(text)}
}

func paragraph(text string) Node {
	return Node{Type: "paragraph", Content: inline(text)}
}

// inline parses a paragraph's text into text nodes with marks and hard
// breaks. Every line break in Trello's Markdown shows as a line break, so
// each newline becomes a hardBreak.
func inline(text string) []Node {
	if text == "" {
		return nil
	}
	p := &inlineParser{src: text, failed: map[string]span{}}
	p.matchBrackets()
	p.parse(0, len(text), nil)
	return p.out
}

type span struct{ from, to int }

type inlineParser struct {
	src string
	out []Node
	// failed remembers, per delimiter, a range already searched without
	// finding a closer, so pathological input stays linear-ish.
	failed  map[string]span
	closeOf map[int]int // '[' index -> matching ']' index
}

// matchBrackets pairs square brackets once, up front.
func (p *inlineParser) matchBrackets() {
	p.closeOf = map[int]int{}
	var stack []int
	for i := 0; i < len(p.src); i++ {
		switch p.src[i] {
		case '\\':
			i++
		case '[':
			stack = append(stack, i)
		case ']':
			if len(stack) > 0 {
				p.closeOf[stack[len(stack)-1]] = i
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func hasMark(marks []Mark, t string) bool {
	for _, m := range marks {
		if m.Type == t {
			return true
		}
	}
	return false
}

func with(marks []Mark, m Mark) []Mark {
	out := make([]Mark, 0, len(marks)+1)
	out = append(out, marks...)
	return append(out, m)
}

func (p *inlineParser) text(s string, marks []Mark) {
	if s == "" {
		return
	}
	if n := len(p.out); n > 0 && p.out[n-1].Type == "text" && sameMarks(p.out[n-1].Marks, marks) {
		p.out[n-1].Text += s
		return
	}
	p.out = append(p.out, Node{Type: "text", Text: s, Marks: marks})
}

func sameMarks(a, b []Mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || (a[i].Type == "link" && a[i].Attrs["href"] != b[i].Attrs["href"]) {
			return false
		}
	}
	return true
}

const punct = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }

func isAlnumBefore(s string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isAlnumAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// parse handles src[from:to] with the marks already in effect.
func (p *inlineParser) parse(from, to int, marks []Mark) {
	s := p.src
	plain := from // start of pending plain text
	flush := func(i int) {
		p.text(s[plain:i], marks)
	}
	for i := from; i < to; {
		c := s[i]
		switch {
		case c == '\\' && i+1 < to && strings.IndexByte(punct, s[i+1]) >= 0:
			flush(i)
			p.text(s[i+1:i+2], marks)
			i += 2
			plain = i
			continue

		case c == '\\' && i+1 < to && s[i+1] == '\n':
			flush(i)
			p.out = append(p.out, Node{Type: "hardBreak"})
			i += 2
			plain = i
			continue

		case c == '\n':
			end := i
			for end > plain && (s[end-1] == ' ' || s[end-1] == '\t') {
				end--
			}
			flush(end)
			p.out = append(p.out, Node{Type: "hardBreak"})
			i++
			plain = i
			continue

		case c == '`':
			n := runLen(s, i, to, '`')
			if j := p.findRun(i+n, to, '`', n); j >= 0 {
				flush(i)
				code := s[i+n : j]
				if len(code) > 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				code = strings.ReplaceAll(code, "\n", " ")
				// The editor's code mark excludes every other mark.
				p.text(code, []Mark{{Type: "code"}})
				i = j + n
				plain = i
				continue
			}
			i += n
			continue

		case c == '!' && i+1 < to && s[i+1] == '[':
			if end, href, ok := p.link(i+1, to); ok {
				flush(i)
				alt := s[i+2 : p.closeOf[i+1]]
				if alt == "" {
					alt = href
				}
				p.linkText(alt, href, marks)
				i = end
				plain = i
				continue
			}

		case c == '[':
			if end, href, ok := p.link(i, to); ok {
				flush(i)
				close := p.closeOf[i]
				if safeHref(href) && !hasMark(marks, "link") {
					p.parse(i+1, close, with(marks, Mark{Type: "link", Attrs: map[string]any{"href": href}}))
				} else {
					p.parse(i+1, close, marks)
				}
				i = end
				plain = i
				continue
			}

		case c == '<':
			if j := strings.IndexByte(s[i:min(to, i+maxLinkScan)], '>'); j > 0 {
				href := s[i+1 : i+j]
				if !strings.ContainsAny(href, " \t\n<") && safeHref(href) && strings.Contains(href, ":") {
					flush(i)
					p.linkText(href, href, marks)
					i += j + 1
					plain = i
					continue
				}
			}

		case (c == 'h' || c == 'H') && (i == from || isSpace(s[i-1]) || s[i-1] == '(') && !hasMark(marks, "link"):
			if l := bareURL(s[i:to]); l != "" {
				flush(i)
				p.linkText(l, l, marks)
				i += len(l)
				plain = i
				continue
			}

		case c == '*' || c == '_' || c == '~':
			if n, ok := p.emphasis(i, to, marks, flush); ok {
				i = n
				plain = i
				continue
			}
			i += runLen(s, i, to, c)
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	flush(to)
}

func runLen(s string, i, to int, c byte) int {
	n := 0
	for i+n < to && s[i+n] == c {
		n++
	}
	return n
}

// findRun finds a run of exactly n copies of c in src[from:to].
func (p *inlineParser) findRun(from, to int, c byte, n int) int {
	key := strings.Repeat(string(c), n) + "run"
	if f, ok := p.failed[key]; ok && from >= f.from && to <= f.to {
		return -1
	}
	for i := from; i < to; {
		if p.src[i] != c {
			i++
			continue
		}
		l := runLen(p.src, i, to, c)
		if l == n {
			return i
		}
		i += l
	}
	p.failed[key] = span{from, to}
	return -1
}

// emphasis handles **bold**, __bold__, *italic*, _italic_ and ~~strike~~
// at s[i]. It returns where parsing continues.
func (p *inlineParser) emphasis(i, to int, marks []Mark, flush func(int)) (int, bool) {
	s := p.src
	c := s[i]
	run := runLen(s, i, to, c)
	if c == '~' && (run != 2 || hasMark(marks, "strike")) {
		return 0, false
	}
	if c == '_' && isAlnumBefore(s, i) {
		return 0, false // snake_case words aren't emphasis
	}
	bold, italic := hasMark(marks, "bold"), hasMark(marks, "italic")
	if c != '~' && run >= 3 && !bold && !italic && i+3 < to && !isSpace(s[i+3]) {
		// ***both***
		if j := p.findCloser(i+3, to, c, 3); j >= 0 {
			flush(i)
			p.parse(i+3, j, with(with(marks, Mark{Type: "bold"}), Mark{Type: "italic"}))
			return j + 3, true
		}
	}
	var mark string
	var n int
	switch {
	case c == '~':
		mark, n = "strike", 2
	case run >= 2 && !bold:
		mark, n = "bold", 2
	case !italic:
		mark, n = "italic", 1
	default:
		return 0, false
	}
	open := i + run - n // the delimiter nearest the text opens
	if open+n >= to || isSpace(s[open+n]) {
		return 0, false
	}
	closeAt := p.findCloser(open+n, to, c, n)
	if closeAt < 0 {
		return 0, false
	}
	flush(i)
	if open > i {
		p.text(s[i:open], marks)
	}
	p.parse(open+n, closeAt, with(marks, Mark{Type: mark}))
	return closeAt + n, true
}

// findCloser finds a closing delimiter of n copies of c in src[from:to]: not
// after whitespace, and for _ not followed by a letter. Within a longer run,
// the closer is the run's first n characters.
func (p *inlineParser) findCloser(from, to int, c byte, n int) int {
	key := string(c) + strconv.Itoa(n)
	if f, ok := p.failed[key]; ok && from >= f.from && to <= f.to {
		return -1
	}
	s := p.src
	for i := from; i < to; {
		switch s[i] {
		case '\\':
			i += 2
			continue
		case '`':
			// Delimiters inside a code span don't count.
			l := runLen(s, i, to, '`')
			if j := p.findRun(i+l, to, '`', l); j >= 0 {
				i = j + l
			} else {
				i += l
			}
			continue
		case c:
		default:
			i++
			continue
		}
		l := runLen(s, i, to, c)
		ok := i > from && !isSpace(s[i-1]) && (c != '_' || !isAlnumAt(s, i+l))
		// A single * closes on * or *** (closing bold too), never on **.
		if ok && ((n == 1 && (l == 1 || l == 3)) || (n >= 2 && l >= n)) {
			return i
		}
		i += l
	}
	p.failed[key] = span{from, to}
	return -1
}

// link parses [text](url "title") with '[' at i. It returns where parsing
// continues and the URL.
func (p *inlineParser) link(i, to int) (int, string, bool) {
	s := p.src
	close, ok := p.closeOf[i]
	if !ok || close >= to || close+1 >= to || s[close+1] != '(' {
		return 0, "", false
	}
	j := close + 2
	limit := min(to, j+maxLinkScan)
	for j < limit && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	var href string
	if j < limit && s[j] == '<' {
		k := strings.IndexByte(s[j:limit], '>')
		if k < 0 {
			return 0, "", false
		}
		href = s[j+1 : j+k]
		j += k + 1
	} else {
		depth, start := 0, j
		for ; j < limit; j++ {
			if s[j] == '(' {
				depth++
			} else if s[j] == ')' {
				if depth == 0 {
					break
				}
				depth--
			} else if isSpace(s[j]) {
				break
			}
		}
		href = s[start:j]
	}
	for j < limit && isSpace(s[j]) {
		j++
	}
	if j < limit && (s[j] == '"' || s[j] == '\'') {
		q := s[j]
		k := strings.IndexByte(s[j+1:limit], q)
		if k < 0 {
			return 0, "", false
		}
		j += k + 2
		for j < limit && isSpace(s[j]) {
			j++
		}
	}
	if j >= limit || s[j] != ')' {
		return 0, "", false
	}
	return j + 1, href, true
}

// linkText adds text carrying a link mark, or as plain text when the address
// isn't a kind the editor allows.
func (p *inlineParser) linkText(text, href string, marks []Mark) {
	if safeHref(href) && !hasMark(marks, "link") {
		marks = with(marks, Mark{Type: "link", Attrs: map[string]any{"href": href}})
	}
	p.text(text, marks)
}

// safeHref matches the editor's link rule: only web and mail links.
func safeHref(href string) bool {
	h := strings.ToLower(href)
	return (strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") || strings.HasPrefix(h, "mailto:")) &&
		len(href) <= 2048 && !strings.ContainsAny(href, " \t\n")
}

// bareURL returns the web address at the start of s, if any, without
// trailing punctuation.
func bareURL(s string) string {
	l := strings.ToLower(s[:min(len(s), 8)])
	if !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
		return ""
	}
	end := strings.IndexAny(s, " \t\n<")
	if end < 0 {
		end = len(s)
	}
	end = min(end, 2048)
	u := s[:end]
	for len(u) > 0 {
		last := u[len(u)-1]
		if strings.IndexByte(".,;:!?'\"*_~", last) >= 0 {
			u = u[:len(u)-1]
			continue
		}
		if last == ')' && strings.Count(u, "(") < strings.Count(u, ")") {
			u = u[:len(u)-1]
			continue
		}
		break
	}
	if len(u) <= len("https://") || !strings.Contains(u[strings.Index(u, "//")+2:], ".") && !strings.Contains(u, "localhost") {
		return ""
	}
	return u
}
