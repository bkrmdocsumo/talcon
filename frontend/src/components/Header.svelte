<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
  import { GetDictationStatus } from '../../wailsjs/go/main/App';

  export let activeTab = 'chat'; // 'chat' | 'agents' | 'flow'
  export let telegramStatus = 'stopped';
  export let telegramToggling = false;

  const dispatch = createEventDispatcher();

  // Dictation state
  let dictationState = 'idle'; // 'idle' | 'recording' | 'transcribing'
  let dictationEnabled = false;
  let cleanupDictStatus = null;
  let cleanupDictError = null;

  onMount(async () => {
    // Check initial dictation status.
    try {
      const status = await GetDictationStatus();
      dictationEnabled = status.enabled || false;
      dictationState = status.state || 'idle';
    } catch (_) {}

    // Listen for dictation state changes.
    cleanupDictStatus = EventsOn('dictation:status', (data) => {
      if (!data) return;
      dictationState = data.state || 'idle';
      dictationEnabled = true;
    });
    cleanupDictError = EventsOn('dictation:error', () => {
      dictationState = 'idle';
    });
  });

  onDestroy(() => {
    if (cleanupDictStatus) cleanupDictStatus();
    if (cleanupDictError) cleanupDictError();
  });

  function setTab(tab) {
    dispatch('tabChange', { tab });
  }
</script>

<header class="header">
  <!-- Left spacer for balance -->
  <div class="header-left">
    <!-- Placeholder for window controls area on macOS -->
  </div>

  <!-- Center tabs (Chat first, then Agents, then Flow) -->
  <nav class="header-tabs">
    <button
      class="tab"
      class:active={activeTab === 'chat'}
      on:click={() => setTab('chat')}
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="margin-right: 4px;">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
      </svg>
      Chat
    </button>
    <button
      class="tab"
      class:active={activeTab === 'agents'}
      on:click={() => setTab('agents')}
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="margin-right: 4px;">
        <circle cx="12" cy="8" r="5"/>
        <path d="M20 21a8 8 0 0 0-16 0"/>
        <path d="M12 2v1"/>
        <path d="M4.93 4.93l.7.7"/>
        <path d="M19.07 4.93l-.7.7"/>
      </svg>
      Agents
    </button>
    <button
      class="tab"
      class:active={activeTab === 'flow'}
      on:click={() => setTab('flow')}
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="margin-right: 4px;">
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
        <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
      </svg>
      Flow
    </button>
  </nav>

  <!-- Right section -->
  <div class="header-right">
    {#if dictationEnabled}
      <div
        class="dictation-indicator"
        class:dict-recording={dictationState === 'recording'}
        class:dict-transcribing={dictationState === 'transcribing'}
        title={dictationState === 'recording' ? 'Dictation: Recording...' : dictationState === 'transcribing' ? 'Dictation: Transcribing...' : 'Dictation Anywhere: Ready'}
      >
        {#if dictationState === 'recording'}
          <span class="dict-rec-dot"></span>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
            <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
          </svg>
        {:else if dictationState === 'transcribing'}
          <div class="dict-spinner"></div>
        {:else}
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" opacity="0.5">
            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
            <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
          </svg>
        {/if}
      </div>
    {/if}

    <button
      class="btn-telegram"
      class:tg-running={telegramStatus === 'running'}
      on:click={() => dispatch('toggleTelegram')}
      disabled={telegramToggling}
      title={telegramStatus === 'running' ? 'Telegram bot running — click to stop' : 'Telegram bot stopped — click to start'}
    >
      <span class="tg-dot" class:tg-dot-active={telegramStatus === 'running'}></span>
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none">
        <path d="M21.2 4.4L2.4 10.8c-.6.2-.6 1.1 0 1.3l4.5 1.7 1.7 5.5c.1.4.6.6.9.3l2.5-2 4.2 3.1c.5.3 1.1 0 1.2-.5L21.9 5.4c.2-.7-.4-1.2-1-.9z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </button>
  </div>
</header>

<style>
  .header {
    --wails-draggable: drag;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 16px;
    height: var(--header-height);
    background: var(--bg-primary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    user-select: none;
  }

  .header-left {
    width: 100px;
    flex-shrink: 0;
  }

  .header-right {
    --wails-draggable: no-drag;
    width: 100px;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
    flex-shrink: 0;
  }

  /* ─── Tabs ─── */
  .header-tabs {
    --wails-draggable: no-drag;
    display: flex;
    align-items: center;
    gap: 2px;
    background: var(--bg-tertiary);
    border-radius: 10px;
    padding: 3px;
  }

  .tab {
    display: flex;
    align-items: center;
    padding: 6px 16px;
    background: none;
    border: none;
    border-radius: 8px;
    color: var(--text-muted);
    font-size: 13px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .tab:hover {
    color: var(--text-secondary);
  }

  .tab.active {
    background: var(--bg-secondary);
    color: var(--text-primary);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  }

  /* ─── Telegram Button ─── */
  .btn-telegram {
    --wails-draggable: no-drag;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 5px;
    width: 32px;
    height: 32px;
    background: transparent;
    border: none;
    border-radius: 8px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
    position: relative;
  }

  .btn-telegram:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text-secondary);
  }

  .btn-telegram:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-telegram.tg-running {
    color: #4ade80;
  }

  .tg-dot {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: transparent;
    transition: background 0.2s ease;
  }

  .tg-dot-active {
    background: #4ade80;
    box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
  }

  /* ─── Dictation Indicator ─── */
  .dictation-indicator {
    --wails-draggable: no-drag;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: 8px;
    color: var(--text-muted);
    position: relative;
    transition: all 0.2s ease;
  }

  .dictation-indicator.dict-recording {
    color: var(--danger);
    background: rgba(239, 68, 68, 0.1);
    animation: dictPulse 2s ease-in-out infinite;
  }

  .dictation-indicator.dict-transcribing {
    color: var(--accent);
    background: rgba(14, 240, 216, 0.1);
  }

  .dict-rec-dot {
    position: absolute;
    top: 5px;
    right: 5px;
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--danger);
    animation: dotBlink 1s ease-in-out infinite;
  }

  .dict-spinner {
    width: 13px;
    height: 13px;
    border: 1.5px solid var(--border-light);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes dictPulse {
    0%, 100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.2); }
    50% { box-shadow: 0 0 0 6px rgba(239, 68, 68, 0); }
  }

  @keyframes dotBlink {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.3; }
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

</style>
