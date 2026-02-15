<script>
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import { renderMarkdown } from '../lib/markdown.js';
  import AgentFileCard from './AgentFileCard.svelte';
  import AgentInfoPanel from './AgentInfoPanel.svelte';
  import TypingIndicator from './TypingIndicator.svelte';

  export let taskTitle = '';
  export let messages = [];            // Array of { role: 'user'|'assistant', content, steps?, isStreaming? }
  export let isStreaming = false;
  export let loading = false;
  export let agentName = 'Talon';
  export let disabled = false;

  // Data for the info panel
  export let progressSteps = [];     // Array of { label, status }
  export let createdFiles = [];      // Array of { name, path, type?, size? }
  export let contextTools = [];      // Array of tool name strings

  // Derive the original prompt from the first user message
  $: prompt = (messages.find(m => m.role === 'user')?.content || '').trim();

  const dispatch = createEventDispatcher();

  let contentContainer;
  let input = '';
  let textareaEl;

  afterUpdate(() => {
    scrollToBottom();
  });

  function scrollToBottom() {
    if (contentContainer) {
      contentContainer.scrollTo({
        top: contentContainer.scrollHeight,
        behavior: 'smooth',
      });
    }
  }

  function formatToolName(name) {
    return (name || '').replace(/_/g, ' ');
  }

  // Build summary from all steps across all messages
  $: allSteps = messages.flatMap(m => m.steps || []);
  $: commandCount = allSteps.filter(s => s.type === 'tool_call').length;
  $: toolNames = [...new Set(allSteps.filter(s => s.type === 'tool_call').map(s => s.tool_name))];
  $: summaryText = buildSummary(commandCount, toolNames.length);

  function buildSummary(cmds, tools) {
    const parts = [];
    if (cmds > 0) parts.push(`Ran ${cmds} command${cmds !== 1 ? 's' : ''}`);
    if (tools > 0) parts.push(`used ${tools} tool${tools !== 1 ? 's' : ''}`);
    return parts.length > 0 ? parts.join(', ') : '';
  }

  // Expandable steps (keyed by message index + step index).
  // NOTE: We access expandedSteps directly in the template (not via a helper
  // function) so that Svelte's compiler can track the reactive dependency.
  let expandedSteps = {};
  function toggleStep(msgIdx, stepIdx) {
    const key = `${msgIdx}-${stepIdx}`;
    expandedSteps[key] = !expandedSteps[key];
    expandedSteps = expandedSteps;
  }

  function handleOpenFile(e) {
    dispatch('openFile', e.detail);
  }

  function handleOpenFolder() {
    dispatch('openFolder');
  }

  function handleInfoOpenFile(e) {
    dispatch('openFile', e.detail);
  }

  // ─── Chat Input ───
  export function focus() {
    textareaEl?.focus();
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleInput(e) {
    const el = e.target;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 200) + 'px';
  }

  function handleSend() {
    const text = input.trim();
    if (!text || loading) return;
    dispatch('sendFollowUp', { text });
    input = '';
    if (textareaEl) {
      textareaEl.style.height = 'auto';
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<div class="workspace-layout">
  <!-- Center Content -->
  <div class="workspace-center">
    <div class="workspace-scroll" bind:this={contentContainer}>
      <div class="workspace-content">
        <!-- Task Title -->
        {#if taskTitle}
          <div class="task-title-bar">
            <h2 class="task-title">{taskTitle}</h2>
          </div>
        {/if}

        <!-- Messages -->
        {#each messages as message, msgIdx}
          {#if message.role === 'user'}
            <div class="user-pill">
              <span class="user-pill-text">{message.content}</span>
            </div>
          {:else if message.role === 'assistant'}
            <!-- Summary line (show after first assistant response only, when not streaming) -->
            {#if msgIdx === 1 && summaryText && !isStreaming}
              <div class="summary-line">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                  <path d="M22 4L12 14.01l-3-3"/>
                </svg>
                <span>{summaryText}</span>
              </div>
            {/if}

            <!-- Steps (collapsed tool calls, thinking) -->
            {#if message.steps && message.steps.length > 0}
              <div class="steps-container">
                {#each message.steps as step, i}
                  {#if step.type === 'thinking'}
                    <button class="step-toggle step-thinking" on:click={() => toggleStep(msgIdx, i)}>
                      <span class="step-icon">{expandedSteps[`${msgIdx}-${i}`] ? '▼' : '▶'}</span>
                      <span class="step-label">Thinking</span>
                    </button>
                    {#if expandedSteps[`${msgIdx}-${i}`]}
                      <div class="step-content step-thinking-content">
                        {step.content}
                      </div>
                    {/if}
                  {:else if step.type === 'tool_call'}
                    <button class="step-toggle step-tool" on:click={() => toggleStep(msgIdx, i)}>
                      <span class="step-icon">{expandedSteps[`${msgIdx}-${i}`] ? '▼' : '▶'}</span>
                      <span class="step-tool-icon">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
                        </svg>
                      </span>
                      <span class="step-label">{formatToolName(step.tool_name)}</span>
                    </button>
                    {#if expandedSteps[`${msgIdx}-${i}`]}
                      <div class="step-content step-tool-content">
                        <pre>{step.tool_input}</pre>
                      </div>
                      {#if message.steps[i + 1]?.type === 'tool_result'}
                        <div class="step-content step-result-content">
                          <div class="step-result-header">Result:</div>
                          <pre>{message.steps[i + 1].content}</pre>
                        </div>
                      {/if}
                    {/if}
                  {:else if step.type === 'tool_result'}
                    <!-- Rendered inline with its tool_call above -->
                  {/if}
                {/each}
              </div>
            {/if}

            <!-- Agent Response Text -->
            {#if message.content}
              <div class="agent-response">
                {@html renderMarkdown(message.content)}
                {#if message.isStreaming}
                  <span class="streaming-cursor"></span>
                {/if}
              </div>
            {:else if message.isStreaming}
              <div class="agent-response">
                <TypingIndicator />
              </div>
            {/if}
          {/if}
        {/each}

        <!-- Created File Cards (shown when not streaming) -->
        {#if createdFiles.length > 0 && !isStreaming}
          <div class="file-cards">
            {#each createdFiles as file}
              <AgentFileCard {file} on:openFile={handleOpenFile} />
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <!-- Chat Input -->
    <footer class="workspace-input-area">
      <div class="workspace-input-container">
        <textarea
          bind:this={textareaEl}
          bind:value={input}
          on:keydown={handleKeydown}
          on:input={handleInput}
          placeholder="Send a follow-up..."
          rows="1"
          disabled={disabled || loading}
        ></textarea>
        <div class="workspace-input-bottom">
          <div class="workspace-input-spacer"></div>
          {#if loading}
            <button
              class="btn-send btn-cancel"
              on:click={handleCancel}
              title="Cancel"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                <rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/>
              </svg>
            </button>
          {:else}
            <button
              class="btn-send"
              on:click={handleSend}
              disabled={!input.trim() || disabled}
              title="Send"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                <path d="M7 11l5-5m0 0l5 5m-5-5v12" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
          {/if}
        </div>
      </div>
    </footer>
  </div>

  <!-- Right Info Panel -->
  <AgentInfoPanel
    {progressSteps}
    files={createdFiles}
    {contextTools}
    {taskTitle}
    {prompt}
    on:openFile={handleInfoOpenFile}
    on:openFolder={handleOpenFolder}
  />
</div>

<style>
  .workspace-layout {
    flex: 1;
    display: flex;
    min-height: 0;
    overflow: hidden;
  }

  .workspace-center {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow: hidden;
  }

  .workspace-scroll {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }

  .workspace-scroll::-webkit-scrollbar {
    width: 6px;
  }

  .workspace-scroll::-webkit-scrollbar-track {
    background: transparent;
  }

  .workspace-scroll::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 3px;
  }

  .workspace-content {
    max-width: 720px;
    margin: 0 auto;
    padding: 24px 24px 24px;
  }

  /* Task Title */
  .task-title-bar {
    margin-bottom: 24px;
  }

  .task-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-primary);
    letter-spacing: -0.3px;
  }

  /* User Message Pill */
  .user-pill {
    display: inline-block;
    max-width: 100%;
    padding: 10px 18px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 20px;
    margin-bottom: 16px;
  }

  .user-pill-text {
    font-size: 14px;
    color: var(--text-primary);
    line-height: 1.5;
    word-wrap: break-word;
  }

  /* Summary Line */
  .summary-line {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-muted);
    margin-bottom: 16px;
    padding: 6px 0;
  }

  .summary-line svg {
    color: #22c55e;
    flex-shrink: 0;
  }

  /* Steps */
  .steps-container {
    margin-bottom: 16px;
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

  /* Agent Response */
  .agent-response {
    font-size: 14px;
    line-height: 1.65;
    color: var(--text-primary);
    word-wrap: break-word;
    overflow-wrap: break-word;
    margin-bottom: 16px;
  }

  .agent-response :global(p) {
    margin: 0 0 8px 0;
  }

  .agent-response :global(p:last-child) {
    margin-bottom: 0;
  }

  .agent-response :global(pre) {
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px 16px;
    overflow-x: auto;
    margin: 10px 0;
    font-size: 13px;
  }

  .agent-response :global(code) {
    font-family: var(--font-mono);
    font-size: 0.88em;
  }

  .agent-response :global(p code),
  .agent-response :global(li code) {
    background: rgba(255, 255, 255, 0.06);
    padding: 2px 6px;
    border-radius: 4px;
    border: 1px solid rgba(255, 255, 255, 0.06);
  }

  .agent-response :global(pre code) {
    background: none;
    padding: 0;
    border: none;
  }

  .agent-response :global(ul),
  .agent-response :global(ol) {
    margin: 8px 0;
    padding-left: 24px;
  }

  .agent-response :global(a) {
    color: var(--accent);
    text-decoration: none;
  }

  .agent-response :global(a:hover) {
    text-decoration: underline;
  }

  /* Streaming Cursor */
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

  /* File Cards */
  .file-cards {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 16px;
  }

  /* ─── Chat Input ─── */
  .workspace-input-area {
    padding: 0 24px 20px;
    flex-shrink: 0;
  }

  .workspace-input-container {
    max-width: 680px;
    margin: 0 auto;
    background: var(--bg-input);
    border: 1px solid var(--border);
    border-radius: 16px;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;
  }

  .workspace-input-container:focus-within {
    border-color: var(--border-light);
    box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.03);
  }

  .workspace-input-container textarea {
    --wails-draggable: no-drag;
    display: block;
    width: 100%;
    background: transparent;
    border: none;
    outline: none;
    color: var(--text-primary);
    font-family: var(--font-sans);
    font-size: 15px;
    line-height: 1.5;
    resize: none;
    min-height: 24px;
    max-height: 200px;
    padding: 14px 16px 6px;
  }

  .workspace-input-container textarea::placeholder {
    color: var(--text-muted);
  }

  .workspace-input-container textarea:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .workspace-input-bottom {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    padding: 4px 8px 8px;
  }

  .workspace-input-spacer {
    flex: 1;
  }

  .btn-send {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--accent);
    border: none;
    border-radius: 50%;
    color: #000;
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.15s ease;
  }

  .btn-send:hover:not(:disabled) {
    background: var(--accent-hover);
    transform: scale(1.05);
  }

  .btn-send:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .btn-cancel {
    background: var(--danger);
  }

  .btn-cancel:hover {
    background: #dc2626;
  }
</style>
