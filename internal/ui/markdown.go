package ui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	goldtext "github.com/yuin/goldmark/text"
)

// MarkdownSpan represents a styled segment of markdown text
type MarkdownSpan struct {
	Text       string
	Bold       bool
	Italic     bool
	Code       bool
	Heading    int  // 0 = not heading, 1-6 = heading level
	Link       string // URL if this is a link
	ListItem   bool
	ListIndent int
	Blockquote bool
	CodeBlock  bool
	NewLine    bool // Force newline after this span
}

// MarkdownBlock represents a block of markdown content (paragraph, heading, etc.)
type MarkdownBlock struct {
	Spans      []MarkdownSpan
	Type       string // "paragraph", "heading", "code", "list", "quote", "hr"
	Level      int    // For headings (1-6) or list indent
	Language   string // For code blocks
}

// ParseMarkdown parses markdown content and returns blocks for rendering
func ParseMarkdown(content string) []MarkdownBlock {
	source := []byte(content)

	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := goldtext.NewReader(source)
	doc := md.Parser().Parse(reader)

	var blocks []MarkdownBlock

	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *ast.Heading:
			block := MarkdownBlock{
				Type:  "heading",
				Level: n.Level,
			}
			block.Spans = extractInlineSpans(n, source, false, false, 0)
			blocks = append(blocks, block)
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			// Skip paragraphs inside list items - they're handled by the list item
			if _, ok := node.Parent().(*ast.ListItem); ok {
				return ast.WalkContinue, nil
			}
			block := MarkdownBlock{
				Type: "paragraph",
			}
			block.Spans = extractInlineSpans(n, source, false, false, 0)
			blocks = append(blocks, block)
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			block := MarkdownBlock{
				Type:     "code",
				Language: string(n.Language(source)),
			}
			var codeContent strings.Builder
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				codeContent.Write(line.Value(source))
			}
			block.Spans = []MarkdownSpan{{
				Text:      codeContent.String(),
				CodeBlock: true,
			}}
			blocks = append(blocks, block)
			return ast.WalkSkipChildren, nil

		case *ast.CodeBlock:
			block := MarkdownBlock{
				Type: "code",
			}
			var codeContent strings.Builder
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				codeContent.Write(line.Value(source))
			}
			block.Spans = []MarkdownSpan{{
				Text:      codeContent.String(),
				CodeBlock: true,
			}}
			blocks = append(blocks, block)
			return ast.WalkSkipChildren, nil

		case *ast.List:
			// Process list items
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if listItem, ok := child.(*ast.ListItem); ok {
					block := MarkdownBlock{
						Type:  "list",
						Level: 0, // TODO: handle nested lists
					}
					block.Spans = extractListItemSpans(listItem, source, n.IsOrdered())
					blocks = append(blocks, block)
				}
			}
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			block := MarkdownBlock{
				Type: "quote",
			}
			// Extract content from blockquote children
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				spans := extractInlineSpans(child, source, false, false, 0)
				for i := range spans {
					spans[i].Blockquote = true
				}
				block.Spans = append(block.Spans, spans...)
			}
			blocks = append(blocks, block)
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			blocks = append(blocks, MarkdownBlock{
				Type: "hr",
			})
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return blocks
}

// extractInlineSpans extracts styled spans from inline content
func extractInlineSpans(node ast.Node, source []byte, bold, italic bool, listIndent int) []MarkdownSpan {
	var spans []MarkdownSpan

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text := string(n.Segment.Value(source))
			if text != "" {
				spans = append(spans, MarkdownSpan{
					Text:       text,
					Bold:       bold,
					Italic:     italic,
					ListIndent: listIndent,
				})
			}
			if n.HardLineBreak() || n.SoftLineBreak() {
				spans = append(spans, MarkdownSpan{NewLine: true})
			}

		case *ast.Emphasis:
			childBold := bold
			childItalic := italic
			if n.Level == 1 {
				childItalic = true
			} else if n.Level >= 2 {
				childBold = true
			}
			spans = append(spans, extractInlineSpans(n, source, childBold, childItalic, listIndent)...)

		case *ast.CodeSpan:
			var code strings.Builder
			for i := 0; i < n.ChildCount(); i++ {
				if text, ok := n.FirstChild().(*ast.Text); ok {
					code.Write(text.Segment.Value(source))
				}
			}
			// Get text directly from segment
			for seg := n.FirstChild(); seg != nil; seg = seg.NextSibling() {
				if t, ok := seg.(*ast.Text); ok {
					code.Write(t.Segment.Value(source))
				}
			}
			if code.Len() == 0 {
				// Fallback: read from segments
				lines := n.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					code.Write(line.Value(source))
				}
			}
			spans = append(spans, MarkdownSpan{
				Text: code.String(),
				Code: true,
			})

		case *ast.Link:
			linkSpans := extractInlineSpans(n, source, bold, italic, listIndent)
			linkURL := string(n.Destination)
			for i := range linkSpans {
				linkSpans[i].Link = linkURL
			}
			spans = append(spans, linkSpans...)

		case *ast.AutoLink:
			url := string(n.URL(source))
			spans = append(spans, MarkdownSpan{
				Text: url,
				Link: url,
			})

		case *ast.Image:
			// Show alt text for images
			altText := string(n.Text(source))
			if altText == "" {
				altText = "[image]"
			}
			spans = append(spans, MarkdownSpan{
				Text:   altText,
				Italic: true,
			})

		case *ast.String:
			spans = append(spans, MarkdownSpan{
				Text:       string(n.Value),
				Bold:       bold,
				Italic:     italic,
				ListIndent: listIndent,
			})

		default:
			// Recursively handle other nodes
			spans = append(spans, extractInlineSpans(child, source, bold, italic, listIndent)...)
		}
	}

	return spans
}

// extractListItemSpans extracts spans from a list item
func extractListItemSpans(item *ast.ListItem, source []byte, ordered bool) []MarkdownSpan {
	var spans []MarkdownSpan

	// Add bullet/number marker
	marker := "• "
	if ordered {
		marker = "1. " // TODO: proper numbering
	}
	spans = append(spans, MarkdownSpan{
		Text:     marker,
		ListItem: true,
	})

	// Extract content from list item children
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		spans = append(spans, extractInlineSpans(child, source, false, false, 1)...)
	}

	return spans
}

// LayoutMarkdownBlock renders a single markdown block
func (r *Renderer) LayoutMarkdownBlock(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	switch block.Type {
	case "heading":
		return r.layoutMarkdownHeading(gtx, block)
	case "code":
		return r.layoutMarkdownCodeBlock(gtx, block)
	case "quote":
		return r.layoutMarkdownBlockquote(gtx, block)
	case "hr":
		return r.layoutMarkdownHR(gtx)
	case "list":
		return r.layoutMarkdownListItem(gtx, block)
	default:
		return r.layoutMarkdownParagraph(gtx, block)
	}
}

func (r *Renderer) layoutMarkdownHeading(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	// Heading sizes: H1=24sp, H2=20sp, H3=18sp, H4=16sp, H5=14sp, H6=12sp
	sizes := []unit.Sp{24, 20, 18, 16, 14, 12}
	size := sizes[0]
	if block.Level >= 1 && block.Level <= 6 {
		size = sizes[block.Level-1]
	}

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.layoutMarkdownSpans(gtx, block.Spans, size, font.Bold)
	})
}

func (r *Renderer) layoutMarkdownParagraph(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.layoutMarkdownSpans(gtx, block.Spans, unit.Sp(14), font.Normal)
	})
}

func (r *Renderer) layoutMarkdownCodeBlock(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		cornerRadius := gtx.Dp(4)

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				// Rounded background
				rr := clip.RRect{
					Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
					NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
				}
				paint.FillShape(gtx.Ops, colCodeBlockBg, rr.Op(gtx.Ops))
				// Border
				borderRR := clip.RRect{
					Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
					NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
				}
				paint.FillShape(gtx.Ops, colCodeBlockBorder, clip.Stroke{Path: borderRR.Path(gtx.Ops), Width: 1}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					var content string
					for _, span := range block.Spans {
						content += span.Text
					}
					lbl := material.Body2(r.Theme, content)
					lbl.Font.Typeface = "monospace"
					lbl.TextSize = unit.Sp(12)
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

func (r *Renderer) layoutMarkdownBlockquote(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		borderWidth := gtx.Dp(3)
		cornerRadius := gtx.Dp(2)

		return layout.Stack{}.Layout(gtx,
			// Background
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				rr := clip.RRect{
					Rect: image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
					NE:   cornerRadius, NW: cornerRadius, SE: cornerRadius, SW: cornerRadius,
				}
				paint.FillShape(gtx.Ops, colBlockquoteBg, rr.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					// Left border
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, colBlockquoteLine, clip.Rect{Max: image.Pt(borderWidth, gtx.Constraints.Max.Y)}.Op())
						return layout.Dimensions{Size: image.Pt(borderWidth, gtx.Constraints.Min.Y)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return r.layoutMarkdownSpans(gtx, block.Spans, unit.Sp(14), font.Normal)
						})
					}),
				)
			}),
		)
	})
}

func (r *Renderer) layoutMarkdownHR(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(1)
		paint.FillShape(gtx.Ops, colLightGray, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
	})
}

func (r *Renderer) layoutMarkdownListItem(gtx layout.Context, block MarkdownBlock) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.layoutMarkdownSpans(gtx, block.Spans, unit.Sp(14), font.Normal)
	})
}

// layoutMarkdownSpans renders a sequence of styled spans as a wrapped text block
func (r *Renderer) layoutMarkdownSpans(gtx layout.Context, spans []MarkdownSpan, baseSize unit.Sp, baseWeight font.Weight) layout.Dimensions {
	// For simplicity, concatenate all text and apply predominant style
	// A more sophisticated implementation would use rich text with multiple styles
	var content strings.Builder
	hasCode := false
	hasLink := false

	for _, span := range spans {
		if span.NewLine {
			content.WriteString("\n")
			continue
		}
		content.WriteString(span.Text)
		if span.Code {
			hasCode = true
		}
		if span.Link != "" {
			hasLink = true
		}
	}

	lbl := material.Body1(r.Theme, content.String())
	lbl.TextSize = baseSize
	lbl.Font.Weight = baseWeight

	if hasCode {
		lbl.Font.Typeface = "monospace"
	}
	if hasLink {
		lbl.Color = colAccent
	}

	// Handle bold/italic from first span (simplified)
	if len(spans) > 0 {
		if spans[0].Bold {
			lbl.Font.Weight = font.Bold
		}
		if spans[0].Italic {
			lbl.Font.Style = font.Italic
		}
	}

	lbl.Alignment = text.Start

	return lbl.Layout(gtx)
}
