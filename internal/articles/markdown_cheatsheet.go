// markdown_cheatsheet.go — issue #565
// The Markdown syntax cheatsheet for the Article editor.
// The Rows() function is the single source of truth for
// what the popover shows.
//
// Source of truth for the supported subset:
//   - goldmark v1.8.2 CommonMark + GFM extension (see
//     internal/records/markdown.go:94-96)
//   - The custom Person Record reference syntax
//     `[Name](#person/D-00123)` parsed by the regex in
//     internal/records/article_service.go:771
//
// The drift detector in markdown_cheatsheet_test.go pins
// this list against those sources of truth. If the renderer
// grows or shrinks the supported subset, this file is the
// place to update — and the drift test will fail until the
// update lands.
package articles

// CheatSheetRow is one row in the Markdown syntax popover.
// The popover renders the rows in the order Rows() returns
// them; callers must not depend on map iteration order.
type CheatSheetRow struct {
	// Key is the stable identifier used for the Copy
	// example button + the drift test's primitive-coverage
	// check. Must be unique within Rows(). Must be a valid
	// CSS-safe string (lowercase kebab-case recommended).
	Key string

	// Syntax is the source-level Markdown the user types.
	// Rendered as <code> in the popover.
	Syntax string

	// Effect is the user-visible result, in 1-3 words.
	// Rendered as a short label in the popover.
	Effect string

	// Example is a self-contained Markdown snippet that
	// demonstrates the syntax. Used for the Copy example
	// button. Must match the syntax the article_service
	// resolver + the goldmark renderer actually accept.
	Example string
}

// Rows returns the Markdown cheatsheet rows in the order
// the popover renders them. The drift test asserts every
// goldmark GFM primitive + the custom Person Record ref is
// covered; the row order is the visual reading order
// (basic primitives first, advanced features last).
func Rows() []CheatSheetRow {
	return []CheatSheetRow{
		// Basic text + structure
		{Key: "heading", Syntax: "## Heading", Effect: "Section heading (h1–h6)", Example: "## The Battle of Gettysburg"},
		{Key: "paragraph", Syntax: "Just text…", Effect: "Paragraph", Example: "The 21st Mississippi held the line at the wheat field."},
		{Key: "emphasis", Syntax: "*italic*", Effect: "Italic", Example: "*emphasis*"},
		{Key: "strong", Syntax: "**bold**", Effect: "Bold", Example: "**important**"},
		{Key: "strikethrough", Syntax: "~~struck~~", Effect: "Strikethrough", Example: "~~draft~~"},
		{Key: "code-inline", Syntax: "`code`", Effect: "Inline code", Example: "`D-00123`"},

		// Lists
		{Key: "unordered-list", Syntax: "- item", Effect: "Bullet list", Example: "- one\n- two\n- three"},
		{Key: "ordered-list", Syntax: "1. item", Effect: "Numbered list", Example: "1. one\n2. two\n3. three"},
		{Key: "task-list", Syntax: "- [ ] task", Effect: "Task list", Example: "- [ ] chase pension file\n- [x] verify service record"},

		// Links + references
		{Key: "link", Syntax: "[label](https://…)", Effect: "External link", Example: "[NPS Gettysburg](https://www.nps.gov/gett/)"},
		{Key: "autolink", Syntax: "<https://…>", Effect: "Auto-linked URL", Example: "<https://www.nps.gov/gett/>"},
		{Key: "image", Syntax: "![alt](url)", Effect: "Image", Example: "![Gettysburg wheat field](https://example.com/gettysburg.jpg)"},
		{Key: "person-record-reference", Syntax: "[Name](#person/D-00123)", Effect: "Person Record reference", Example: "[John Doe](#person/D-00123)"},

		// Blocks
		{Key: "blockquote", Syntax: "> quote", Effect: "Block quote", Example: "> said the captain"},
		{Key: "code-block", Syntax: "```\ncode\n```", Effect: "Code block", Example: "```\nCompany A, 21st Mississippi\n```"},
		{Key: "table", Syntax: "| col | col |\n| --- | --- |", Effect: "Table", Example: "| Rank | Name |\n| --- | --- |\n| Pvt | John Doe |"},
		{Key: "horizontal-rule", Syntax: "---", Effect: "Horizontal rule", Example: "---"},
	}
}
