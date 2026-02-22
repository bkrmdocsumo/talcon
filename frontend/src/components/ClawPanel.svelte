<script>
  import { createEventDispatcher, tick } from 'svelte';

  export let events = [];
  export let config = {};
  export let stats = {};
  export let crons = [];
  export let activeSubTab = 'overview';
  export let focusedEventId = null;

  const dispatch = createEventDispatcher();

  let eventRefs = {};

  function openEvent(event) {
    if (!event.session_id || event.status === 'suppressed' || event.status === 'processing') return;
    dispatch('openEvent', event);
  }

  $: if (focusedEventId && activeSubTab === 'events') {
    tick().then(() => {
      const el = eventRefs[focusedEventId];
      if (el) {
        el.scrollIntoView({ behavior: 'smooth', block: 'center' });
        setTimeout(() => dispatch('clearFocusedEvent'), 2000);
      }
    });
  }

  function setSubTab(tab) {
    dispatch('subTabChange', { tab });
  }

  // ─── Local state ───
  let manualMessage = '';
  let manualEventType = 'message';
  let cronSchedule = '*/5 * * * *';
  let cronMessage = '';
  let cronError = '';

  // ─── Helpers ───

  const typeLabels = {
    message: 'Message',
    heartbeat: 'Heartbeat',
    cron: 'Cron',
    hook: 'Hook',
    webhook: 'Webhook',
  };

  const typeIcons = {
    message: 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z',
    heartbeat: 'M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78L12 21.23l8.84-8.84a5.5 5.5 0 0 0 0-7.78z',
    cron: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM12 6v6l4 2',
    hook: 'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71',
    webhook: 'M22 12h-4l-3 9L9 3l-3 9H2',
  };

  const statusColors = {
    pending: 'var(--text-muted)',
    processing: 'var(--accent)',
    completed: '#4ade80',
    suppressed: 'var(--text-muted)',
    failed: 'var(--danger)',
  };

  function formatTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    const now = new Date();
    const diff = now - d;
    if (diff < 60000) return 'just now';
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function handleToggleHeartbeat() {
    dispatch('configChange', {
      ...config,
      heartbeat_enabled: !config.heartbeat_enabled,
    });
  }

  function handleIntervalChange(e) {
    const mins = parseInt(e.target.value, 10);
    if (!mins || mins < 1) return;
    dispatch('configChange', {
      ...config,
      heartbeat_interval_minutes: mins,
    });
  }

  function handleFireHeartbeat() {
    dispatch('fireHeartbeat');
  }

  function handleSendManual() {
    if (!manualMessage.trim()) return;
    dispatch('pushEvent', { type: manualEventType, message: manualMessage.trim() });
    manualMessage = '';
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSendManual();
    }
  }

  function handleAddCron() {
    if (!cronSchedule.trim() || !cronMessage.trim()) return;
    cronError = '';
    dispatch('addCron', { schedule: cronSchedule.trim(), message: cronMessage.trim() });
    cronMessage = '';
  }

  function handleRemoveCron(id) {
    dispatch('removeCron', { id });
  }

  function handleCronKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleAddCron();
    }
  }

  const sourceTitleMap = {
    'lifecycle:startup': 'App Started',
    'lifecycle:shutdown': 'App Shutdown',
    'heartbeat-ticker': 'Heartbeat Check',
    'manual:ui': 'Manual Event',
  };

  function formatSource(source) {
    if (sourceTitleMap[source]) return sourceTitleMap[source];
    if (source === 'scheduler') return 'Scheduled Task';
    if (source && !source.includes(':')) {
      return source.charAt(0).toUpperCase() + source.slice(1);
    }
    return source || '';
  }

  function describeSchedule(expr) {
    const parts = expr.split(/\s+/);
    if (parts.length !== 5) return expr;
    const [min, hour, dom, mon, dow] = parts;

    if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
      return `Every ${min.slice(2)} minutes`;
    }
    if (min !== '*' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
      return `Every ${hour.slice(2)} hours at :${min.padStart(2, '0')}`;
    }
    if (min !== '*' && hour !== '*' && dom === '*' && mon === '*' && dow === '*') {
      return `Daily at ${hour}:${min.padStart(2, '0')}`;
    }
    return expr;
  }
</script>

<div class="claw-panel">
  <!-- Sub-tab Navigation -->
  <div class="sub-tabs">
    <button
      class="sub-tab"
      class:sub-tab-active={activeSubTab === 'overview'}
      on:click={() => setSubTab('overview')}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/>
      </svg>
      Overview
    </button>
    <button
      class="sub-tab"
      class:sub-tab-active={activeSubTab === 'crons'}
      on:click={() => setSubTab('crons')}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d={typeIcons.cron}/>
      </svg>
      Crons
      {#if crons.length > 0}
        <span class="sub-tab-badge">{crons.length}</span>
      {/if}
    </button>
    <button
      class="sub-tab"
      class:sub-tab-active={activeSubTab === 'events'}
      on:click={() => setSubTab('events')}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d={typeIcons.webhook}/>
      </svg>
      Events
      {#if events.length > 0}
        <span class="sub-tab-badge">{events.length}</span>
      {/if}
    </button>
  </div>

  <!-- Tab Content -->
  <div class="tab-content">
    {#if activeSubTab === 'overview'}
      <!-- Trigger Controls -->
      <div class="trigger-controls">
        <h3 class="section-title">Triggers</h3>
        <div class="trigger-grid">
          <div class="trigger-card heartbeat-card" class:trigger-active={config.heartbeat_enabled}>
            <button class="heartbeat-toggle" on:click={handleToggleHeartbeat}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d={typeIcons.heartbeat}/>
              </svg>
              <span class="trigger-name">Heartbeat</span>
              <span class="trigger-status">{config.heartbeat_enabled ? 'ON' : 'OFF'}</span>
            </button>
            {#if config.heartbeat_enabled}
              <div class="heartbeat-interval" on:click|stopPropagation>
                <span class="interval-label">Every</span>
                <select class="interval-select" value={config.heartbeat_interval_minutes || 1} on:change={handleIntervalChange}>
                  <option value={1}>1 min</option>
                  <option value={5}>5 min</option>
                  <option value={10}>10 min</option>
                  <option value={15}>15 min</option>
                  <option value={30}>30 min</option>
                  <option value={60}>1 hour</option>
                  <option value={120}>2 hours</option>
                </select>
              </div>
            {/if}
          </div>

          <button class="trigger-card trigger-active" disabled>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d={typeIcons.message}/>
            </svg>
            <span class="trigger-name">Messages</span>
            <span class="trigger-status">ON</span>
          </button>

          <button class="trigger-card trigger-active" disabled>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d={typeIcons.cron}/>
            </svg>
            <span class="trigger-name">Crons</span>
            <span class="trigger-status">ON</span>
          </button>

          <button class="trigger-card trigger-active" disabled>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d={typeIcons.hook}/>
            </svg>
            <span class="trigger-name">Hooks</span>
            <span class="trigger-status">ON</span>
          </button>

          <button class="trigger-card trigger-active" disabled>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d={typeIcons.webhook}/>
            </svg>
            <span class="trigger-name">Webhooks</span>
            <span class="trigger-status">ON</span>
          </button>
        </div>
      </div>

      <!-- Quick Actions -->
      <div class="quick-actions">
        <h3 class="section-title">Quick Actions</h3>
        <div class="action-row">
          <button class="action-btn" on:click={handleFireHeartbeat}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d={typeIcons.heartbeat}/>
            </svg>
            Fire Heartbeat
          </button>

          <div class="manual-push">
            <select bind:value={manualEventType} class="event-type-select">
              <option value="message">Message</option>
              <option value="hook">Hook</option>
              <option value="heartbeat">Heartbeat</option>
            </select>
            <input
              type="text"
              class="manual-input"
              placeholder="Push a manual event..."
              bind:value={manualMessage}
              on:keydown={handleKeydown}
            />
            <button class="action-btn send-btn" on:click={handleSendManual} disabled={!manualMessage.trim()}>
              Send
            </button>
          </div>
        </div>
      </div>

    {:else if activeSubTab === 'crons'}
      <!-- Cron Tasks -->
      <div class="cron-tasks">
        <div class="cron-form">
          <input
            type="text"
            class="cron-schedule-input"
            placeholder="*/5 * * * *"
            bind:value={cronSchedule}
            on:keydown={handleCronKeydown}
          />
          <input
            type="text"
            class="cron-message-input"
            placeholder="Message to send on schedule..."
            bind:value={cronMessage}
            on:keydown={handleCronKeydown}
          />
          <button class="action-btn cron-add-btn" on:click={handleAddCron} disabled={!cronSchedule.trim() || !cronMessage.trim()}>
            Add
          </button>
        </div>
        {#if cronError}
          <div class="cron-error">{cronError}</div>
        {/if}
        {#if crons.length > 0}
          <div class="cron-list">
            {#each crons as cron (cron.id)}
              <div class="cron-item">
                <div class="cron-item-info">
                  <span class="cron-item-schedule">{cron.schedule}</span>
                  <span class="cron-item-desc">{describeSchedule(cron.schedule)}</span>
                </div>
                <div class="cron-item-message">{cron.message}</div>
                <button class="cron-remove-btn" on:click={() => handleRemoveCron(cron.id)} title="Remove cron task">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                  </svg>
                </button>
              </div>
            {/each}
          </div>
        {:else}
          <div class="cron-empty">No cron tasks yet. Add one above to schedule periodic agent prompts.</div>
        {/if}
      </div>

    {:else if activeSubTab === 'events'}
      <!-- Event Feed -->
      <div class="event-feed">
        {#if events.length === 0}
          <div class="empty-feed">
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" opacity="0.3">
              <path d={typeIcons.webhook}/>
            </svg>
            <p>No events yet. The gateway is listening for triggers.</p>
          </div>
        {:else}
          <div class="event-list">
            {#each events as event (event.id)}
              <div
                class="event-item"
                class:event-suppressed={event.status === 'suppressed'}
                class:event-focused={focusedEventId === event.id}
                class:event-openable={event.session_id && event.status !== 'suppressed' && event.status !== 'processing'}
                bind:this={eventRefs[event.id]}
                on:click={() => openEvent(event)}
              >
                <div class="event-header">
                  <div class="event-type-badge" style="--badge-color: {statusColors[event.status] || 'var(--text-muted)'}">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                      <path d={typeIcons[event.type] || typeIcons.message}/>
                    </svg>
                    <span>{typeLabels[event.type] || event.type}</span>
                  </div>
                  <span class="event-source">{formatSource(event.source)}</span>
                  <span class="event-time">{formatTime(event.timestamp)}</span>
                  <span class="event-status-dot" style="background: {statusColors[event.status] || 'var(--text-muted)'}"></span>
                  {#if event.session_id && event.status !== 'suppressed' && event.status !== 'processing'}
                    <button class="event-open-btn" on:click|stopPropagation={() => openEvent(event)} title="Open conversation">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
                      </svg>
                    </button>
                  {/if}
                </div>

                {#if event.payload && event.status !== 'suppressed'}
                  <div class="event-payload">{event.payload.length > 120 ? event.payload.slice(0, 120) + '...' : event.payload}</div>
                {/if}

                {#if event.result && event.status === 'completed'}
                  <div class="event-result">{event.result}</div>
                {:else if event.status === 'suppressed'}
                  <div class="event-result event-ok">Heartbeat OK — no action needed</div>
                {:else if event.status === 'failed'}
                  <div class="event-result event-error">{event.result}</div>
                {:else if event.status === 'processing'}
                  <div class="event-result event-processing">Processing...</div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .claw-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    padding: 20px 24px;
    max-width: 800px;
    margin: 0 auto;
    width: 100%;
    gap: 20px;
  }

  .claw-panel::-webkit-scrollbar {
    width: 6px;
  }
  .claw-panel::-webkit-scrollbar-track {
    background: transparent;
  }
  .claw-panel::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 3px;
  }

  /* ─── Sub-tabs ─── */
  .sub-tabs {
    display: flex;
    gap: 2px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 3px;
  }

  .sub-tab {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    justify-content: center;
    padding: 8px 14px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .sub-tab:hover:not(.sub-tab-active) {
    color: var(--text-secondary);
    background: var(--bg-tertiary);
  }

  .sub-tab-active {
    background: var(--bg-tertiary);
    border-color: var(--border);
    color: var(--text-primary);
  }

  .sub-tab-active svg {
    color: var(--accent);
  }

  .sub-tab-badge {
    font-size: 10px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 10px;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }

  .tab-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 20px;
    min-height: 0;
  }

  /* ─── Section Title ─── */
  .section-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 10px;
  }

  /* ─── Trigger Controls ─── */
  .trigger-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
    gap: 8px;
  }

  .trigger-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 14px 12px;
    height: 120px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--text-muted);
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .trigger-card:hover:not(:disabled) {
    border-color: var(--border-light);
    background: var(--bg-tertiary);
  }

  .trigger-card:disabled {
    cursor: default;
  }

  .trigger-card.trigger-active {
    border-color: rgba(14, 240, 216, 0.2);
    color: var(--text-secondary);
  }

  .trigger-card.trigger-active svg {
    color: var(--accent);
  }

  .trigger-name {
    font-size: 12px;
    font-weight: 500;
  }

  .trigger-status {
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    opacity: 0.7;
  }

  .trigger-detail {
    font-size: 10px;
    color: var(--text-muted);
  }

  .heartbeat-card {
    cursor: default;
  }

  .heartbeat-toggle {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    width: 100%;
    background: none;
    border: none;
    color: inherit;
    font-family: var(--font-sans);
    cursor: pointer;
    padding: 0;
  }

  .heartbeat-interval {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-top: 4px;
    width: 100%;
    justify-content: center;
  }

  .interval-label {
    font-size: 10px;
    color: var(--text-muted);
  }

  .interval-select {
    padding: 2px 4px;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-secondary);
    font-size: 10px;
    font-family: var(--font-sans);
    cursor: pointer;
    outline: none;
  }

  .interval-select:focus {
    border-color: var(--accent);
  }

  /* ─── Cron Tasks ─── */
  .cron-form {
    display: flex;
    gap: 6px;
    align-items: center;
    margin-bottom: 10px;
  }

  .cron-schedule-input {
    width: 140px;
    padding: 8px 10px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 12px;
    font-family: var(--font-mono, monospace);
    outline: none;
    flex-shrink: 0;
  }

  .cron-schedule-input::placeholder {
    color: var(--text-muted);
  }

  .cron-schedule-input:focus {
    border-color: var(--accent);
  }

  .cron-message-input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 12px;
    font-family: var(--font-sans);
    outline: none;
  }

  .cron-message-input::placeholder {
    color: var(--text-muted);
  }

  .cron-message-input:focus {
    border-color: var(--accent);
  }

  .cron-add-btn {
    flex-shrink: 0;
  }

  .cron-error {
    font-size: 12px;
    color: var(--danger);
    margin-bottom: 8px;
  }

  .cron-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .cron-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  .cron-item:hover {
    border-color: var(--border-light);
  }

  .cron-item-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex-shrink: 0;
    min-width: 120px;
  }

  .cron-item-schedule {
    font-size: 12px;
    font-family: var(--font-mono, monospace);
    color: var(--accent);
    font-weight: 600;
  }

  .cron-item-desc {
    font-size: 10px;
    color: var(--text-muted);
  }

  .cron-item-message {
    flex: 1;
    font-size: 12px;
    color: var(--text-secondary);
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cron-remove-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    padding: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.15s ease;
  }

  .cron-remove-btn:hover {
    background: var(--bg-tertiary);
    border-color: var(--danger);
    color: var(--danger);
  }

  .cron-empty {
    font-size: 12px;
    color: var(--text-muted);
    padding: 12px 0;
    text-align: center;
  }

  /* ─── Quick Actions ─── */
  .action-row {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .action-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }

  .action-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: var(--border-light);
    color: var(--text-primary);
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .manual-push {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .event-type-select {
    padding: 8px 10px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 12px;
    font-family: var(--font-sans);
    cursor: pointer;
    outline: none;
    width: 100px;
    flex-shrink: 0;
  }

  .event-type-select:focus {
    border-color: var(--accent);
  }

  .manual-input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 12px;
    font-family: var(--font-sans);
    outline: none;
  }

  .manual-input::placeholder {
    color: var(--text-muted);
  }

  .manual-input:focus {
    border-color: var(--accent);
  }

  .send-btn {
    flex-shrink: 0;
  }

  /* ─── Event Feed ─── */
  .event-feed {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .empty-feed {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 48px 24px;
    color: var(--text-muted);
    text-align: center;
    font-size: 13px;
  }

  .event-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .event-item {
    padding: 12px 14px;
    background: var(--bg-secondary);
    border: 1px solid var(--border);
    border-radius: 10px;
    transition: all 0.15s ease;
  }

  .event-item:hover {
    border-color: var(--border-light);
  }

  .event-item.event-openable {
    cursor: pointer;
  }

  .event-item.event-suppressed {
    opacity: 0.5;
  }

  .event-item.event-focused {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent), 0 0 12px rgba(14, 240, 216, 0.15);
    animation: focus-fade 2s ease-out forwards;
  }

  @keyframes focus-fade {
    0% { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent), 0 0 12px rgba(14, 240, 216, 0.15); }
    70% { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent), 0 0 12px rgba(14, 240, 216, 0.15); }
    100% { border-color: var(--border); box-shadow: none; }
  }

  .event-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .event-type-badge {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: 6px;
    font-size: 11px;
    font-weight: 600;
    color: var(--badge-color);
    background: color-mix(in srgb, var(--badge-color) 12%, transparent);
  }

  .event-source {
    font-size: 11px;
    color: var(--text-muted);
    flex: 1;
  }

  .event-time {
    font-size: 11px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .event-status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .event-payload {
    margin-top: 6px;
    font-size: 12px;
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .event-result {
    margin-top: 6px;
    font-size: 12px;
    color: var(--text-primary);
    line-height: 1.5;
    padding: 8px 10px;
    background: var(--bg-tertiary);
    border-radius: 6px;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .event-ok {
    color: var(--text-muted);
    font-style: italic;
  }

  .event-error {
    color: var(--danger);
  }

  .event-processing {
    color: var(--accent);
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }

  /* ─── Open chat button ─── */
  .event-open-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    padding: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: all 0.15s ease;
  }

  .event-open-btn:hover {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    border-color: color-mix(in srgb, var(--accent) 25%, transparent);
    color: var(--accent);
  }
</style>
