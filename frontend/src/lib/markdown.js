import { marked } from 'marked';

marked.setOptions({
  breaks: true,
  gfm: true,
});

/**
 * Safely render a markdown string to HTML.
 * Returns the raw text on failure.
 */
export function renderMarkdown(text) {
  try {
    return marked.parse(text);
  } catch {
    return text;
  }
}
