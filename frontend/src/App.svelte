<script>
  import { onMount, onDestroy, afterUpdate, tick } from 'svelte';
  import { OpenFileInApp } from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime/runtime';

  // ─── Stores ───
  import {
    messages, loading, agentName, userName, ready, initError,
    activeTab, chatHistory, activeChatId, isFirstMessage,
    telegramStatus, telegramToggling, showSettings,
    applyStatus, checkBackendStatus, refreshChatHistory,
    sendMessage, cancelStream, newSession, selectChat, deleteChat,
    toggleTelegram, changeModel,
  } from './lib/stores/chatStore.js';

  import {
    agentPhase, agentTaskHistory, activeAgentTaskId,
    agentTaskTitle, agentMessages, agentProgressSteps,
    agentCreatedFiles, agentContextTools, agentLoading,
    agentIsStreaming,
    refreshAgentTaskHistory, startAgentTask, sendAgentFollowUp,
    cancelAgent, newAgentTask, selectAgentTask, deleteAgentTask,
  } from './lib/stores/agentStore.js';

  import {
    flowHistory, activeFlowId, viewingTranscript,
    refreshFlowHistory, selectFlow, deleteFlow, newFlow,
    saveFlow, clearFlowView,
  } from './lib/stores/flowStore.js';

  // ─── Components ───
  import Sidebar from './components/Sidebar.svelte';
  import Header from './components/Header.svelte';
  import ErrorBanner from './components/ErrorBanner.svelte';
  import WelcomeScreen from './components/WelcomeScreen.svelte';
  import ChatMessage from './components/ChatMessage.svelte';
  import ChatInput from './components/ChatInput.svelte';
  import SettingsModal from './components/SettingsModal.svelte';
  import FlowPanel from './components/FlowPanel.svelte';
  import AgentWelcome from './components/AgentWelcome.svelte';
  import AgentWorkspace from './components/AgentWorkspace.svelte';

  // ─── Local UI refs ───
  let chatContainer;
  let chatInputRef;
  let agentWelcomeRef;
  let userScrolledUp = false;

  // ─── Lifecycle ───
  let statusPollTimer = null;
  let flowSavedCleanup = null;

  onMount(async () => {
    statusPollTimer = await checkBackendStatus();
    await refreshFlowHistory();
    await refreshAgentTaskHistory();
    chatInputRef?.focus();

    // Listen for hotkey dictation auto-saves so we can refresh the flow sidebar.
    flowSavedCleanup = EventsOn('flow:saved', () => {
      refreshFlowHistory();
    });
  });

  onDestroy(() => {
    if (statusPollTimer) clearTimeout(statusPollTimer);
    if (flowSavedCleanup) flowSavedCleanup();
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
    await newSession();
    chatInputRef?.focus();
  }

  function handleSelectChat(e) {
    selectChat(e.detail.id);
  }

  function handleDeleteChat(e) {
    deleteChat(e.detail.id);
  }

  function handleToggleTelegram() {
    toggleTelegram();
  }

  function handleSettingsStatusUpdate(e) {
    applyStatus(e.detail);
  }

  function handleTabChange(e) {
    activeTab.set(e.detail.tab);
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
  }

  function handleSaveFlow(e) {
    saveFlow(e.detail.text, e.detail.duration);
  }

  function handleClearFlowView() {
    clearFlowView();
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
    on:newChat={handleNewSession}
    on:selectChat={handleSelectChat}
    on:deleteChat={handleDeleteChat}
    on:newFlow={handleNewFlow}
    on:selectFlow={handleSelectFlow}
    on:deleteFlow={handleDeleteFlow}
    on:newAgentTask={handleNewAgentTask}
    on:selectAgentTask={handleSelectAgentTask}
    on:deleteAgentTask={handleDeleteAgentTask}
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
      <FlowPanel
        viewingTranscript={$viewingTranscript}
        on:save={handleSaveFlow}
        on:clearView={handleClearFlowView}
      />
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
      <main class="chat-area" bind:this={chatContainer} on:scroll={handleChatScroll}>
        <div class="chat-container">
          {#if !$ready && $initError}
            <ErrorBanner message={$initError} />
          {/if}

          {#if $messages.length === 0}
            <WelcomeScreen userName={$userName} />
          {:else}
            {#each $messages as message (message)}
              <ChatMessage {message} agentName={$agentName} />
            {/each}
          {/if}
        </div>
      </main>

      <ChatInput
        bind:this={chatInputRef}
        disabled={$loading || !$ready}
        loading={$loading}
        on:send={handleSend}
        on:cancel={handleCancel}
        on:modelChange={handleModelChange}
      />
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
</style>
