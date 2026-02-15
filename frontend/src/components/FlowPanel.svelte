<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { StartVoiceRecording, StopVoiceAndTranscribe } from '../../wailsjs/go/main/App';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

  const dispatch = createEventDispatcher();

  // ─── Props ───
  export let viewingTranscript = null; // { id, text, duration, wordCount, timestamp } or null

  // ─── State ───
  let state = 'idle'; // 'idle' | 'recording' | 'transcribing'
  let transcript = '';
  let error = '';
  let wordCount = 0;
  let sessionSeconds = 0;
  let sessionTimer = null;
  let textareaEl;
  let copyFeedback = false;
  let saveFeedback = false;

  // Event cleanup
  let cleanupVoiceError = null;

  // ─── Viewing mode ───
  $: isViewingHistory = viewingTranscript !== null && state === 'idle';

  // When viewingTranscript changes, update the textarea
  $: if (viewingTranscript && state === 'idle') {
    transcript = viewingTranscript.text || '';
    wordCount = viewingTranscript.wordCount || 0;
    sessionSeconds = viewingTranscript.duration || 0;
  }

  // ─── Lifecycle ───
  onMount(() => {
    cleanupVoiceError = EventsOn('voice:error', (data) => {
      if (!data) return;
      error = data.error || 'An unknown error occurred.';
      if (state === 'recording') {
        state = 'idle';
        if (sessionTimer) {
          clearInterval(sessionTimer);
          sessionTimer = null;
        }
      }
    });
  });

  onDestroy(() => {
    if (sessionTimer) {
      clearInterval(sessionTimer);
      sessionTimer = null;
    }
    if (cleanupVoiceError) cleanupVoiceError();
  });

  // ─── Actions ───
  function toggleRecording() {
    if (state === 'recording') {
      stopAndTranscribe();
    } else if (state === 'idle') {
      startRecording();
    }
  }

  async function startRecording() {
    // If viewing a saved transcript, clear it to start fresh.
    if (isViewingHistory) {
      dispatch('clearView');
    }

    error = '';
    transcript = '';
    wordCount = 0;
    sessionSeconds = 0;
    state = 'recording';
    sessionTimer = setInterval(() => { sessionSeconds++; }, 1000);

    try {
      await StartVoiceRecording();
    } catch (e) {
      error = `Failed to start recording: ${e}`;
      state = 'idle';
      if (sessionTimer) { clearInterval(sessionTimer); sessionTimer = null; }
    }
  }

  async function stopAndTranscribe() {
    state = 'transcribing';

    if (sessionTimer) {
      clearInterval(sessionTimer);
      sessionTimer = null;
    }

    try {
      const text = await StopVoiceAndTranscribe();

      if (text && text.trim()) {
        transcript += (transcript ? '\n\n' : '') + text.trim();
        wordCount = transcript.trim().split(/\s+/).filter(w => w).length;
      } else {
        error = 'No speech detected. Try speaking louder or closer to the microphone.';
      }
    } catch (e) {
      error = `Transcription failed: ${e}`;
    }

    state = 'idle';
  }

  function clearTranscript() {
    transcript = '';
    wordCount = 0;
    sessionSeconds = 0;
    error = '';
    dispatch('clearView');
    if (textareaEl) textareaEl.focus();
  }

  async function copyTranscript() {
    if (!transcript) return;
    try {
      await navigator.clipboard.writeText(transcript);
      copyFeedback = true;
      setTimeout(() => (copyFeedback = false), 2000);
    } catch (_) {
      textareaEl?.select();
      document.execCommand('copy');
      copyFeedback = true;
      setTimeout(() => (copyFeedback = false), 2000);
    }
  }

  async function saveTranscript() {
    if (!transcript || isViewingHistory) return;
    dispatch('save', { text: transcript, duration: sessionSeconds });
    saveFeedback = true;
    setTimeout(() => (saveFeedback = false), 2000);
  }

  function formatTime(secs) {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return m > 0 ? `${m}m ${s.toString().padStart(2, '0')}s` : `${s}s`;
  }

  function handleTextareaInput() {
    wordCount = transcript.trim().split(/\s+/).filter(w => w).length;
  }

  function formatDate(ts) {
    if (!ts) return '';
    return new Date(ts).toLocaleString(undefined, {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }
</script>

<div class="flow-panel">
  <!-- Header bar when viewing a saved transcript -->
  {#if isViewingHistory}
    <div class="flow-view-header">
      <button class="back-btn" on:click={clearTranscript} title="Back to new recording">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 18 9 12 15 6"/>
        </svg>
        Back
      </button>
      <span class="view-date">{formatDate(viewingTranscript.timestamp)}</span>
    </div>
  {/if}

  <!-- Stats Bar -->
  <div class="flow-stats">
    <div class="stat-card">
      <div class="stat-icon">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{formatTime(sessionSeconds)}</span>
        <span class="stat-label">Session Time</span>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 20h9"/>
          <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{wordCount.toLocaleString()}</span>
        <span class="stat-label">Words</span>
      </div>
    </div>
  </div>

  <!-- Main Content -->
  <div class="flow-content">
    <!-- Transcript Area -->
    <div class="transcript-area" class:recording={state === 'recording'} class:transcribing={state === 'transcribing'}>
      {#if state === 'recording'}
        <div class="transcript-display">
          {#if transcript}
            <span class="committed-text">{transcript}</span>
            <div class="recording-indicator">
              <span class="rec-dot"></span>
              Recording... {formatTime(sessionSeconds)}
            </div>
          {:else}
            <div class="recording-indicator center">
              <span class="rec-dot"></span>
              Recording... speak now
            </div>
          {/if}
        </div>
      {:else if state === 'transcribing'}
        <div class="transcript-display">
          {#if transcript}
            <span class="committed-text">{transcript}</span>
          {/if}
          <div class="transcribing-indicator">
            <div class="spinner"></div>
            Transcribing...
          </div>
        </div>
      {:else}
        <textarea
          bind:this={textareaEl}
          bind:value={transcript}
          on:input={handleTextareaInput}
          placeholder="Click the microphone to start speaking"
          readonly={isViewingHistory}
        ></textarea>
      {/if}
    </div>

    {#if error}
      <div class="flow-error">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <circle cx="12" cy="12" r="10"/>
          <line x1="15" y1="9" x2="9" y2="15"/>
          <line x1="9" y1="9" x2="15" y2="15"/>
        </svg>
        {error}
      </div>
    {/if}

    <!-- Controls -->
    <div class="flow-controls">
      <div class="controls-left">
        <button
          class="btn-action"
          on:click={clearTranscript}
          disabled={(!transcript && state === 'idle') || state === 'transcribing'}
          title="Clear transcript"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="3 6 5 6 21 6"/>
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
          Clear
        </button>
        <button
          class="btn-action"
          on:click={copyTranscript}
          disabled={!transcript}
          title="Copy to clipboard"
        >
          {#if copyFeedback}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <path d="M20 6L9 17l-5-5"/>
            </svg>
            Copied!
          {:else}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
            </svg>
            Copy
          {/if}
        </button>
        {#if !isViewingHistory}
          <button
            class="btn-action btn-save"
            on:click={saveTranscript}
            disabled={!transcript || state !== 'idle'}
            title="Save transcript"
          >
            {#if saveFeedback}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                <path d="M20 6L9 17l-5-5"/>
              </svg>
              Saved!
            {:else}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/>
                <polyline points="17 21 17 13 7 13 7 21"/>
                <polyline points="7 3 7 8 15 8"/>
              </svg>
              Save
            {/if}
          </button>
        {/if}
      </div>

      <!-- Mic Button -->
      {#if !isViewingHistory}
        <button
          class="btn-mic"
          class:recording={state === 'recording'}
          class:transcribing={state === 'transcribing'}
          on:click={toggleRecording}
          disabled={state === 'transcribing'}
          title={state === 'recording' ? 'Stop recording & transcribe' : state === 'transcribing' ? 'Transcribing...' : 'Start recording'}
        >
          <div class="mic-ring" class:active={state === 'recording'}></div>
          <div class="mic-ring mic-ring-2" class:active={state === 'recording'}></div>
          <div class="mic-icon">
            {#if state === 'recording'}
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
                <rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor"/>
              </svg>
            {:else if state === 'transcribing'}
              <div class="mic-spinner"></div>
            {:else}
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
                <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                <line x1="12" y1="19" x2="12" y2="23"/>
                <line x1="8" y1="23" x2="16" y2="23"/>
              </svg>
            {/if}
          </div>
        </button>
      {/if}

      <div class="controls-right">
        <span class="status-label" class:active={state === 'recording'} class:busy={state === 'transcribing'}>
          {#if state === 'recording'}
            <span class="pulse-dot"></span>
            Recording...
          {:else if state === 'transcribing'}
            <span class="pulse-dot transcribing-dot"></span>
            Transcribing...
          {:else if isViewingHistory}
            Viewing saved
          {:else}
            Ready
          {/if}
        </span>
      </div>
    </div>
  </div>
</div>

<style>
  .flow-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  /* ─── View Header ─── */
  .flow-view-header {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 24px;
    max-width: 720px;
    margin: 0 auto;
    width: 100%;
  }

  .back-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 6px 12px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .back-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .view-date {
    font-size: 13px;
    color: var(--text-muted);
  }

  /* ─── Stats ─── */
  .flow-stats {
    display: flex;
    gap: 12px;
    padding: 20px 24px 0;
    max-width: 720px;
    margin: 0 auto;
    width: 100%;
  }

  .stat-card {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 16px 20px;
  }

  .stat-icon {
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(14, 240, 216, 0.1);
    border-radius: 10px;
    color: var(--accent);
    flex-shrink: 0;
  }

  .stat-info {
    display: flex;
    flex-direction: column;
  }

  .stat-value {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-primary);
    line-height: 1.2;
  }

  .stat-label {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  /* ─── Content ─── */
  .flow-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    max-width: 720px;
    margin: 0 auto;
    width: 100%;
    padding: 20px 24px 24px;
    overflow: hidden;
  }

  /* ─── Transcript ─── */
  .transcript-area {
    flex: 1;
    background: var(--bg-input);
    border: 1px solid var(--border);
    border-radius: 16px;
    overflow: hidden;
    transition: border-color 0.2s ease, box-shadow 0.2s ease;
  }

  .transcript-area:focus-within {
    border-color: var(--border-light);
    box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.03);
  }

  .transcript-area.recording {
    border-color: var(--danger);
    box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.15);
  }

  .transcript-area.transcribing {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow);
  }

  .transcript-area textarea {
    width: 100%;
    height: 100%;
    background: transparent;
    border: none;
    outline: none;
    color: var(--text-primary);
    font-family: var(--font-sans);
    font-size: 16px;
    line-height: 1.7;
    resize: none;
    padding: 20px;
  }

  .transcript-area textarea::placeholder {
    color: var(--text-muted);
  }

  .transcript-display {
    width: 100%;
    height: 100%;
    padding: 20px;
    font-family: var(--font-sans);
    font-size: 16px;
    line-height: 1.7;
    overflow-y: auto;
    min-height: 100px;
  }

  .committed-text {
    color: var(--text-primary);
    white-space: pre-wrap;
  }

  /* ─── Recording Indicator ─── */
  .recording-indicator {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--danger);
    font-size: 14px;
    font-weight: 500;
    margin-top: 16px;
    animation: fadeIn 0.3s ease;
  }

  .recording-indicator.center {
    margin-top: 0;
    justify-content: center;
    padding: 40px 0;
    font-size: 16px;
  }

  .rec-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--danger);
    animation: recPulse 1.2s ease-in-out infinite;
    flex-shrink: 0;
  }

  @keyframes recPulse {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.4; transform: scale(0.8); }
  }

  /* ─── Transcribing Indicator ─── */
  .transcribing-indicator {
    display: flex;
    align-items: center;
    gap: 10px;
    color: var(--accent);
    font-size: 14px;
    font-weight: 500;
    margin-top: 16px;
    animation: fadeIn 0.3s ease;
  }

  .spinner {
    width: 16px;
    height: 16px;
    border: 2px solid var(--border-light);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    flex-shrink: 0;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  /* ─── Error ─── */
  .flow-error {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    margin-top: 12px;
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.2);
    border-radius: 10px;
    color: #f87171;
    font-size: 13px;
  }

  /* ─── Controls ─── */
  .flow-controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 20px;
    gap: 16px;
  }

  .controls-left,
  .controls-right {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .controls-right {
    justify-content: flex-end;
  }

  .btn-action {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-action:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: var(--border-light);
    color: var(--text-primary);
  }

  .btn-action:disabled {
    opacity: 0.35;
    cursor: not-allowed;
  }

  .btn-save:not(:disabled) {
    border-color: var(--accent);
    color: var(--accent);
  }

  .btn-save:hover:not(:disabled) {
    background: rgba(14, 240, 216, 0.1);
    color: var(--accent);
  }

  /* ─── Mic Button ─── */
  .btn-mic {
    position: relative;
    width: 64px;
    height: 64px;
    background: var(--accent);
    border: none;
    border-radius: 50%;
    color: #000;
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1;
  }

  .btn-mic:hover:not(:disabled) {
    background: var(--accent-hover);
    transform: scale(1.05);
  }

  .btn-mic:disabled {
    cursor: not-allowed;
    opacity: 0.7;
  }

  .btn-mic.recording {
    background: var(--danger);
    animation: micPulse 2s ease-in-out infinite;
  }

  .btn-mic.recording:hover {
    background: #dc2626;
  }

  .btn-mic.transcribing {
    background: var(--accent);
    opacity: 0.8;
  }

  .mic-icon {
    position: relative;
    z-index: 2;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .mic-spinner {
    width: 22px;
    height: 22px;
    border: 2.5px solid rgba(0, 0, 0, 0.2);
    border-top-color: #000;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .mic-ring {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 64px;
    height: 64px;
    border-radius: 50%;
    transform: translate(-50%, -50%);
    border: 2px solid transparent;
    pointer-events: none;
    transition: opacity 0.3s ease;
    opacity: 0;
  }

  .mic-ring.active {
    opacity: 1;
    border-color: var(--danger);
    animation: ringPulse 2s ease-out infinite;
  }

  .mic-ring-2.active {
    animation: ringPulse 2s ease-out 0.6s infinite;
  }

  @keyframes ringPulse {
    0% { width: 64px; height: 64px; opacity: 0.6; }
    100% { width: 110px; height: 110px; opacity: 0; }
  }

  @keyframes micPulse {
    0%, 100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.4); }
    50% { box-shadow: 0 0 0 12px rgba(239, 68, 68, 0); }
  }

  /* ─── Status Label ─── */
  .status-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-muted);
    font-family: var(--font-sans);
  }

  .status-label.active {
    color: var(--danger);
  }

  .status-label.busy {
    color: var(--accent);
  }

  .pulse-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--danger);
    animation: dotPulse 1.5s ease-in-out infinite;
  }

  .pulse-dot.transcribing-dot {
    background: var(--accent);
  }

  @keyframes dotPulse {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.5; transform: scale(0.8); }
  }
</style>
