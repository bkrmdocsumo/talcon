<script>
  import { onMount, onDestroy, afterUpdate, tick } from 'svelte';
  import { OpenFileInApp } from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime/runtime';

  // ─── Stores ───
  import {
    messages, loading, agentName, userName, ready, initError,
    activeTab, chatHistory, activeChatId, isFirstMessage,
    telegramStatus, telegramToggling, showSettings,
    chatCreatedFiles, configuredProviders,
    telegramChats, activeTelegramChatId, telegramMessages, viewingTelegram,
    backgroundStreamingSessions,
    applyStatus, checkBackendStatus, refreshChatHistory,
    sendMessage, cancelStream, newSession, selectChat, deleteChat,
    toggleTelegram, changeModel,
    refreshTelegramChats, selectTelegramChat, closeTelegramView, deleteTelegramChat,
  } from './lib/stores/chatStore.js';

  import {
    agentPhase, agentTaskHistory, activeAgentTaskId,
    agentTaskTitle, agentMessages, agentProgressSteps,
    agentCreatedFiles, agentContextTools, agentLoading,
    agentIsStreaming,
    backgroundAgentStreamingSessions,
    refreshAgentTaskHistory, startAgentTask, sendAgentFollowUp,
    cancelAgent, newAgentTask, selectAgentTask, deleteAgentTask,
  } from './lib/stores/agentStore.js';

  import {
    flowHistory, activeFlowId, viewingTranscript,
    refreshFlowHistory, selectFlow, deleteFlow, newFlow,
    saveFlow, clearFlowView,
  } from './lib/stores/flowStore.js';

  import {
    snippets, managingSnippets,
    refreshSnippets,
    showSnippetsPanel, hideSnippetsPanel,
  } from './lib/stores/snippetsStore.js';

  // ─── Components ───
  import Sidebar from './components/Sidebar.svelte';
  import Header from './components/Header.svelte';
  import ErrorBanner from './components/ErrorBanner.svelte';
  import WelcomeScreen from './components/WelcomeScreen.svelte';
  import ChatMessage from './components/ChatMessage.svelte';
  import ChatInput from './components/ChatInput.svelte';
  import SettingsModal from './components/SettingsModal.svelte';
  import FlowPanel from './components/FlowPanel.svelte';
  import SnippetsPanel from './components/SnippetsPanel.svelte';
  import AgentWelcome from './components/AgentWelcome.svelte';
  import AgentWorkspace from './components/AgentWorkspace.svelte';
  import AgentFileCard from './components/AgentFileCard.svelte';

  // ─── Local UI refs ───
  let chatContainer;
  let chatInputRef;
  let agentWelcomeRef;
  let userScrolledUp = false;

  // ─── Lifecycle ───
  let statusPollTimer = null;
  let flowSavedCleanup = null;
  let telegramActivityCleanup = null;

  onMount(async () => {
    statusPollTimer = await checkBackendStatus();
    await refreshFlowHistory();
    await refreshAgentTaskHistory();
    await refreshTelegramChats();
    await refreshSnippets();
    chatInputRef?.focus();

    // Listen for hotkey dictation auto-saves so we can refresh the flow sidebar.
    flowSavedCleanup = EventsOn('flow:saved', () => {
      refreshFlowHistory();
    });

    // Listen for Telegram message activity to refresh the Telegram chat list.
    telegramActivityCleanup = EventsOn('telegram:activity', (data) => {
      refreshTelegramChats();
      // If we're currently viewing this Telegram session, reload it.
      if ($viewingTelegram && data.session_id === $activeTelegramChatId) {
        selectTelegramChat(data.session_id);
      }
    });
  });

  onDestroy(() => {
    if (statusPollTimer) clearTimeout(statusPollTimer);
    if (flowSavedCleanup) flowSavedCleanup();
    if (telegramActivityCleanup) telegramActivityCleanup();
  });

  // ─── Scroll behaviour ───
  function handleChatScroll() {
    if (!chatContainer) return;
    const { scrollTop, scrollHeight, clientHeight } = chatContainer;
    userScrolledUp = scrollHeight - scrollTop - clientHeight > 150;
  }

  afterUpdate(() => {
    if ($loading && !userScrolledUp) {
      scrollToBottom();
    }
  });

  function scrollToBottom() {
    if (chatContainer) {
      chatContainer.scrollTo({
        top: chatContainer.scrollHeight,
        behavior: 'smooth',
      });
    }
  }

  // ─── Event handlers (thin wrappers that delegate to stores) ───

  async function handleSend(e) {
    const { text, files } = e.detail;
    userScrolledUp = false;
    chatInputRef?.clear();
    await sendMessage(text, files);
    tick().then(() => chatInputRef?.focus());
  }

  async function handleCancel() {
    await cancelStream();
    tick().then(() => chatInputRef?.focus());
  }

  async function handleNewSession() {
    closeTelegramView();
    await newSession();
    chatInputRef?.focus();
  }

  function handleSelectChat(e) {
    closeTelegramView();
    selectChat(e.detail.id);
  }

  function handleDeleteChat(e) {
    deleteChat(e.detail.id);
  }

  function handleToggleTelegram() {
    toggleTelegram().then(() => refreshTelegramChats());
  }

  function handleSelectTelegramChat(e) {
    selectTelegramChat(e.detail.id);
  }

  function handleDeleteTelegramChat(e) {
    deleteTelegramChat(e.detail.id);
  }

  function handleCloseTelegramView() {
    closeTelegramView();
  }

  function handleSettingsStatusUpdate(e) {
    applyStatus(e.detail);
  }

  function handleTabChange(e) {
    activeTab.set(e.detail.tab);
    // Reset snippets panel view when switching away from flow.
    if (e.detail.tab !== 'flow') {
      hideSnippetsPanel();
    }
  }

  function handleModelChange(e) {
    changeModel(e.detail.id, e.detail.label);
  }

  // ─── Flow event handlers ───

  function handleSelectFlow(e) {
    selectFlow(e.detail.id);
  }

  function handleDeleteFlow(e) {
    deleteFlow(e.detail.id);
  }

  function handleNewFlow() {
    newFlow();
    hideSnippetsPanel();
  }

  function handleSaveFlow(e) {
    saveFlow(e.detail.text, e.detail.duration);
  }

  function handleClearFlowView() {
    clearFlowView();
  }

  // ─── Snippet event handlers ───

  function handleManageSnippets() {
    showSnippetsPanel();
  }

  // ─── Agent event handlers ───

  function handleAgentWelcomeSend(e) {
    startAgentTask(e.detail.text, $ready);
  }

  function handleStartAgentTask(e) {
    startAgentTask(e.detail.text, $ready);
  }

  function handleAgentFollowUp(e) {
    sendAgentFollowUp(e.detail.text, $ready);
  }

  function handleAgentCancel() {
    cancelAgent();
  }

  function handleNewAgentTask() {
    newAgentTask();
  }

  function handleSelectAgentTask(e) {
    selectAgentTask(e.detail.id);
  }

  function handleDeleteAgentTask(e) {
    deleteAgentTask(e.detail.id);
  }

  async function handleOpenFile(e) {
    const { path } = e.detail;
    if (!path) return;
    try {
      await OpenFileInApp(path);
    } catch (err) {
      console.error('Failed to open file:', err);
    }
  }

  async function handleOpenFolder() {
    if ($activeAgentTaskId) {
      try {
        const baseDir = '~/.talon/agents/' + $activeAgentTaskId;
        await OpenFileInApp(baseDir);
      } catch (err) {
        console.error('Failed to open folder:', err);
      }
    }
  }
</script>

<div class="app">
  <Sidebar
    chatHistory={$chatHistory}
    activeChatId={$activeChatId}
    activeTab={$activeTab}
    flowHistory={$flowHistory}
    activeFlowId={$activeFlowId}
    agentTaskHistory={$agentTaskHistory}
    activeAgentTaskId={$activeAgentTaskId}
    telegramChats={$telegramChats}
    activeTelegramChatId={$activeTelegramChatId}
    telegramStatus={$telegramStatus}
    snippets={$snippets}
    managingSnippets={$managingSnippets}
    bgStreamingChats={$backgroundStreamingSessions}
    bgStreamingAgents={$backgroundAgentStreamingSessions}
    on:newChat={handleNewSession}
    on:selectChat={handleSelectChat}
    on:deleteChat={handleDeleteChat}
    on:selectTelegramChat={handleSelectTelegramChat}
    on:deleteTelegramChat={handleDeleteTelegramChat}
    on:newFlow={handleNewFlow}
    on:selectFlow={handleSelectFlow}
    on:deleteFlow={handleDeleteFlow}
    on:newAgentTask={handleNewAgentTask}
    on:selectAgentTask={handleSelectAgentTask}
    on:deleteAgentTask={handleDeleteAgentTask}
    on:manageSnippets={handleManageSnippets}
    on:openSettings={() => showSettings.set(true)}
  />

  <div class="main-panel">
    <Header
      activeTab={$activeTab}
      telegramStatus={$telegramStatus}
      telegramToggling={$telegramToggling}
      on:toggleTelegram={handleToggleTelegram}
      on:tabChange={handleTabChange}
    />

    {#if $activeTab === 'flow'}
      {#if $managingSnippets}
        <SnippetsPanel />
      {:else}
        <FlowPanel
          viewingTranscript={$viewingTranscript}
          on:save={handleSaveFlow}
          on:clearView={handleClearFlowView}
          on:openSettings={() => showSettings.set(true)}
        />
      {/if}
    {:else if $activeTab === 'agents'}
      {#if $agentPhase === 'welcome'}
        <main class="chat-area">
          <div class="chat-container">
            <AgentWelcome userName={$userName} />
          </div>
        </main>

        <ChatInput
          bind:this={agentWelcomeRef}
          disabled={$agentLoading || !$ready}
          loading={$agentLoading}
          configuredProviders={$configuredProviders}
          on:send={handleAgentWelcomeSend}
          on:cancel={handleAgentCancel}
          on:modelChange={handleModelChange}
        />
      {:else}
        <AgentWorkspace
          taskTitle={$agentTaskTitle}
          messages={$agentMessages}
          isStreaming={$agentIsStreaming}
          loading={$agentLoading}
          progressSteps={$agentProgressSteps}
          createdFiles={$agentCreatedFiles}
          contextTools={$agentContextTools}
          on:openFile={handleOpenFile}
          on:openFolder={handleOpenFolder}
          on:sendFollowUp={handleAgentFollowUp}
          on:cancel={handleAgentCancel}
        />
      {/if}
    {:else}
      {#if $viewingTelegram}
        <!-- Telegram read-only conversation view -->
        <main class="chat-area" bind:this={chatContainer} on:scroll={handleChatScroll}>
          <div class="chat-container">
            <div class="telegram-header-bar">
              <button class="telegram-back-btn" on:click={handleCloseTelegramView}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M19 12H5M12 19l-7-7 7-7"/>
                </svg>
                Back to chat
              </button>
              <div class="telegram-header-title">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M21.198 2.433a2.242 2.242 0 0 0-1.022.215l-16.5 7.5a2.25 2.25 0 0 0 .126 4.073l6.198 2.066 2.066 6.198a2.25 2.25 0 0 0 4.073.126l7.5-16.5a2.252 2.252 0 0 0-2.441-3.678z"/>
                </svg>
                <span>Telegram Conversation</span>
              </div>
            </div>

            {#if $telegramMessages.length === 0}
              <p class="telegram-empty">No messages in this conversation yet.</p>
            {:else}
              {#each $telegramMessages as message, i (i)}
                <ChatMessage {message} agentName={$agentName} />
              {/each}
            {/if}
          </div>
        </main>
      {:else}
        <main class="chat-area" bind:this={chatContainer} on:scroll={handleChatScroll}>
          <div class="chat-container">
            {#if !$ready && $initError}
              <ErrorBanner message={$initError} on:openSettings={() => showSettings.set(true)} />
            {/if}

            {#if $messages.length === 0}
              <WelcomeScreen userName={$userName} />
            {:else}
              {#each $messages as message, i (i)}
                <ChatMessage {message} agentName={$agentName} />
              {/each}

              {#if $chatCreatedFiles.length > 0 && !$loading}
                <div class="chat-file-cards">
                  {#each $chatCreatedFiles as file}
                    <AgentFileCard {file} on:openFile={handleOpenFile} />
                  {/each}
                </div>
              {/if}
            {/if}
          </div>
        </main>

        <ChatInput
          bind:this={chatInputRef}
          disabled={$loading || !$ready}
          loading={$loading}
          configuredProviders={$configuredProviders}
          on:send={handleSend}
          on:cancel={handleCancel}
          on:modelChange={handleModelChange}
        />
      {/if}
    {/if}
  </div>

  {#if $showSettings}
    <SettingsModal
      telegramStatus={$telegramStatus}
      on:close={() => showSettings.set(false)}
      on:statusUpdate={handleSettingsStatusUpdate}
    />
  {/if}
</div>

<style>
  .app {
    height: 100vh;
    display: flex;
    flex-direction: row;
    background: var(--bg-primary);
  }

  .main-panel {
    --wails-draggable: no-drag;
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow: hidden;
  }

  .chat-area {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }

  .chat-container {
    max-width: 720px;
    margin: 0 auto;
    padding: 16px 24px;
  }

  /* ─── Scrollbar ─── */
  .chat-area::-webkit-scrollbar {
    width: 6px;
  }

  .chat-area::-webkit-scrollbar-track {
    background: transparent;
  }

  .chat-area::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 3px;
  }

  .chat-area::-webkit-scrollbar-thumb:hover {
    background: var(--text-muted);
  }

  /* ─── Chat File Cards ─── */
  .chat-file-cards {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 12px;
    padding-bottom: 8px;
  }

  /* ─── Telegram View ─── */
  .telegram-header-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 0 16px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 8px;
  }

  .telegram-back-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 13px;
    font-family: var(--font-sans);
    padding: 6px 10px;
    border-radius: 8px;
    transition: all 0.15s ease;
  }

  .telegram-back-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .telegram-header-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    font-weight: 500;
    color: #29b6f6;
  }

  .telegram-empty {
    padding: 24px 0;
    text-align: center;
    color: var(--text-muted);
    font-size: 14px;
  }
</style>
