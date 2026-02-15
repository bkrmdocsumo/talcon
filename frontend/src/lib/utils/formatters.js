/**
 * Shared formatting utilities for tool calls, paths, and text truncation.
 * Used by ChatMessage, AgentWorkspace, and App components.
 */

/**
 * Convert a snake_case tool name to a readable label.
 * e.g. "read_file" → "read file"
 */
export function formatToolName(name) {
  return (name || '').replace(/_/g, ' ');
}

/**
 * Truncate a string to `max` characters, appending "…" if truncated.
 */
export function truncate(s, max) {
  if (!s) return '';
  return s.length > max ? s.slice(0, max) + '…' : s;
}

/**
 * Shorten a file path to just the last two segments.
 * e.g. "/Users/me/project/src/main.go" → "…/src/main.go"
 */
export function shortPath(p) {
  if (!p) return '';
  const parts = p.split('/');
  return parts.length > 2
    ? '…/' + parts.slice(-2).join('/')
    : parts.join('/');
}

/**
 * Shorten a URL to hostname + truncated path.
 * e.g. "https://example.com/some/long/path" → "example.com/some/long/pa…"
 */
export function shortUrl(url) {
  if (!url) return '';
  try {
    const u = new URL(url);
    const path = u.pathname === '/' ? '' : u.pathname;
    return u.hostname + truncate(path, 30);
  } catch {
    return truncate(url, 40);
  }
}

/**
 * Build a descriptive label for a tool call from its name + input JSON.
 * e.g. "read file — config.json", "navigate to — docsumo.com"
 */
export function formatToolLabel(name, inputJson) {
  let detail = '';
  try {
    const input = JSON.parse(inputJson || '{}');
    switch (name) {
      case 'read_file':
        detail = shortPath(input.path);
        break;
      case 'write_file':
        detail = shortPath(input.path);
        break;
      case 'run_command':
        detail = truncate(input.command, 50);
        break;
      case 'execute_code':
        detail = input.language || '';
        break;
      case 'browser_navigate':
        detail = shortUrl(input.url);
        break;
      case 'browser_act': {
        const action = input.action || '';
        if (action === 'click') detail = `click #${input.target_id ?? ''}`;
        else if (action === 'type') detail = `type "${truncate(input.text, 30)}"`;
        else if (action === 'scroll') detail = `scroll ${input.text || 'down'}`;
        else detail = action;
        break;
      }
      case 'save_memory':
        detail = input.key || '';
        break;
      case 'memory_search':
        detail = input.query || '';
        break;
      case 'delete_memory':
        detail = input.key || '';
        break;
      case 'todo_write':
        detail = 'update plan';
        break;
    }
  } catch { /* ignore parse errors */ }

  const label = formatToolName(name);
  return detail ? `${label} — ${detail}` : label;
}
