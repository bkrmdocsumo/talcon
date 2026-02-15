<script>
  import { renderMarkdown } from '../lib/markdown.js';
  import TypingIndicator from './TypingIndicator.svelte';
  import appIcon from '../assets/appicon.png';

  export let message = null;   // { role, content, isError?, files?, steps? }
  export let agentName = 'Talon';
  export let isTyping = false;  // render as a typing placeholder instead

  const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp']);

  // Track which step sections are expanded.
  let expandedSteps = {};

  function toggleStep(index) {
    expandedSteps[index] = !expandedSteps[index];
    expandedSteps = expandedSteps; // trigger reactivity
  }

  function isImage(mime) {
    return IMAGE_TYPES.has(mime);
  }

  function formatSize(bytes) {
    if (!bytes) return '';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function fileExtension(name) {
    const parts = (name || '').split('.');
    return parts.length > 1 ? parts.pop().toUpperCase() : 'FILE';
  }

  function formatToolName(name) {
    return (name || '').replace(/_/g, ' ');
  }

  $: steps = (!isTyping && message?.steps) ? message.steps : [];
  $: hasSteps = steps.length > 0;
  $: isStreamingEmpty = message?.isStreaming && !message.content && steps.length === 0;
</script>

<div
  class="message message-{isTyping ? 'assistant' : message.role}"
  class:message-user={!isTyping && message?.role === 'user'}
  class:error={!isTyping && message?.isError}
>
  <div class="message-avatar">
    {#if !isTyping && message.role === 'user'}
      <div class="avatar avatar-user">U</div>
    {:else}
      <div class="avatar avatar-assistant">
        <img src={appIcon} alt="Talon" class="avatar-icon" />
      </div>
    {/if}
  </div>
  <div class="message-body">
    <div class="message-sender">
      {#if isTyping}
        {agentName}
      {:else}
        {message.role === 'user' ? 'You' : agentName}
      {/if}
    </div>
    <div class="message-content">
      {#if isTyping}
        <TypingIndicator />
      {:else if message.role === 'assistant'}
        {#if isStreamingEmpty}
          <TypingIndicator />
        {:else}
          {#if hasSteps}
            <div class="steps-container">
              {#each steps as step, i}
                {#if step.type === 'thinking'}
                  <button class="step-toggle step-thinking" on:click={() => toggleStep(i)}>
                    <span class="step-icon">{expandedSteps[i] ? '▼' : '▶'}</span>
                    <span class="step-label">Thinking</span>
                  </button>
                  {#if expandedSteps[i]}
                    <div class="step-content step-thinking-content">
                      {step.content}
                    </div>
                  {/if}
                {:else if step.type === 'tool_call'}
                  <button class="step-toggle step-tool" on:click={() => toggleStep(i)}>
                    <span class="step-icon">{expandedSteps[i] ? '▼' : '▶'}</span>
                    <span class="step-tool-icon">
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
                      </svg>
                    </span>
                    <span class="step-label">{formatToolName(step.tool_name)}</span>
                  </button>
                  {#if expandedSteps[i]}
                    <div class="step-content step-tool-content">
                      <pre>{step.tool_input}</pre>
                    </div>
                    {#if steps[i + 1]?.type === 'tool_result'}
                      <div class="step-content step-result-content">
                        <div class="step-result-header">Result:</div>
                        <pre>{steps[i + 1].content}</pre>
                      </div>
                    {/if}
                  {/if}
                {:else if step.type === 'tool_result'}
                  <!-- Rendered inline with its tool_call above -->
                {/if}
              {/each}
            </div>
          {/if}
          {#if message.content}
            {@html renderMarkdown(message.content)}
          {/if}
          {#if message.isStreaming && message.content}
            <span class="streaming-cursor"></span>
          {/if}
        {/if}
      {:else}
        {#if message.files && message.files.length > 0}
          <div class="attachments">
            {#each message.files as file}
              {#if isImage(file.type)}
                <div class="attachment-image">
                  <img src={file.dataUrl} alt={file.name} />
                </div>
              {:else}
                <div class="attachment-file">
                  <div class="attachment-file-icon">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                      <path d="M14 2v6h6M16 13H8M16 17H8M10 9H8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  </div>
                  <div class="attachment-file-info">
                    <span class="attachment-file-name">{file.name}</span>
                    <span class="attachment-file-meta">{fileExtension(file.name)} · {formatSize(file.size)}</span>
                  </div>
                </div>
              {/if}
            {/each}
          </div>
        {/if}
        {#if message.content}
          <p>{message.content}</p>
        {/if}
      {/if}
    </div>
  </div>
</div>

<style>
  .message {
    display: flex;
    gap: 12px;
    padding: 16px 0;
    animation: fadeIn 0.2s ease;
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(6px); }
    to   { opacity: 1; transform: translateY(0); }
  }

  .message + :global(.message) {
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .message.message-user {
    flex-direction: row-reverse;
    margin-left: auto;
    max-width: 85%;
  }

  .message.message-user .message-body {
    text-align: right;
  }

  .message.message-user .message-sender {
    text-align: right;
  }

  .message-avatar {
    flex-shrink: 0;
  }

  .avatar {
    width: 30px;
    height: 30px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    font-weight: 600;
  }

  .avatar-user {
    background: var(--user-accent);
    color: #fff;
  }

  .avatar-assistant {
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    border: 1px solid var(--accent);
    box-shadow: 0 0 8px var(--accent-glow);
    overflow: hidden;
    padding: 0;
  }

  .avatar-icon {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .message-body {
    flex: 1;
    min-width: 0;
  }

  .message-sender {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 4px;
  }

  .message-content {
    font-size: 14px;
    line-height: 1.65;
    color: var(--text-primary);
    word-wrap: break-word;
    overflow-wrap: break-word;
  }

  .message.error .message-content {
    color: #f87171;
  }

  /* ─── Markdown Styles ─── */
  .message-content :global(p) {
    margin: 0 0 8px 0;
  }

  .message-content :global(p:last-child) {
    margin-bottom: 0;
  }

  .message-content :global(pre) {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px 16px;
    overflow-x: auto;
    margin: 10px 0;
    font-size: 13px;
  }

  .message-content :global(code) {
    font-family: var(--font-mono);
    font-size: 0.88em;
  }

  .message-content :global(p code),
  .message-content :global(li code) {
    background: rgba(255, 255, 255, 0.06);
    padding: 2px 6px;
    border-radius: 4px;
    border: 1px solid rgba(255, 255, 255, 0.06);
  }

  .message-content :global(pre code) {
    background: none;
    padding: 0;
    border: none;
    border-radius: 0;
  }

  .message-content :global(ul),
  .message-content :global(ol) {
    margin: 8px 0;
    padding-left: 24px;
  }

  .message-content :global(li) {
    margin: 4px 0;
  }

  .message-content :global(h1),
  .message-content :global(h2),
  .message-content :global(h3) {
    margin: 16px 0 8px 0;
    color: var(--text-primary);
  }

  .message-content :global(h1) { font-size: 20px; }
  .message-content :global(h2) { font-size: 17px; }
  .message-content :global(h3) { font-size: 15px; }

  .message-content :global(blockquote) {
    border-left: 3px solid var(--accent);
    padding-left: 12px;
    margin: 8px 0;
    color: var(--text-secondary);
  }

  .message-content :global(a) {
    color: var(--accent);
    text-decoration: none;
  }

  .message-content :global(a:hover) {
    text-decoration: underline;
  }

  .message-content :global(table) {
    width: 100%;
    border-collapse: collapse;
    margin: 10px 0;
  }

  .message-content :global(th),
  .message-content :global(td) {
    padding: 8px 12px;
    border: 1px solid var(--border);
    text-align: left;
  }

  .message-content :global(th) {
    background: var(--bg-tertiary);
    font-weight: 600;
  }

  .message-content :global(hr) {
    border: none;
    border-top: 1px solid var(--border);
    margin: 16px 0;
  }

  /* ─── Steps (Thinking & Tool Use) ─── */
  .steps-container {
    margin-bottom: 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.02);
  }

  .step-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 8px 12px;
    border: none;
    background: none;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 12px;
    font-family: inherit;
    text-align: left;
    transition: background 0.15s ease;
  }

  .step-toggle:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .step-toggle + .step-toggle {
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .step-icon {
    font-size: 9px;
    color: var(--text-muted);
    flex-shrink: 0;
    width: 12px;
  }

  .step-tool-icon {
    display: flex;
    align-items: center;
    color: var(--accent);
    flex-shrink: 0;
  }

  .step-label {
    font-weight: 500;
  }

  .step-thinking .step-label {
    color: #c084fc;
  }

  .step-tool .step-label {
    color: var(--accent);
  }

  .step-content {
    padding: 8px 12px 10px 32px;
    font-size: 12px;
    line-height: 1.5;
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .step-thinking-content {
    color: var(--text-muted);
    white-space: pre-wrap;
    word-wrap: break-word;
    max-height: 300px;
    overflow-y: auto;
  }

  .step-tool-content pre,
  .step-result-content pre {
    margin: 0;
    padding: 8px;
    background: rgba(0, 0, 0, 0.3);
    border-radius: 6px;
    font-size: 11px;
    font-family: var(--font-mono);
    overflow-x: auto;
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-wrap: break-word;
    max-height: 200px;
    overflow-y: auto;
  }

  .step-result-header {
    font-weight: 500;
    color: var(--text-muted);
    margin-bottom: 4px;
  }

  .step-result-content {
    border-top: none;
    padding-top: 0;
  }

  /* ─── Streaming Cursor ─── */
  .streaming-cursor {
    display: inline-block;
    width: 2px;
    height: 1em;
    background: var(--accent);
    animation: cursorBlink 1s step-end infinite;
    margin-left: 2px;
    vertical-align: text-bottom;
  }

  @keyframes cursorBlink {
    0%, 50% { opacity: 1; }
    51%, 100% { opacity: 0; }
  }

  /* ─── File Attachments ─── */
  .attachments {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 8px;
  }

  .attachment-image {
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid var(--border);
    max-width: 300px;
  }

  .attachment-image img {
    display: block;
    max-width: 100%;
    max-height: 240px;
    object-fit: contain;
    background: rgba(0, 0, 0, 0.2);
  }

  .attachment-file {
    display: flex;
    align-items: center;
    gap: 10px;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 10px 14px;
    max-width: 260px;
  }

  .attachment-file-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.06);
    border-radius: 6px;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .attachment-file-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .attachment-file-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .attachment-file-meta {
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
