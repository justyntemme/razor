package ui

import (
	"strings"

	"github.com/justyntemme/organelle/ast"
	"github.com/justyntemme/organelle/lexer"
	"github.com/justyntemme/organelle/parser"
)

// ParseOrgMode parses org-mode content and returns blocks for rendering
// using the same MarkdownBlock/MarkdownSpan types for unified rendering
func ParseOrgMode(content string) []MarkdownBlock {
	l := lexer.New(content)
	p := parser.New(l)
	doc := p.ParseDocument()

	if len(p.Errors()) > 0 {
		// Return error as a paragraph block
		return []MarkdownBlock{{
			Type:  "paragraph",
			Spans: []MarkdownSpan{{Text: "Parse error: " + strings.Join(p.Errors(), "; ")}},
		}}
	}

	var blocks []MarkdownBlock
	for _, node := range doc.Children {
		blocks = append(blocks, convertOrgNode(node)...)
	}
	return blocks
}

// convertOrgNode converts an org-mode AST node to MarkdownBlocks
func convertOrgNode(node ast.Node) []MarkdownBlock {
	var blocks []MarkdownBlock

	switch n := node.(type) {
	case *ast.Headline:
		blocks = append(blocks, convertHeadline(n)...)

	case *ast.Paragraph:
		blocks = append(blocks, convertParagraph(n))

	case *ast.Block:
		blocks = append(blocks, convertBlock(n))

	case *ast.List:
		blocks = append(blocks, convertList(n)...)

	case *ast.Keyword:
		blocks = append(blocks, convertKeyword(n))

	case *ast.HorizontalRule:
		blocks = append(blocks, MarkdownBlock{Type: "hr"})

	case *ast.Comment:
		// Skip comments in rendered view

	case *ast.Drawer:
		// Skip drawers in rendered view (properties, etc.)

	case *ast.Table:
		blocks = append(blocks, convertTable(n))
	}

	return blocks
}

// convertHeadline converts an org headline to blocks
func convertHeadline(h *ast.Headline) []MarkdownBlock {
	var blocks []MarkdownBlock

	// Build headline text with optional keyword and priority
	var titleParts []MarkdownSpan

	if h.Keyword != "" {
		// Style TODO/DONE keywords
		titleParts = append(titleParts, MarkdownSpan{
			Text: h.Keyword + " ",
			Bold: true,
		})
	}

	if h.Priority != "" {
		titleParts = append(titleParts, MarkdownSpan{
			Text:   "[#" + h.Priority + "] ",
			Italic: true,
		})
	}

	titleParts = append(titleParts, MarkdownSpan{Text: h.Title})

	if len(h.Tags) > 0 {
		titleParts = append(titleParts, MarkdownSpan{
			Text:   " :" + strings.Join(h.Tags, ":") + ":",
			Italic: true,
			Code:   true,
		})
	}

	blocks = append(blocks, MarkdownBlock{
		Type:  "heading",
		Level: h.Level,
		Spans: titleParts,
	})

	// Process children (nested headlines and content)
	for _, child := range h.Children {
		blocks = append(blocks, convertOrgNode(child)...)
	}

	return blocks
}

// convertParagraph converts an org paragraph to a block
func convertParagraph(p *ast.Paragraph) MarkdownBlock {
	var spans []MarkdownSpan

	if len(p.Inline) > 0 {
		// Use parsed inline elements
		spans = convertInlineElements(p.Inline)
	} else {
		// Fallback to raw content
		spans = []MarkdownSpan{{Text: p.Content}}
	}

	return MarkdownBlock{
		Type:  "paragraph",
		Spans: spans,
	}
}

// convertInlineElements converts org inline elements to spans
func convertInlineElements(elements []ast.InlineElement) []MarkdownSpan {
	var spans []MarkdownSpan

	for _, elem := range elements {
		spans = append(spans, convertInlineElement(elem, false, false)...)
	}

	return spans
}

// convertInlineElement converts a single inline element to spans
func convertInlineElement(elem ast.InlineElement, parentBold, parentItalic bool) []MarkdownSpan {
	var spans []MarkdownSpan

	switch elem.Type {
	case ast.InlineText:
		if elem.Content != "" {
			spans = append(spans, MarkdownSpan{
				Text:   elem.Content,
				Bold:   parentBold,
				Italic: parentItalic,
			})
		}

	case ast.InlineBold:
		if len(elem.Children) > 0 {
			for _, child := range elem.Children {
				spans = append(spans, convertInlineElement(child, true, parentItalic)...)
			}
		} else {
			spans = append(spans, MarkdownSpan{
				Text: elem.Content,
				Bold: true,
			})
		}

	case ast.InlineItalic:
		if len(elem.Children) > 0 {
			for _, child := range elem.Children {
				spans = append(spans, convertInlineElement(child, parentBold, true)...)
			}
		} else {
			spans = append(spans, MarkdownSpan{
				Text:   elem.Content,
				Italic: true,
			})
		}

	case ast.InlineCode, ast.InlineVerbatim:
		spans = append(spans, MarkdownSpan{
			Text: elem.Content,
			Code: true,
		})

	case ast.InlineStrikethrough:
		// No strikethrough in MarkdownSpan, show as-is with markers
		spans = append(spans, MarkdownSpan{
			Text: "~" + elem.Content + "~",
		})

	case ast.InlineUnderline:
		// No underline in MarkdownSpan, treat as italic
		if len(elem.Children) > 0 {
			for _, child := range elem.Children {
				spans = append(spans, convertInlineElement(child, parentBold, true)...)
			}
		} else {
			spans = append(spans, MarkdownSpan{
				Text:   elem.Content,
				Italic: true,
			})
		}

	case ast.InlineLink:
		spans = append(spans, MarkdownSpan{
			Text: elem.Content,
			Link: elem.URL,
		})
	}

	return spans
}

// convertBlock converts an org block (SRC, QUOTE, etc.) to a MarkdownBlock
func convertBlock(b *ast.Block) MarkdownBlock {
	switch strings.ToUpper(b.Type) {
	case "SRC":
		return MarkdownBlock{
			Type:     "code",
			Language: b.Language,
			Spans:    []MarkdownSpan{{Text: b.Content, CodeBlock: true}},
		}

	case "QUOTE":
		return MarkdownBlock{
			Type:  "quote",
			Spans: []MarkdownSpan{{Text: b.Content, Blockquote: true}},
		}

	case "EXAMPLE":
		return MarkdownBlock{
			Type:  "code",
			Spans: []MarkdownSpan{{Text: b.Content, CodeBlock: true}},
		}

	default:
		// VERSE, CENTER, EXPORT, etc. - render as paragraph
		return MarkdownBlock{
			Type:  "paragraph",
			Spans: []MarkdownSpan{{Text: b.Content}},
		}
	}
}

// convertList converts an org list to blocks
func convertList(l *ast.List) []MarkdownBlock {
	var blocks []MarkdownBlock

	for i, item := range l.Items {
		// Build marker
		marker := "• "
		if l.Ordered {
			marker = string(rune('1'+i)) + ". "
		}

		// Add checkbox if present
		switch item.Checkbox {
		case ast.CheckboxUnchecked:
			marker += "[ ] "
		case ast.CheckboxChecked:
			marker += "[X] "
		case ast.CheckboxPartial:
			marker += "[-] "
		}

		spans := []MarkdownSpan{{Text: marker, ListItem: true}}
		spans = append(spans, MarkdownSpan{Text: item.Content, ListIndent: item.Indent})

		blocks = append(blocks, MarkdownBlock{
			Type:  "list",
			Level: item.Indent,
			Spans: spans,
		})

		// Process nested content
		for _, child := range item.Children {
			childBlocks := convertOrgNode(child)
			// Indent nested blocks
			for i := range childBlocks {
				for j := range childBlocks[i].Spans {
					childBlocks[i].Spans[j].ListIndent++
				}
			}
			blocks = append(blocks, childBlocks...)
		}
	}

	return blocks
}

// convertKeyword converts an org keyword (#+TITLE, etc.) to a block
func convertKeyword(k *ast.Keyword) MarkdownBlock {
	// Show document keywords as italic metadata
	return MarkdownBlock{
		Type: "paragraph",
		Spans: []MarkdownSpan{
			{Text: k.Key + ": ", Bold: true},
			{Text: k.Value, Italic: true},
		},
	}
}

// convertTable converts an org table to a code block (simple rendering)
func convertTable(t *ast.Table) MarkdownBlock {
	var content strings.Builder

	for _, row := range t.Rows {
		if row.Separator {
			content.WriteString("|")
			for range row.Cells {
				content.WriteString("---|")
			}
			content.WriteString("\n")
		} else {
			content.WriteString("| ")
			content.WriteString(strings.Join(row.Cells, " | "))
			content.WriteString(" |\n")
		}
	}

	// Render tables as code blocks for now (monospace alignment)
	return MarkdownBlock{
		Type:  "code",
		Spans: []MarkdownSpan{{Text: content.String(), CodeBlock: true}},
	}
}
