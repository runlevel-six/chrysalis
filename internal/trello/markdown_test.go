// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package trello

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mdJSON(t *testing.T, src string) string {
	t.Helper()
	doc := Markdown(src)
	if doc == nil {
		return "null"
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc.Content); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(b.String())
}

func TestMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "  \n\n ", `null`},
		{"paragraphs", "First one.\n\nSecond one.",
			`[{"type":"paragraph","content":[{"type":"text","text":"First one."}]},{"type":"paragraph","content":[{"type":"text","text":"Second one."}]}]`},
		{"line breaks become hard breaks", "Line one  \nline two\\\nline three",
			`[{"type":"paragraph","content":[{"type":"text","text":"Line one"},{"type":"hardBreak"},{"type":"text","text":"line two"},{"type":"hardBreak"},{"type":"text","text":"line three"}]}]`},
		{"headings map to levels 2 and 3", "# Big\n## Medium\n### Small ###\n###### Tiny",
			`[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Big"}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Medium"}]},{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Small"}]},{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Tiny"}]}]`},
		{"setext headings", "Title\n=====\n\nSub\n---",
			`[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Sub"}]}]`},
		{"hashtag without space is text", "#notaheading",
			`[{"type":"paragraph","content":[{"type":"text","text":"#notaheading"}]}]`},
		{"bold and italic", "**bold** and *it* and __b2__ and _i2_",
			`[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"bold"}]},{"type":"text","text":" and "},{"type":"text","text":"it","marks":[{"type":"italic"}]},{"type":"text","text":" and "},{"type":"text","text":"b2","marks":[{"type":"bold"}]},{"type":"text","text":" and "},{"type":"text","text":"i2","marks":[{"type":"italic"}]}]}]`},
		{"bold and italic together", "***both***",
			`[{"type":"paragraph","content":[{"type":"text","text":"both","marks":[{"type":"bold"},{"type":"italic"}]}]}]`},
		{"nested emphasis", "*a **b** c*",
			`[{"type":"paragraph","content":[{"type":"text","text":"a ","marks":[{"type":"italic"}]},{"type":"text","text":"b","marks":[{"type":"italic"},{"type":"bold"}]},{"type":"text","text":" c","marks":[{"type":"italic"}]}]}]`},
		{"strikethrough", "~~gone~~ ~kept~",
			`[{"type":"paragraph","content":[{"type":"text","text":"gone","marks":[{"type":"strike"}]},{"type":"text","text":" ~kept~"}]}]`},
		{"snake_case is not emphasis", "water_the_beans and 2*3*4",
			`[{"type":"paragraph","content":[{"type":"text","text":"water_the_beans and 2"},{"type":"text","text":"3","marks":[{"type":"italic"}]},{"type":"text","text":"4"}]}]`},
		{"unclosed delimiters stay as text", "**open and * alone and __x",
			`[{"type":"paragraph","content":[{"type":"text","text":"**open and * alone and __x"}]}]`},
		{"inline code excludes other marks", "**run `make *all*` now**",
			`[{"type":"paragraph","content":[{"type":"text","text":"run ","marks":[{"type":"bold"}]},{"type":"text","text":"make *all*","marks":[{"type":"code"}]},{"type":"text","text":" now","marks":[{"type":"bold"}]}]}]`},
		{"double backtick code", "``a ` b``",
			`[{"type":"paragraph","content":[{"type":"text","text":"a ` + "`" + ` b","marks":[{"type":"code"}]}]}]`},
		{"escapes", `\*not italic\* and \# and a\\b`,
			`[{"type":"paragraph","content":[{"type":"text","text":"*not italic* and # and a\\b"}]}]`},
		{"links", `See [the **seed** list](https://example.com/seeds "Seeds") today.`,
			`[{"type":"paragraph","content":[{"type":"text","text":"See "},{"type":"text","text":"the ","marks":[{"type":"link","attrs":{"href":"https://example.com/seeds"}}]},{"type":"text","text":"seed","marks":[{"type":"link","attrs":{"href":"https://example.com/seeds"}},{"type":"bold"}]},{"type":"text","text":" list","marks":[{"type":"link","attrs":{"href":"https://example.com/seeds"}}]},{"type":"text","text":" today."}]}]`},
		{"link with parens in url", `[wiki](https://example.com/a_(b))`,
			`[{"type":"paragraph","content":[{"type":"text","text":"wiki","marks":[{"type":"link","attrs":{"href":"https://example.com/a_(b)"}}]}]}]`},
		{"unsafe links become text", `[click](javascript:alert(1)) and <javascript:x>`,
			`[{"type":"paragraph","content":[{"type":"text","text":"click and <javascript:x>"}]}]`},
		{"autolinks and bare urls", "Go to <https://example.com/a> or https://example.com/b.",
			`[{"type":"paragraph","content":[{"type":"text","text":"Go to "},{"type":"text","text":"https://example.com/a","marks":[{"type":"link","attrs":{"href":"https://example.com/a"}}]},{"type":"text","text":" or "},{"type":"text","text":"https://example.com/b","marks":[{"type":"link","attrs":{"href":"https://example.com/b"}}]},{"type":"text","text":"."}]}]`},
		{"mail link", "[write](mailto:alice@example.com)",
			`[{"type":"paragraph","content":[{"type":"text","text":"write","marks":[{"type":"link","attrs":{"href":"mailto:alice@example.com"}}]}]}]`},
		{"images become links", "![a tomato](https://example.com/t.png) ![](https://example.com/u.png)",
			`[{"type":"paragraph","content":[{"type":"text","text":"a tomato","marks":[{"type":"link","attrs":{"href":"https://example.com/t.png"}}]},{"type":"text","text":" "},{"type":"text","text":"https://example.com/u.png","marks":[{"type":"link","attrs":{"href":"https://example.com/u.png"}}]}]}]`},
		{"brackets that aren't links", "[draft] notes [x](",
			`[{"type":"paragraph","content":[{"type":"text","text":"[draft] notes [x]("}]}]`},
		{"fenced code", "```go\nfunc main() {\n    **not bold**\n}\n```\nafter",
			`[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"func main() {\n    **not bold**\n}"}]},{"type":"paragraph","content":[{"type":"text","text":"after"}]}]`},
		{"tilde fence, unclosed", "~~~\nline\n",
			`[{"type":"codeBlock","attrs":{"language":null},"content":[{"type":"text","text":"line\n"}]}]`},
		{"indented code", "Text:\n\n    indented\n      more\n\nback",
			`[{"type":"paragraph","content":[{"type":"text","text":"Text:"}]},{"type":"codeBlock","attrs":{"language":null},"content":[{"type":"text","text":"indented\n  more"}]},{"type":"paragraph","content":[{"type":"text","text":"back"}]}]`},
		{"bullet list", "- one\n* two\n+ three",
			`[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"three"}]}]}]}]`},
		{"ordered list keeps its start", "3. three\n4) four",
			`[{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"three"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"four"}]}]}]}]`},
		{"nested lists", "- fruit\n  - apples\n  - pears\n- veg\n  1. beans",
			`[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"fruit"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"apples"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"pears"}]}]}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"veg"}]},{"type":"orderedList","attrs":{"start":1},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"beans"}]}]}]}]}]}]`},
		{"loose list and lazy continuation", "- a\nstill a\n\n- b\n\n  more b\n\nafter",
			`[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"still a"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]},{"type":"paragraph","content":[{"type":"text","text":"more b"}]}]}]},{"type":"paragraph","content":[{"type":"text","text":"after"}]}]`},
		{"list ends a paragraph; empty item", "Shopping:\n- eggs\n- \n- milk",
			`[{"type":"paragraph","content":[{"type":"text","text":"Shopping:"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"eggs"}]}]},{"type":"listItem","content":[{"type":"paragraph"}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"milk"}]}]}]}]`},
		{"list item starting with code", "- ```\n  x\n  ```",
			`[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph"},{"type":"codeBlock","attrs":{"language":null},"content":[{"type":"text","text":"x"}]}]}]}]`},
		{"blockquote", "> quoted *text*\n> # head\n>\n> > deeper",
			`[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted "},{"type":"text","text":"text","marks":[{"type":"italic"}]}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"head"}]},{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"deeper"}]}]}]}]`},
		{"horizontal rule", "above\n\n***\n\n- - -\nbelow",
			`[{"type":"paragraph","content":[{"type":"text","text":"above"}]},{"type":"horizontalRule"},{"type":"horizontalRule"},{"type":"paragraph","content":[{"type":"text","text":"below"}]}]`},
		{"CRLF and tabs", "a\r\nb\r\n\r\n-\tc",
			`[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"c"}]}]}]}]`},
		{"unicode", "**Größe** 🌱 _ok_",
			`[{"type":"paragraph","content":[{"type":"text","text":"Größe","marks":[{"type":"bold"}]},{"type":"text","text":" 🌱 "},{"type":"text","text":"ok","marks":[{"type":"italic"}]}]}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mdJSON(t, tc.in); got != tc.want {
				t.Errorf("Markdown(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

// Hostile input must stay fast and never panic.
func TestMarkdownPathological(t *testing.T) {
	inputs := []string{
		strings.Repeat("**a [", 8000),
		strings.Repeat("*", 60000),
		strings.Repeat("_a ", 20000),
		strings.Repeat("[", 30000) + strings.Repeat("]", 30000),
		strings.Repeat("[x](", 15000),
		strings.Repeat("`", 30000) + "x" + strings.Repeat("``", 100),
		strings.Repeat("> ", 5000) + "deep",
		strings.Repeat("- ", 5000) + "deep",
		strings.Repeat("  - a\n", 5000),
		strings.Repeat("~~a ", 20000),
		strings.Repeat("<", 30000),
		strings.Repeat("https://", 8000),
		"\xff\xfe broken utf8 **\xff**",
	}
	for i, in := range inputs {
		start := time.Now()
		doc := Markdown(in)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("input %d took %v", i, d)
		}
		if _, err := json.Marshal(doc); err != nil {
			t.Errorf("input %d: %v", i, err)
		}
	}
}

func TestMarkdownTruncatesHugeInput(t *testing.T) {
	doc := Markdown(strings.Repeat("é", maxMarkdownBytes))
	if doc == nil {
		t.Fatal("nil doc")
	}
	text := doc.Content[0].Content[0].Text
	if len(text) > maxMarkdownBytes || !strings.HasSuffix(text, "é") {
		t.Errorf("truncated to %d bytes, suffix ok=%v", len(text), strings.HasSuffix(text, "é"))
	}
}
