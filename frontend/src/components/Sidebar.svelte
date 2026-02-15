<script>
  import { createEventDispatcher } from 'svelte';

  export let chatHistory = []; // Array of { id, title, timestamp }
  export let activeChatId = null;
  export let activeTab = 'chat'; // 'chat' | 'agents' | 'flow'
  export let flowHistory = []; // Array of { id, title, duration, wordCount, timestamp }
  export let activeFlowId = null;
  export let agentTaskHistory = []; // Array of { id, title, timestamp }
  export let activeAgentTaskId = null;

  const dispatch = createEventDispatcher();

  let searchQuery = '';
  let hoveredChatId = null;
  let hoveredFlowId = null;

  // Clear search when switching tabs.
  $: activeTab, searchQuery = '';

  // ─── Chat filtering ───
  $: filteredHistory = searchQuery
    ? chatHistory.filter(c => c.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : chatHistory;

  // ─── Flow filtering ───
  $: filteredFlowHistory = searchQuery
    ? flowHistory.filter(f => f.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : flowHistory;

  // ─── Chat handlers ───
  function handleNewChat() {
    dispatch('newChat');
  }

  function handleSelectChat(chatId) {
    dispatch('selectChat', { id: chatId });
  }

  function handleDeleteChat(e, chatId) {
    e.stopPropagation();
    dispatch('deleteChat', { id: chatId });
  }

  // ─── Flow handlers ───
  function handleNewFlow() {
    dispatch('newFlow');
  }

  function handleSelectFlow(flowId) {
    dispatch('selectFlow', { id: flowId });
  }

  function handleDeleteFlow(e, flowId) {
    e.stopPropagation();
    dispatch('deleteFlow', { id: flowId });
  }

  // ─── Agent task handlers ───
  let hoveredAgentTaskId = null;

  function handleNewAgentTask() {
    dispatch('newAgentTask');
  }

  function handleSelectAgentTask(taskId) {
    dispatch('selectAgentTask', { id: taskId });
  }

  function handleDeleteAgentTask(e, taskId) {
    e.stopPropagation();
    dispatch('deleteAgentTask', { id: taskId });
  }

  // ─── Time formatting ───
  function formatTimestamp(ts) {
    if (!ts) return '';
    const date = new Date(ts);
    const now = new Date();
    const diffMs = now - date;
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) return 'Today';
    if (diffDays === 1) return 'Yesterday';
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function formatDuration(secs) {
    if (!secs) return '';
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return m > 0 ? `${m}m ${s.toString().padStart(2, '0')}s` : `${s}s`;
  }

  // ─── Agent task filtering ───
  $: filteredAgentTasks = searchQuery
    ? agentTaskHistory.filter(t => t.title.toLowerCase().includes(searchQuery.toLowerCase()))
    : agentTaskHistory;

  // Group items by time period.
  $: groupedChats = groupByTime(filteredHistory);
  $: groupedFlows = groupByTime(filteredFlowHistory);
  $: groupedAgentTasks = groupByTime(filteredAgentTasks);

  function groupByTime(items) {
    const groups = [];
    let currentLabel = '';
    for (const item of items) {
      const label = formatTimestamp(item.timestamp);
      if (label !== currentLabel) {
        currentLabel = label;
        groups.push({ label, items: [] });
      }
      groups[groups.length - 1].items.push(item);
    }
    return groups;
  }
</script>

<aside class="sidebar">
  <div class="sidebar-drag-region"></div>
  <div class="sidebar-inner">
    <!-- ─── Chat Tab Nav ─── -->
    {#if activeTab === 'chat'}
      <button class="nav-item nav-new-chat" on:click={handleNewChat}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M12 5v14M5 12h14" />
        </svg>
        <span>New chat</span>
      </button>

      <div class="search-wrapper">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          class="search-input"
          type="text"
          placeholder="Search chats"
          bind:value={searchQuery}
        />
      </div>

      <div class="nav-divider"></div>

      <div class="chat-history">
        {#if filteredHistory.length === 0}
          <p class="empty-history">
            {searchQuery ? 'No matching chats' : 'Your chats will show up here'}
          </p>
        {:else}
          {#each groupedChats as group}
            <div class="history-group">
              <div class="history-group-label">{group.label}</div>
              {#each group.items as chat (chat.id)}
                <button
                  class="history-item"
                  class:active={chat.id === activeChatId}
                  on:click={() => handleSelectChat(chat.id)}
                  on:mouseenter={() => (hoveredChatId = chat.id)}
                  on:mouseleave={() => (hoveredChatId = null)}
                  title={chat.title}
                >
                  <span class="history-title">{chat.title}</span>
                  {#if hoveredChatId === chat.id}
                    <button
                      class="delete-btn"
                      on:click={(e) => handleDeleteChat(e, chat.id)}
                      title="Delete chat"
                    >
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                        <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
                      </svg>
                    </button>
                  {/if}
                </button>
              {/each}
            </div>
          {/each}
        {/if}
      </div>

    <!-- ─── Agents Tab Nav ─── -->
    {:else if activeTab === 'agents'}
      <button class="nav-item nav-new-chat" on:click={handleNewAgentTask}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <path d="M12 5v14M5 12h14" />
        </svg>
        <span>New task</span>
      </button>

      <div class="search-wrapper">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          class="search-input"
          type="text"
          placeholder="Search tasks"
          bind:value={searchQuery}
        />
      </div>

      <div class="nav-divider"></div>

      <div class="chat-history">
        {#if filteredAgentTasks.length === 0}
          <p class="empty-history">
            {searchQuery ? 'No matching tasks' : 'Your tasks will show up here'}
          </p>
        {:else}
          {#each groupedAgentTasks as group}
            <div class="history-group">
              <div class="history-group-label">{group.label}</div>
              {#each group.items as task (task.id)}
                <button
                  class="history-item"
                  class:active={task.id === activeAgentTaskId}
                  on:click={() => handleSelectAgentTask(task.id)}
                  on:mouseenter={() => (hoveredAgentTaskId = task.id)}
                  on:mouseleave={() => (hoveredAgentTaskId = null)}
                  title={task.title}
                >
                  <span class="history-title">{task.title}</span>
                  {#if hoveredAgentTaskId === task.id}
                    <button
                      class="delete-btn"
                      on:click={(e) => handleDeleteAgentTask(e, task.id)}
                      title="Delete task"
                    >
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                        <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
                      </svg>
                    </button>
                  {/if}
                </button>
              {/each}
            </div>
          {/each}
        {/if}
      </div>

    <!-- ─── Flow Tab Nav ─── -->
    {:else if activeTab === 'flow'}
      <button class="nav-item nav-new-chat" on:click={handleNewFlow}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
          <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
        </svg>
        <span>New recording</span>
      </button>

      <div class="search-wrapper">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          class="search-input"
          type="text"
          placeholder="Search recordings"
          bind:value={searchQuery}
        />
      </div>

      <div class="nav-divider"></div>

      <div class="chat-history">
        {#if filteredFlowHistory.length === 0}
          <p class="empty-history">
            {searchQuery ? 'No matching recordings' : 'Your recordings will show up here'}
          </p>
        {:else}
          {#each groupedFlows as group}
            <div class="history-group">
              <div class="history-group-label">{group.label}</div>
              {#each group.items as flow (flow.id)}
                <button
                  class="history-item"
                  class:active={flow.id === activeFlowId}
                  on:click={() => handleSelectFlow(flow.id)}
                  on:mouseenter={() => (hoveredFlowId = flow.id)}
                  on:mouseleave={() => (hoveredFlowId = null)}
                  title={flow.title}
                >
                  <div class="flow-item-content">
                    <span class="history-title">{flow.title}</span>
                    <span class="flow-meta">
                      {flow.wordCount} words · {formatDuration(flow.duration)}
                    </span>
                  </div>
                  {#if hoveredFlowId === flow.id}
                    <button
                      class="delete-btn"
                      on:click={(e) => handleDeleteFlow(e, flow.id)}
                      title="Delete recording"
                    >
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                        <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
                      </svg>
                    </button>
                  {/if}
                </button>
              {/each}
            </div>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  <!-- Sidebar Footer -->
  <div class="sidebar-footer">
    <button class="footer-btn" title="Account">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
        <circle cx="12" cy="7" r="4" />
      </svg>
    </button>
    <button class="footer-btn" on:click={() => dispatch('openSettings')} title="Settings">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09a1.65 1.65 0 00-1-1.51 1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09a1.65 1.65 0 001.51-1 1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
      </svg>
    </button>
  </div>
</aside>

<style>
  .sidebar {
    width: var(--sidebar-width);
    height: 100%;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    user-select: none;
  }

  .sidebar-drag-region {
    --wails-draggable: drag;
    height: 38px;
    flex-shrink: 0;
  }

  .sidebar-inner {
    display: flex;
    flex-direction: column;
    flex: 1;
    padding: 4px 10px 12px;
    overflow-y: auto;
    min-height: 0;
  }

  .sidebar-inner::-webkit-scrollbar {
    width: 4px;
  }

  .sidebar-inner::-webkit-scrollbar-track {
    background: transparent;
  }

  .sidebar-inner::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 2px;
  }

  /* ─── Section Header ─── */
  .section-header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    color: var(--text-primary);
    font-size: 13.5px;
    font-weight: 500;
    font-family: var(--font-sans);
  }

  /* ─── Nav Items ─── */
  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 12px;
    background: none;
    border: none;
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 13.5px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    text-align: left;
  }

  .nav-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .nav-new-chat {
    color: var(--text-primary);
    font-weight: 500;
    margin-bottom: 2px;
  }

  /* ─── Search ─── */
  .search-wrapper {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    border-radius: 8px;
    transition: all 0.15s ease;
  }

  .search-wrapper:focus-within {
    background: var(--bg-hover);
  }

  .search-icon {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .search-input {
    flex: 1;
    background: none;
    border: none;
    outline: none;
    color: var(--text-secondary);
    font-size: 13.5px;
    font-family: var(--font-sans);
    padding: 4px 0;
  }

  .search-input::placeholder {
    color: var(--text-muted);
  }

  .nav-divider {
    height: 1px;
    background: var(--border);
    margin: 8px 12px;
  }

  /* ─── Chat / Flow History ─── */
  .chat-history {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-height: 0;
    overflow-y: auto;
  }

  .chat-history::-webkit-scrollbar {
    width: 4px;
  }

  .chat-history::-webkit-scrollbar-track {
    background: transparent;
  }

  .chat-history::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 2px;
  }

  .empty-history {
    padding: 12px;
    font-size: 13px;
    color: var(--text-muted);
    line-height: 1.4;
  }

  .history-group {
    margin-bottom: 4px;
  }

  .history-group-label {
    padding: 8px 12px 4px;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .history-item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 8px 12px;
    background: none;
    border: none;
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s ease;
    text-align: left;
    position: relative;
  }

  .history-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .history-item.active {
    background: rgba(255, 255, 255, 0.08);
    color: var(--text-primary);
  }

  .history-title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  /* ─── Flow Item ─── */
  .flow-item-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .flow-meta {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .delete-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    background: none;
    border: none;
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
    margin-left: 4px;
  }

  .delete-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ef4444;
  }

  /* ─── Sidebar Footer ─── */
  .sidebar-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-top: 1px solid var(--border);
    flex-shrink: 0;
  }

  .footer-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    background: transparent;
    border: none;
    border-radius: 8px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .footer-btn:hover {
    background: var(--bg-hover);
    color: var(--text-secondary);
  }
</style>
