package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/ledongthuc/pdf"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ── read_pdf ──

// ReadPDFTool extracts text content from a PDF file.
type ReadPDFTool struct{}

func (t *ReadPDFTool) Name() string { return "read_pdf" }

func (t *ReadPDFTool) Description() string {
	return "Extract text content from a PDF file. Relative paths are resolved within the session workspace. Absolute paths outside the workspace are also allowed. Output is truncated to 10KB."
}

func (t *ReadPDFTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the PDF file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadPDFTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	readPath := resolvePDFPath(ctx, in.Path)

	f, r, err := pdf.Open(readPath)
	if err != nil {
		return fmt.Sprintf("Error opening PDF: %v", err), nil
	}
	defer f.Close()

	var buf bytes.Buffer
	totalPages := r.NumPage()
	buf.WriteString(fmt.Sprintf("[PDF: %d page(s)]\n\n", totalPages))

	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			buf.WriteString(fmt.Sprintf("--- Page %d: (error extracting text: %v) ---\n", i, err))
			continue
		}
		content := strings.TrimSpace(text)
		if content != "" {
			buf.WriteString(fmt.Sprintf("--- Page %d ---\n%s\n\n", i, content))
		} else {
			buf.WriteString(fmt.Sprintf("--- Page %d: (no text content) ---\n\n", i))
		}

		// Stop early if we're already past the output limit.
		if buf.Len() > maxReadBytes {
			break
		}
	}

	output := buf.String()
	if len(output) > maxReadBytes {
		output = output[:maxReadBytes] + "\n... [output truncated at 10KB]"
	}

	return output, nil
}

// ── create_pdf ──

// CreatePDFTool generates a PDF from text content.
type CreatePDFTool struct{}

func (t *CreatePDFTool) Name() string { return "create_pdf" }

func (t *CreatePDFTool) Description() string {
	return "Create a PDF document from text content. The PDF is saved in the session workspace. Use relative paths like 'report.pdf' or 'docs/output.pdf'. Parent directories are created automatically."
}

func (t *CreatePDFTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Output path for the PDF file (e.g. 'report.pdf')",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The text content to render in the PDF",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Optional title displayed at the top of the PDF",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *CreatePDFTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	writePath := in.Path
	if sessionDir := SessionDirFromContext(ctx); sessionDir != "" {
		writePath = resolveSessionPath(sessionDir, in.Path)
	}

	dir := filepath.Dir(writePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err), nil
	}

	doc := fpdf.New("P", "mm", "A4", "")
	doc.SetAutoPageBreak(true, 15)
	doc.AddPage()

	// Title if provided.
	if in.Title != "" {
		doc.SetFont("Helvetica", "B", 18)
		doc.CellFormat(0, 10, in.Title, "", 1, "C", false, 0, "")
		doc.Ln(6)
	}

	// Body text.
	doc.SetFont("Helvetica", "", 11)
	doc.MultiCell(0, 6, in.Content, "", "L", false)

	if err := doc.OutputFileAndClose(writePath); err != nil {
		return fmt.Sprintf("Error creating PDF: %v", err), nil
	}

	return fmt.Sprintf("Successfully created PDF at %s", writePath), nil
}

// ── pdf_info ──

// PDFInfoTool returns metadata and page info for a PDF file.
type PDFInfoTool struct{}

func (t *PDFInfoTool) Name() string { return "pdf_info" }

func (t *PDFInfoTool) Description() string {
	return "Get metadata and information about a PDF file including page count, title, author, creation date, and more. Supports both session-relative and absolute paths."
}

func (t *PDFInfoTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the PDF file",
			},
		},
		"required": []string{"path"},
	}
}

func (t *PDFInfoTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	readPath := resolvePDFPath(ctx, in.Path)

	f, err := os.Open(readPath)
	if err != nil {
		return fmt.Sprintf("Error opening PDF: %v", err), nil
	}
	defer f.Close()

	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	info, err := pdfapi.PDFInfo(f, filepath.Base(readPath), nil, false, conf)
	if err != nil {
		return fmt.Sprintf("Error reading PDF info: %v", err), nil
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("File:          %s\n", filepath.Base(readPath)))
	buf.WriteString(fmt.Sprintf("Pages:         %d\n", info.PageCount))
	buf.WriteString(fmt.Sprintf("Version:       %s\n", info.Version))
	if info.Title != "" {
		buf.WriteString(fmt.Sprintf("Title:         %s\n", info.Title))
	}
	if info.Author != "" {
		buf.WriteString(fmt.Sprintf("Author:        %s\n", info.Author))
	}
	if info.Subject != "" {
		buf.WriteString(fmt.Sprintf("Subject:       %s\n", info.Subject))
	}
	if info.Creator != "" {
		buf.WriteString(fmt.Sprintf("Creator:       %s\n", info.Creator))
	}
	if info.Producer != "" {
		buf.WriteString(fmt.Sprintf("Producer:      %s\n", info.Producer))
	}
	if info.CreationDate != "" {
		buf.WriteString(fmt.Sprintf("Created:       %s\n", info.CreationDate))
	}
	if info.ModificationDate != "" {
		buf.WriteString(fmt.Sprintf("Modified:      %s\n", info.ModificationDate))
	}
	if len(info.Keywords) > 0 {
		buf.WriteString(fmt.Sprintf("Keywords:      %s\n", strings.Join(info.Keywords, ", ")))
	}
	buf.WriteString(fmt.Sprintf("Tagged:        %v\n", info.Tagged))
	buf.WriteString(fmt.Sprintf("Linearized:    %v\n", info.Linearized))
	buf.WriteString(fmt.Sprintf("Encrypted:     %v\n", info.Hybrid))

	return buf.String(), nil
}

// ── merge_pdf ──

// MergePDFTool merges multiple PDF files into one.
type MergePDFTool struct{}

func (t *MergePDFTool) Name() string { return "merge_pdf" }

func (t *MergePDFTool) Description() string {
	return "Merge multiple PDF files into a single PDF. Provide an array of input paths and an output path. All paths are resolved within the session workspace."
}

func (t *MergePDFTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"paths": map[string]interface{}{
				"type":        "array",
				"description": "Array of PDF file paths to merge (in order)",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "Output path for the merged PDF file",
			},
		},
		"required": []string{"paths", "output_path"},
	}
}

func (t *MergePDFTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Paths      []string `json:"paths"`
		OutputPath string   `json:"output_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if len(in.Paths) < 2 {
		return "Error: at least 2 PDF files are required for merging", nil
	}

	// Resolve all paths.
	resolvedPaths := make([]string, len(in.Paths))
	for i, p := range in.Paths {
		resolvedPaths[i] = resolvePDFPath(ctx, p)
	}

	outPath := in.OutputPath
	if sessionDir := SessionDirFromContext(ctx); sessionDir != "" {
		outPath = resolveSessionPath(sessionDir, in.OutputPath)
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err), nil
	}

	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	if err := pdfapi.MergeCreateFile(resolvedPaths, outPath, false, conf); err != nil {
		return fmt.Sprintf("Error merging PDFs: %v", err), nil
	}

	return fmt.Sprintf("Successfully merged %d PDFs into %s", len(in.Paths), outPath), nil
}

// ── split_pdf ──

// SplitPDFTool extracts selected pages from a PDF into a new file.
type SplitPDFTool struct{}

func (t *SplitPDFTool) Name() string { return "split_pdf" }

func (t *SplitPDFTool) Description() string {
	return "Extract selected pages from a PDF into a new PDF file. Pages can be specified as ranges like '1-3', individual pages like '1,3,5', or combinations like '1-3,7,9-12'. All paths are resolved within the session workspace."
}

func (t *SplitPDFTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the source PDF file",
			},
			"pages": map[string]interface{}{
				"type":        "string",
				"description": "Page selection (e.g. '1-3', '1,3,5', '1-3,7,9-12')",
			},
			"output_path": map[string]interface{}{
				"type":        "string",
				"description": "Output path for the new PDF with selected pages",
			},
		},
		"required": []string{"path", "pages", "output_path"},
	}
}

func (t *SplitPDFTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Path       string `json:"path"`
		Pages      string `json:"pages"`
		OutputPath string `json:"output_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	readPath := resolvePDFPath(ctx, in.Path)

	outPath := in.OutputPath
	if sessionDir := SessionDirFromContext(ctx); sessionDir != "" {
		outPath = resolveSessionPath(sessionDir, in.OutputPath)
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err), nil
	}

	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	// TrimFile extracts the selected pages into a new file.
	selectedPages := []string{in.Pages}
	if err := pdfapi.TrimFile(readPath, outPath, selectedPages, conf); err != nil {
		return fmt.Sprintf("Error splitting PDF: %v", err), nil
	}

	return fmt.Sprintf("Successfully extracted pages [%s] from %s to %s", in.Pages, filepath.Base(readPath), outPath), nil
}

// ── helpers ──

// resolvePDFPath resolves a PDF path using session context, similar to ReadFileTool.
// For relative paths, tries the session workspace first. Absolute paths are used as-is.
func resolvePDFPath(ctx context.Context, path string) string {
	if sessionDir := SessionDirFromContext(ctx); sessionDir != "" && !filepath.IsAbs(path) {
		candidate := resolveSessionPath(sessionDir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return path
}
