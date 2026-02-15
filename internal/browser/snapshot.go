package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// snapshotElement is a single interactive element extracted from the DOM.
type snapshotElement struct {
	ID          int    `json:"id"`
	Tag         string `json:"tag"`
	Text        string `json:"text"`
	Placeholder string `json:"placeholder"`
	Href        string `json:"href"`
	InputType   string `json:"type"`
	Role        string `json:"role"`
}

// domJS is the JavaScript injected into the page to walk the DOM, assign IDs
// to interactive elements, and return a structured description.
const domJS = `
(() => {
    const interactiveSelectors = 'a, button, input, select, textarea, [role="button"], [onclick]';
    const elements = document.querySelectorAll(interactiveSelectors);
    const results = [];
    let id = 1;
    
    elements.forEach(el => {
        // Skip hidden elements.
        const style = window.getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden') return;
        
        el.setAttribute('data-talon-id', String(id));
        
        const text = (el.innerText || el.textContent || '').trim().substring(0, 100);
        results.push({
            id: id,
            tag: el.tagName.toLowerCase(),
            text: text,
            placeholder: el.getAttribute('placeholder') || '',
            href: el.getAttribute('href') || '',
            type: el.getAttribute('type') || '',
            role: el.getAttribute('role') || ''
        });
        
        id++;
    });
    
    // Also collect headings and paragraphs for context.
    const contextElements = document.querySelectorAll('h1, h2, h3, h4, h5, h6, p');
    const context = [];
    contextElements.forEach(el => {
        const style = window.getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden') return;
        const text = (el.innerText || el.textContent || '').trim().substring(0, 200);
        if (text) {
            context.push({
                tag: el.tagName.toLowerCase(),
                text: text
            });
        }
    });
    
    return JSON.stringify({ interactive: results, context: context });
})()
`

// contextElement represents non-interactive page context (headings, paragraphs).
type contextElement struct {
	Tag  string `json:"tag"`
	Text string `json:"text"`
}

// domResult is the deserialized output from the injected JS.
type domResult struct {
	Interactive []snapshotElement `json:"interactive"`
	Context     []contextElement  `json:"context"`
}

// TakeSnapshot injects JS into the page to build a semantic representation.
// Returns the text snapshot and a set of interactive element IDs.
func TakeSnapshot(ctx context.Context) (string, map[int]bool, error) {
	// Get page title.
	var title string
	if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
		title = ""
	}

	// Run DOM traversal script.
	var jsonStr string
	if err := chromedp.Run(ctx, chromedp.Evaluate(domJS, &jsonStr)); err != nil {
		return "", nil, fmt.Errorf("evaluate DOM script: %w", err)
	}

	var result domResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", nil, fmt.Errorf("unmarshal DOM result: %w", err)
	}

	// Build text representation.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Page: %s]\n", title))

	// Interactive elements.
	for _, el := range result.Interactive {
		label := el.Text
		if label == "" {
			label = el.Placeholder
		}
		if label == "" {
			label = el.Href
		}

		tagDisplay := formatTag(el.Tag, el.InputType)
		sb.WriteString(fmt.Sprintf("[%d] <%s> %s\n", el.ID, tagDisplay, label))
	}

	// Separator and context.
	if len(result.Context) > 0 {
		sb.WriteString("---\n")
		for _, c := range result.Context {
			tagDisplay := formatContextTag(c.Tag)
			sb.WriteString(fmt.Sprintf("<%s> %s\n", tagDisplay, c.Text))
		}
	}

	// Build element ID set (used for validation in Act).
	elementIDs := make(map[int]bool, len(result.Interactive))
	for _, el := range result.Interactive {
		elementIDs[el.ID] = true
	}

	return sb.String(), elementIDs, nil
}

// formatTag converts tag name + type into a human-readable label.
func formatTag(tag, inputType string) string {
	switch tag {
	case "input":
		if inputType != "" {
			return "Input:" + inputType
		}
		return "Input"
	case "a":
		return "Link"
	case "button":
		return "Button"
	case "select":
		return "Select"
	case "textarea":
		return "Textarea"
	default:
		return titleCase(tag)
	}
}

// formatContextTag converts heading/paragraph tags to display names.
func formatContextTag(tag string) string {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "Heading"
	case "p":
		return "Paragraph"
	default:
		return titleCase(tag)
	}
}

// titleCase converts a string to title case without the deprecated strings.Title.
func titleCase(s string) string {
	return cases.Title(language.English).String(s)
}
