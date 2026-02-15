<script>
  import { tick, createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { StartVoiceRecording, StopVoiceAndTranscribe } from '../../wailsjs/go/main/App';
  import { EventsOn } from '../../wailsjs/runtime/runtime';

  export let disabled = false;
  export let loading = false;
  export let agentName = 'Talon';

  let input = '';
  let textareaEl;
  let fileInputEl;
  let files = []; // { name, type, size, dataUrl, data }

  // Voice recording state
  let isRecording = false;
  let isTranscribing = false;
  let voiceError = '';
  let recordingSeconds = 0;
  let recordingTimer = null;

  // Listen for native recording errors (e.g. permission denied).
  let cleanupVoiceError = null;

  onMount(() => {
    cleanupVoiceError = EventsOn('voice:error', (data) => {
      if (!data) return;
      voiceError = data.error || 'Recording error.';
      if (isRecording) {
        isRecording = false;
        if (recordingTimer) { clearInterval(recordingTimer); recordingTimer = null; }
      }
      setTimeout(() => { voiceError = ''; }, 5000);
    });
  });

  onDestroy(() => {
    cleanupVoice();
    if (cleanupVoiceError) cleanupVoiceError();
  });

  // Model selector
  let selectedModel = 'Sonnet 4.5';
  let showModelMenu = false;
  const models = [
    { id: 'claude-sonnet-4-5-20250514', label: 'Sonnet 4.5' },
    { id: 'claude-opus-4-5-20250514', label: 'Opus 4.5' },
    { id: 'claude-haiku-4-5-20250514', label: 'Haiku 4.5' },
  ];

  const dispatch = createEventDispatcher();

  // Accepted file types
  const ACCEPTED_TYPES = [
    'image/png', 'image/jpeg', 'image/gif', 'image/webp',
    'application/pdf',
    'text/plain', 'text/csv', 'text/markdown', 'text/html',
    'application/json', 'application/xml',
  ].join(',');

  const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp']);
  const MAX_FILE_SIZE = 20 * 1024 * 1024; // 20 MB

  export function focus() {
    textareaEl?.focus();
  }

  export async function clear() {
    input = '';
    files = [];
    await tick();
    if (textareaEl) {
      textareaEl.style.height = 'auto';
    }
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
    if ((!text && files.length === 0) || disabled) return;
    dispatch('send', { text, files: [...files] });
  }

  function openFilePicker() {
    fileInputEl?.click();
  }

  async function handleFileSelect(e) {
    const selected = Array.from(e.target.files || []);
    for (const file of selected) {
      if (file.size > MAX_FILE_SIZE) {
        alert(`File "${file.name}" exceeds the 20 MB limit.`);
        continue;
      }
      const result = await readFileAsBase64(file);
      files = [...files, {
        name: file.name,
        type: file.type || 'text/plain',
        size: file.size,
        dataUrl: result.dataUrl,
        data: result.base64,
      }];
    }
    e.target.value = '';
    textareaEl?.focus();
  }

  function readFileAsBase64(file) {
    return new Promise((resolve) => {
      const reader = new FileReader();
      reader.onload = () => {
        const dataUrl = reader.result;
        const base64 = dataUrl.split(',')[1] || '';
        resolve({ dataUrl, base64 });
      };
      reader.readAsDataURL(file);
    });
  }

  function removeFile(index) {
    files = files.filter((_, i) => i !== index);
    textareaEl?.focus();
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function isImage(mime) {
    return IMAGE_TYPES.has(mime);
  }

  function selectModel(model) {
    selectedModel = model.label;
    showModelMenu = false;
    dispatch('modelChange', { id: model.id, label: model.label });
  }

  function toggleModelMenu() {
    showModelMenu = !showModelMenu;
  }

  // Close model menu on outside click
  function handleWindowClick(e) {
    if (showModelMenu) {
      showModelMenu = false;
    }
  }

  // ─── Voice Recording (native macOS via Go backend) ───
  async function toggleVoice() {
    if (isRecording) {
      await stopAndTranscribeVoice();
    } else {
      await startVoice();
    }
  }

  async function startVoice() {
    voiceError = '';
    isRecording = true;
    recordingSeconds = 0;
    recordingTimer = setInterval(() => { recordingSeconds++; }, 1000);

    try {
      await StartVoiceRecording();
    } catch (e) {
      voiceError = `Failed to start recording: ${e}`;
      isRecording = false;
      if (recordingTimer) { clearInterval(recordingTimer); recordingTimer = null; }
      setTimeout(() => { voiceError = ''; }, 5000);
    }
  }

  async function stopAndTranscribeVoice() {
    isRecording = false;
    isTranscribing = true;

    if (recordingTimer) {
      clearInterval(recordingTimer);
      recordingTimer = null;
    }

    try {
      const text = await StopVoiceAndTranscribe();

      if (text && text.trim()) {
        // Append transcribed text to current input.
        input += (input && !input.endsWith(' ') ? ' ' : '') + text.trim();
        await tick();
        if (textareaEl) {
          textareaEl.style.height = 'auto';
          textareaEl.style.height = Math.min(textareaEl.scrollHeight, 200) + 'px';
          textareaEl.focus();
        }
      } else {
        voiceError = 'No speech detected.';
        setTimeout(() => { voiceError = ''; }, 3000);
      }
    } catch (e) {
      voiceError = `Transcription failed: ${e}`;
      setTimeout(() => { voiceError = ''; }, 5000);
    }

    isTranscribing = false;
  }

  function cleanupVoice() {
    if (recordingTimer) {
      clearInterval(recordingTimer);
      recordingTimer = null;
    }
  }

  function formatRecTime(secs) {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  }

  // Drag and drop
  let dragOver = false;

  function handleDragOver(e) {
    e.preventDefault();
    dragOver = true;
  }

  function handleDragLeave() {
    dragOver = false;
  }

  async function handleDrop(e) {
    e.preventDefault();
    dragOver = false;
    const droppedFiles = Array.from(e.dataTransfer?.files || []);
    for (const file of droppedFiles) {
      if (file.size > MAX_FILE_SIZE) {
        alert(`File "${file.name}" exceeds the 20 MB limit.`);
        continue;
      }
      const result = await readFileAsBase64(file);
      files = [...files, {
        name: file.name,
        type: file.type || 'text/plain',
        size: file.size,
        dataUrl: result.dataUrl,
        data: result.base64,
      }];
    }
  }
</script>

<svelte:window on:click={handleWindowClick} />

<footer class="input-area">
  <div
    class="input-container"
    class:drag-over={dragOver}
    on:dragover={handleDragOver}
    on:dragleave={handleDragLeave}
    on:drop={handleDrop}
  >
    {#if files.length > 0}
      <div class="file-preview-row">
        {#each files as file, i}
          <div class="file-chip" title={file.name}>
            {#if isImage(file.type)}
              <img class="file-thumb" src={file.dataUrl} alt={file.name} />
            {:else}
              <div class="file-icon">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                  <path d="M14 2v6h6M16 13H8M16 17H8M10 9H8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </div>
            {/if}
            <span class="file-name">{file.name}</span>
            <span class="file-size">{formatSize(file.size)}</span>
            <button class="file-remove" on:click={() => removeFile(i)} title="Remove file">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none">
                <path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
              </svg>
            </button>
          </div>
        {/each}
      </div>
    {/if}

    <textarea
      bind:this={textareaEl}
      bind:value={input}
      on:keydown={handleKeydown}
      on:input={handleInput}
      placeholder="How can I help you today?"
      rows="1"
      disabled={disabled}
    ></textarea>

    <div class="input-bottom">
      <div class="input-bottom-left">
        <button
          class="btn-attach"
          on:click|stopPropagation={openFilePicker}
          disabled={disabled}
          title="Attach file"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M12 5v14M5 12h14" />
          </svg>
        </button>

        <!-- Voice Input Button -->
        <button
          class="btn-voice"
          class:voice-recording={isRecording}
          class:voice-transcribing={isTranscribing}
          on:click|stopPropagation={toggleVoice}
          disabled={disabled || isTranscribing}
          title={isRecording ? `Recording ${formatRecTime(recordingSeconds)} — click to stop` : isTranscribing ? 'Transcribing...' : 'Voice input'}
        >
          {#if isRecording}
            <span class="voice-rec-dot"></span>
            <span class="voice-time">{formatRecTime(recordingSeconds)}</span>
          {:else if isTranscribing}
            <div class="voice-spinner"></div>
          {:else}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
              <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
              <line x1="12" y1="19" x2="12" y2="23"/>
              <line x1="8" y1="23" x2="16" y2="23"/>
            </svg>
          {/if}
        </button>

        {#if voiceError}
          <span class="voice-error-hint">{voiceError}</span>
        {/if}
      </div>

      <div class="input-bottom-right">
        <!-- Model Selector -->
        <div class="model-selector-wrapper">
          <button
            class="model-selector"
            on:click|stopPropagation={toggleModelMenu}
          >
            {selectedModel}
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
              <path d="M6 9l6 6 6-6" />
            </svg>
          </button>

          {#if showModelMenu}
            <div class="model-menu" on:click|stopPropagation>
              {#each models as model}
                <button
                  class="model-option"
                  class:selected={model.label === selectedModel}
                  on:click={() => selectModel(model)}
                >
                  {model.label}
                  {#if model.label === selectedModel}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                      <path d="M20 6L9 17l-5-5" />
                    </svg>
                  {/if}
                </button>
              {/each}
            </div>
          {/if}
        </div>

        {#if loading}
          <button
            class="btn-send btn-cancel"
            on:click={() => dispatch('cancel')}
            title="Cancel request"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
              <rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/>
            </svg>
          </button>
        {:else}
          <button
            class="btn-send"
            on:click={handleSend}
            disabled={(!input.trim() && files.length === 0) || disabled}
            title="Send message"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
              <path d="M7 11l5-5m0 0l5 5m-5-5v12" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        {/if}
      </div>
    </div>
  </div>

  <input
    bind:this={fileInputEl}
    type="file"
    multiple
    accept={ACCEPTED_TYPES}
    on:change={handleFileSelect}
    class="hidden-file-input"
  />
</footer>

<style>
  .input-area {
    padding: 0 24px 24px;
    flex-shrink: 0;
    position: relative;
    overflow: visible;
  }

  .input-container {
    max-width: 680px;
    margin: 0 auto;
    background: var(--bg-input);
    border: 1px solid var(--border);
    border-radius: 16px;
    padding: 0;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;
    position: relative;
  }

  .input-container:focus-within {
    border-color: var(--border-light);
    box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.03);
  }

  .input-container.drag-over {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow);
    background: rgba(14, 240, 216, 0.03);
  }

  textarea {
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
    padding: 16px 18px 8px;
  }

  textarea::placeholder {
    color: var(--text-muted);
  }

  textarea:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  /* ─── Bottom Row ─── */
  .input-bottom {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 8px 8px;
  }

  .input-bottom-left {
    display: flex;
    align-items: center;
  }

  .input-bottom-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .btn-attach {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: 8px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.15s ease;
  }

  .btn-attach:hover:not(:disabled) {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .btn-attach:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  /* ─── Model Selector ─── */
  .model-selector-wrapper {
    position: relative;
  }

  .model-selector {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 5px 10px;
    background: transparent;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .model-selector:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .model-menu {
    position: absolute;
    bottom: 100%;
    right: 0;
    margin-bottom: 6px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-light);
    border-radius: 10px;
    padding: 4px;
    min-width: 160px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    z-index: 50;
    animation: menuFadeIn 0.12s ease;
  }

  @keyframes menuFadeIn {
    from { opacity: 0; transform: translateY(4px); }
    to   { opacity: 1; transform: translateY(0); }
  }

  .model-option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 8px 12px;
    background: none;
    border: none;
    border-radius: 7px;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.1s ease;
    text-align: left;
  }

  .model-option:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .model-option.selected {
    color: var(--text-primary);
  }

  /* ─── Send Button ─── */
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

  /* ─── File Preview ─── */
  .file-preview-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 12px 14px 0;
  }

  .file-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 4px 8px;
    max-width: 220px;
    animation: chipFadeIn 0.15s ease;
  }

  @keyframes chipFadeIn {
    from { opacity: 0; transform: scale(0.95); }
    to   { opacity: 1; transform: scale(1); }
  }

  .file-thumb {
    width: 28px;
    height: 28px;
    border-radius: 4px;
    object-fit: cover;
    flex-shrink: 0;
  }

  .file-icon {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.04);
    border-radius: 4px;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .file-name {
    font-size: 12px;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100px;
  }

  .file-size {
    font-size: 11px;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .file-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    background: transparent;
    border: none;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.1s ease;
  }

  .file-remove:hover {
    color: #f87171;
    background: rgba(248, 113, 113, 0.1);
  }

  .hidden-file-input {
    position: absolute;
    width: 0;
    height: 0;
    opacity: 0;
    pointer-events: none;
  }

  /* ─── Voice Button ─── */
  .btn-voice {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 5px;
    height: 32px;
    min-width: 32px;
    padding: 0 6px;
    background: transparent;
    border: none;
    border-radius: 8px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.15s ease;
  }

  .btn-voice:hover:not(:disabled) {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .btn-voice:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .btn-voice.voice-recording {
    color: var(--danger);
    background: rgba(239, 68, 68, 0.08);
    padding: 0 10px;
  }

  .btn-voice.voice-recording:hover {
    background: rgba(239, 68, 68, 0.15);
  }

  .btn-voice.voice-transcribing {
    color: var(--accent);
    background: rgba(14, 240, 216, 0.08);
  }

  .voice-rec-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--danger);
    animation: voicePulse 1s ease-in-out infinite;
    flex-shrink: 0;
  }

  @keyframes voicePulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.3; }
  }

  .voice-time {
    font-size: 12px;
    font-weight: 600;
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .voice-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid var(--border-light);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: voiceSpin 0.8s linear infinite;
  }

  @keyframes voiceSpin {
    to { transform: rotate(360deg); }
  }

  .voice-error-hint {
    font-size: 11px;
    color: #f87171;
    white-space: nowrap;
    animation: voiceErrorFade 0.2s ease;
  }

  @keyframes voiceErrorFade {
    from { opacity: 0; }
    to { opacity: 1; }
  }
</style>
