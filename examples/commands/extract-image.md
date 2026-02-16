Extract all visible content from the provided image(s) with high fidelity. Follow these rules strictly:

## Extraction Rules

1. **Text**: Extract every piece of visible text exactly as it appears — preserve spelling, casing, punctuation, and line breaks.
2. **Tables**: Reproduce any tables in clean Markdown table format. Keep column headers, alignment, and all cell values intact.
3. **Lists**: Preserve numbered/bulleted list structure using Markdown syntax.
4. **Labels & Annotations**: Include all labels, captions, axis titles, legends, watermarks, and footnotes.
5. **Layout**: Maintain the logical reading order (top-to-bottom, left-to-right). Use Markdown headings (`##`, `###`) to separate distinct sections or regions of the image.
6. **Numbers & Data**: Reproduce all numerical values exactly — do not round, reformat, or convert units.
7. **Non-text Elements**: For charts, diagrams, or icons, provide a brief `[Description: ...]` placeholder describing what is shown.

## Output Format

- Output **only** the extracted content in Markdown.
- Do **not** add any analysis, interpretation, commentary, or summary.
- Do **not** wrap the output in a code block — just produce clean Markdown text.
- If the image contains multiple distinct sections (e.g. a header, body, footer), separate them with `---`.

## When the Image is Unclear

- If a word or value is partially obscured or unreadable, use `[unclear]` as a placeholder.
- If the image is blank or contains no extractable content, respond with: "No extractable content found in this image."

## File Saving

- After extraction, save the result to a `.md` file using `write_file` with a descriptive name (e.g. `extracted-invoice.md`, `extracted-receipt.md`).
- If the user provides a filename hint, use that instead.
