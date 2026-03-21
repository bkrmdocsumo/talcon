package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
)

// NotionTool lets the agent create, query, read, and append to Notion pages.
// The AI discovers databases and pages dynamically — no hardcoded database ID.
type NotionTool struct {
	baseDir string
}

// NewNotionTool creates a NotionTool that reads credentials from config on each call.
func NewNotionTool(baseDir string) *NotionTool {
	return &NotionTool{baseDir: baseDir}
}

func (t *NotionTool) Name() string { return "manage_notion" }

func (t *NotionTool) Description() string {
	return `Manage Notion workspace — search, read, create, and append to pages and databases.

Actions:
- "search": Search the entire Notion workspace for pages or databases. Use "query" to filter by title. Use "object_type" to filter by "page" or "database". Use this first to discover database IDs and page IDs.
- "list_databases": List all databases the integration has access to.
- "create_page": Create a new page. Provide either "database_id" (page in a database) or "page_id" (nested child page under an existing page). Requires "title" and "content".
- "append_content": Append blocks to an existing page. Requires "page_id" and "content".
- "query_pages": List or search pages in a specific database. Requires "database_id". Optional "query" to filter by title.
- "read_page": Read the content of a page. Requires "page_id".

Workflow: Use "search" or "list_databases" first to find the right database/page, then use other actions with the discovered IDs.

Content supports basic formatting:
- Lines starting with "# ", "## ", "### " become headings.
- Double newlines separate paragraphs.`
}

func (t *NotionTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"search", "list_databases", "create_page", "append_content", "query_pages", "read_page"},
				"description": "The action to perform.",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Page title. Required for create_page.",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Text content (supports # headings and paragraph breaks). Required for create_page and append_content.",
			},
			"page_id": map[string]interface{}{
				"type":        "string",
				"description": "Page ID. Required for append_content and read_page.",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search text to filter results. Used by search and query_pages.",
			},
			"database_id": map[string]interface{}{
				"type":        "string",
				"description": "Database ID. Required for create_page and query_pages. Use search or list_databases to discover this.",
			},
			"object_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"page", "database"},
				"description": "Filter search results by object type. Optional for search.",
			},
		},
		"required": []string{"action"},
	}
}

type notionInput struct {
	Action     string `json:"action"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	PageID     string `json:"page_id"`
	Query      string `json:"query"`
	DatabaseID string `json:"database_id"`
	ObjectType string `json:"object_type"`
}

func (t *NotionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in notionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	cfg, err := config.Load(t.baseDir)
	if err != nil {
		return fmt.Sprintf("Error loading config: %v", err), nil
	}

	token := cfg.NotionToken
	if token == "" {
		return "Error: Notion integration is not configured. Set notion_token in Settings → Integrations.", nil
	}

	switch in.Action {
	case "search":
		return t.handleSearch(ctx, token, in)
	case "list_databases":
		return t.handleListDatabases(ctx, token)
	case "create_page":
		return t.handleCreatePage(ctx, token, in)
	case "append_content":
		return t.handleAppendContent(ctx, token, in)
	case "query_pages":
		return t.handleQueryPages(ctx, token, in.DatabaseID, in)
	case "read_page":
		return t.handleReadPage(ctx, token, in)
	default:
		return fmt.Sprintf("Error: unknown action %q — use search, list_databases, create_page, append_content, query_pages, or read_page", in.Action), nil
	}
}

// ── Action handlers ──

func (t *NotionTool) handleSearch(ctx context.Context, token string, in notionInput) (string, error) {
	body := map[string]interface{}{
		"page_size": 20,
	}
	if strings.TrimSpace(in.Query) != "" {
		body["query"] = in.Query
	}
	if in.ObjectType == "page" || in.ObjectType == "database" {
		body["filter"] = map[string]interface{}{
			"value":    in.ObjectType,
			"property": "object",
		}
	}
	body["sort"] = map[string]interface{}{
		"direction": "descending",
		"timestamp": "last_edited_time",
	}

	resp, err := notionRequest(ctx, http.MethodPost, "https://api.notion.com/v1/search", token, body)
	if err != nil {
		return fmt.Sprintf("Error calling Notion API: %v", err), nil
	}

	var result struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Sprintf("Error parsing response: %v", err), nil
	}

	if len(result.Results) == 0 {
		return "No results found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d result(s):\n", len(result.Results))
	for _, raw := range result.Results {
		var obj struct {
			Object         string                     `json:"object"`
			ID             string                     `json:"id"`
			URL            string                     `json:"url"`
			LastEditedTime string                     `json:"last_edited_time"`
			Properties     map[string]json.RawMessage `json:"properties"`
			Title          []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		title := "(untitled)"
		if obj.Object == "database" && len(obj.Title) > 0 {
			title = obj.Title[0].PlainText
		} else if obj.Object == "page" {
			title = extractTitle(obj.Properties)
		}
		fmt.Fprintf(&b, "  - [%s] %s\n    ID: %s\n    URL: %s\n    Last edited: %s\n", obj.Object, title, obj.ID, obj.URL, obj.LastEditedTime)
	}
	return b.String(), nil
}

func (t *NotionTool) handleListDatabases(ctx context.Context, token string) (string, error) {
	body := map[string]interface{}{
		"filter": map[string]interface{}{
			"value":    "database",
			"property": "object",
		},
		"page_size": 50,
	}

	resp, err := notionRequest(ctx, http.MethodPost, "https://api.notion.com/v1/search", token, body)
	if err != nil {
		return fmt.Sprintf("Error calling Notion API: %v", err), nil
	}

	var result struct {
		Results []struct {
			ID             string `json:"id"`
			URL            string `json:"url"`
			LastEditedTime string `json:"last_edited_time"`
			Title          []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Sprintf("Error parsing response: %v", err), nil
	}

	if len(result.Results) == 0 {
		return "No databases found. Make sure the Notion integration has been added to at least one page or database.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d database(s):\n", len(result.Results))
	for _, db := range result.Results {
		title := "(untitled)"
		if len(db.Title) > 0 {
			title = db.Title[0].PlainText
		}
		fmt.Fprintf(&b, "  - %s\n    ID: %s\n    URL: %s\n    Last edited: %s\n", title, db.ID, db.URL, db.LastEditedTime)
	}
	return b.String(), nil
}

func (t *NotionTool) handleReadPage(ctx context.Context, token string, in notionInput) (string, error) {
	if strings.TrimSpace(in.PageID) == "" {
		return "Error: 'page_id' is required for the read_page action", nil
	}

	// Fetch page metadata.
	pageURL := fmt.Sprintf("https://api.notion.com/v1/pages/%s", in.PageID)
	pageResp, err := notionRequest(ctx, http.MethodGet, pageURL, token, nil)
	if err != nil {
		return fmt.Sprintf("Error fetching page: %v", err), nil
	}

	var pageMeta struct {
		ID             string                     `json:"id"`
		URL            string                     `json:"url"`
		LastEditedTime string                     `json:"last_edited_time"`
		Properties     map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(pageResp, &pageMeta); err != nil {
		return fmt.Sprintf("Error parsing page metadata: %v", err), nil
	}

	title := extractTitle(pageMeta.Properties)

	// Fetch page content (blocks).
	blocksURL := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children?page_size=100", in.PageID)
	blocksResp, err := notionRequest(ctx, http.MethodGet, blocksURL, token, nil)
	if err != nil {
		return fmt.Sprintf("Error fetching page blocks: %v", err), nil
	}

	var blocksResult struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(blocksResp, &blocksResult); err != nil {
		return fmt.Sprintf("Error parsing blocks: %v", err), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Page: %s\n  ID: %s\n  URL: %s\n  Last edited: %s\n\nContent:\n", title, pageMeta.ID, pageMeta.URL, pageMeta.LastEditedTime)

	for _, raw := range blocksResult.Results {
		var block struct {
			Type      string `json:"type"`
			Paragraph struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"paragraph"`
			Heading1 struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"heading_1"`
			Heading2 struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"heading_2"`
			Heading3 struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"heading_3"`
			BulletedListItem struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"bulleted_list_item"`
			NumberedListItem struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
			} `json:"numbered_list_item"`
			ToDo struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
				Checked  bool                                              `json:"checked"`
			} `json:"to_do"`
			Code struct {
				RichText []struct{ PlainText string `json:"plain_text"` } `json:"rich_text"`
				Language string                                            `json:"language"`
			} `json:"code"`
		}
		if json.Unmarshal(raw, &block) != nil {
			continue
		}

		switch block.Type {
		case "paragraph":
			text := richTextToPlain(block.Paragraph.RichText)
			if text != "" {
				fmt.Fprintf(&b, "%s\n\n", text)
			}
		case "heading_1":
			fmt.Fprintf(&b, "# %s\n\n", richTextToPlain(block.Heading1.RichText))
		case "heading_2":
			fmt.Fprintf(&b, "## %s\n\n", richTextToPlain(block.Heading2.RichText))
		case "heading_3":
			fmt.Fprintf(&b, "### %s\n\n", richTextToPlain(block.Heading3.RichText))
		case "bulleted_list_item":
			fmt.Fprintf(&b, "- %s\n", richTextToPlain(block.BulletedListItem.RichText))
		case "numbered_list_item":
			fmt.Fprintf(&b, "1. %s\n", richTextToPlain(block.NumberedListItem.RichText))
		case "to_do":
			check := " "
			if block.ToDo.Checked {
				check = "x"
			}
			fmt.Fprintf(&b, "[%s] %s\n", check, richTextToPlain(block.ToDo.RichText))
		case "code":
			fmt.Fprintf(&b, "```%s\n%s\n```\n\n", block.Code.Language, richTextToPlain(block.Code.RichText))
		default:
			// Skip unsupported block types silently.
		}
	}

	return b.String(), nil
}

func (t *NotionTool) handleCreatePage(ctx context.Context, token string, in notionInput) (string, error) {
	if strings.TrimSpace(in.Title) == "" {
		return "Error: 'title' is required for the create_page action", nil
	}
	if strings.TrimSpace(in.Content) == "" {
		return "Error: 'content' is required for the create_page action", nil
	}

	// Determine parent: database_id or page_id (for nested/child pages).
	var parent map[string]interface{}
	if in.PageID != "" {
		parent = map[string]interface{}{"page_id": in.PageID}
	} else if in.DatabaseID != "" {
		parent = map[string]interface{}{"database_id": in.DatabaseID}
	} else {
		return "Error: provide either 'database_id' (page in a database) or 'page_id' (nested child page) for create_page", nil
	}

	body := map[string]interface{}{
		"parent": parent,
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"title": []map[string]interface{}{
					{"text": map[string]interface{}{"content": in.Title}},
				},
			},
		},
		"children": contentToBlocks(in.Content),
	}

	resp, err := notionRequest(ctx, http.MethodPost, "https://api.notion.com/v1/pages", token, body)
	if err != nil {
		return fmt.Sprintf("Error calling Notion API: %v", err), nil
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Sprintf("Page created but could not parse response: %s", string(resp)), nil
	}

	return fmt.Sprintf("Page created successfully.\n  ID: %s\n  URL: %s\n  Title: %s", result.ID, result.URL, in.Title), nil
}

func (t *NotionTool) handleAppendContent(ctx context.Context, token string, in notionInput) (string, error) {
	if strings.TrimSpace(in.PageID) == "" {
		return "Error: 'page_id' is required for the append_content action", nil
	}
	if strings.TrimSpace(in.Content) == "" {
		return "Error: 'content' is required for the append_content action", nil
	}

	body := map[string]interface{}{
		"children": contentToBlocks(in.Content),
	}

	url := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", in.PageID)
	resp, err := notionRequest(ctx, http.MethodPatch, url, token, body)
	if err != nil {
		return fmt.Sprintf("Error calling Notion API: %v", err), nil
	}

	// Check for error in response.
	var check struct {
		Object string `json:"object"`
	}
	if json.Unmarshal(resp, &check) == nil && check.Object == "error" {
		return fmt.Sprintf("Notion API error: %s", string(resp)), nil
	}

	return "Content appended successfully.", nil
}

func (t *NotionTool) handleQueryPages(ctx context.Context, token, dbID string, in notionInput) (string, error) {
	if dbID == "" {
		return "Error: no database_id provided and none configured in settings", nil
	}

	body := map[string]interface{}{
		"page_size": 10,
		"sorts": []map[string]interface{}{
			{"timestamp": "last_edited_time", "direction": "descending"},
		},
	}

	if strings.TrimSpace(in.Query) != "" {
		body["filter"] = map[string]interface{}{
			"property": "title",
			"rich_text": map[string]interface{}{
				"contains": in.Query,
			},
		}
	}

	url := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", dbID)
	resp, err := notionRequest(ctx, http.MethodPost, url, token, body)
	if err != nil {
		return fmt.Sprintf("Error calling Notion API: %v", err), nil
	}

	var result struct {
		Results []struct {
			ID             string `json:"id"`
			URL            string `json:"url"`
			LastEditedTime string `json:"last_edited_time"`
			Properties     map[string]json.RawMessage `json:"properties"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Sprintf("Error parsing response: %v\n%s", err, string(resp)), nil
	}

	if len(result.Results) == 0 {
		return "No pages found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d page(s):\n", len(result.Results))
	for _, p := range result.Results {
		title := extractTitle(p.Properties)
		fmt.Fprintf(&b, "  - %s\n    ID: %s\n    URL: %s\n    Last edited: %s\n", title, p.ID, p.URL, p.LastEditedTime)
	}
	return b.String(), nil
}

// ── Notion API helpers ──

func notionRequest(ctx context.Context, method, url, token string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Notion API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// contentToBlocks converts a text string into Notion block children.
// Lines starting with #/##/### become heading blocks; paragraphs are
// separated by double newlines. Text chunks are capped at 2000 chars
// per rich_text element (Notion API limit).
func contentToBlocks(content string) []interface{} {
	paragraphs := strings.Split(content, "\n\n")
	blocks := make([]interface{}, 0, len(paragraphs))

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		blockType, text := detectHeading(p)
		richText := toRichText(text)

		block := map[string]interface{}{
			"object": "block",
			"type":   blockType,
			blockType: map[string]interface{}{
				"rich_text": richText,
			},
		}
		blocks = append(blocks, block)
	}

	return blocks
}

// detectHeading checks for markdown-style heading prefixes and returns
// the Notion block type and the remaining text.
func detectHeading(text string) (string, string) {
	if strings.HasPrefix(text, "### ") {
		return "heading_3", strings.TrimPrefix(text, "### ")
	}
	if strings.HasPrefix(text, "## ") {
		return "heading_2", strings.TrimPrefix(text, "## ")
	}
	if strings.HasPrefix(text, "# ") {
		return "heading_1", strings.TrimPrefix(text, "# ")
	}
	return "paragraph", text
}

// toRichText splits text into chunks of at most 2000 characters,
// each wrapped as a Notion rich_text element.
func toRichText(text string) []map[string]interface{} {
	const maxLen = 2000

	if len(text) <= maxLen {
		return []map[string]interface{}{
			{"type": "text", "text": map[string]interface{}{"content": text}},
		}
	}

	var parts []map[string]interface{}
	for len(text) > 0 {
		end := maxLen
		if end > len(text) {
			end = len(text)
		}
		parts = append(parts, map[string]interface{}{
			"type": "text",
			"text": map[string]interface{}{"content": text[:end]},
		})
		text = text[end:]
	}
	return parts
}

// richTextToPlain concatenates a slice of Notion rich_text objects into plain text.
func richTextToPlain(parts []struct{ PlainText string `json:"plain_text"` }) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.PlainText)
	}
	return sb.String()
}

// extractTitle pulls the page title from Notion properties.
func extractTitle(props map[string]json.RawMessage) string {
	// The title property can be named anything; find the one with type "title".
	for _, raw := range props {
		var prop struct {
			Type  string `json:"type"`
			Title []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		}
		if json.Unmarshal(raw, &prop) == nil && prop.Type == "title" && len(prop.Title) > 0 {
			return prop.Title[0].PlainText
		}
	}
	return "(untitled)"
}
