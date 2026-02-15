<script>
  import { onMount, onDestroy, afterUpdate, tick } from 'svelte';
  import { SendMessageStream, SendMessageStreamWithFiles, GetStatus, NewSession, GetSessionID, ListSessions, LoadSession, DeleteSession, ToggleTelegram, CancelStream, ListFlowTranscripts, LoadFlowTranscript, SaveFlowTranscript, DeleteFlowTranscript } from '../wailsjs/go/main/App';
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

  import Sidebar from './components/Sidebar.svelte';
  import Header from './components/Header.svelte';
  import ErrorBanner from './components/ErrorBanner.svelte';
  import WelcomeScreen from './components/WelcomeScreen.svelte';
  import ChatMessage from './components/ChatMessage.svelte';
  import ChatInput from './components/ChatInput.svelte';
  import SettingsModal from './components/SettingsModal.svelte';
  import FlowPanel from './components/FlowPanel.svelte';

  // ─── App state ───
  let messages = [];
  let loading = false;
  let agentName = 'Talon';
  let ready = false;
  let initError = '';
  let chatContainer;
  let chatInputRef;

  // Streaming state
  let streamingIndex = -1;
  let currentThinkingIdx = -1;
  let streamCleanup = null;

  // Telegram
  let telegramStatus = 'stopped';
  let telegramToggling = false;

  // Settings modal
  let showSettings = false;

  // Active tab
  let activeTab = 'chat';

  // Chat history
  let chatHistory = [];
  let activeChatId = null;
  let isFirstMessage = true; // Track whether this is the first message in a new chat

  // Flow history
  let flowHistory = [];
  let activeFlowId = null;
  let viewingTranscript = null;

  // ─── Lifecycle ───
  let statusPollTimer = null;
  let flowSavedCleanup = null;

  onMount(async () => {
    await checkBackendStatus();
    chatInputRef?.focus();

    // Listen for hotkey dictation auto-saves so we can refresh the flow sidebar.
    flowSavedCleanup = EventsOn('flow:saved', () => {
      refreshFlowHistory();
    });
  });

  async function checkBackendStatus() {
    try {
      const status = await GetStatus();
      applyStatus(status);

      if (!status.ready) {
        // Backend is still starting up — poll until it's ready.
        console.log('Backend not ready yet, retrying in 500ms...');
        statusPollTimer = setTimeout(checkBackendStatus, 500);
        return;
      }

      // Backend is ready — load session data.
      activeChatId = await GetSessionID();
      await refreshChatHistory();
      await refreshFlowHistory();
    } catch (e) {
      console.error('Status check failed:', e);
      // Backend might not be up yet — retry.
      statusPollTimer = setTimeout(checkBackendStatus, 500);
    }
  }

  onDestroy(() => {
    if (statusPollTimer) clearTimeout(statusPollTimer);
    if (flowSavedCleanup) flowSavedCleanup();
  });

  afterUpdate(() => {
    scrollToBottom();
  });

  // ─── Helpers ───
  function applyStatus(status) {
    ready = status.ready;
    agentName = status.agentName || 'Talon';
    telegramStatus = status.telegramStatus || 'stopped';
    if (!ready && status.error) {
      initError = status.error;
    } else {
      initError = '';
    }
  }

  function scrollToBottom() {
    if (chatContainer) {
      chatContainer.scrollTo({
        top: chatContainer.scrollHeight,
        behavior: 'smooth',
      });
    }
  }

  async function refreshChatHistory() {
    try {
      const sessions = await ListSessions();
      chatHistory = (sessions || []).map(s => ({
        id: s.id,
        title: s.title || 'Untitled chat',
        timestamp: s.timestamp,
      }));
    } catch (e) {
      console.error('Failed to load chat history:', e);
    }
  }

  // ─── Flow history ───
  async function refreshFlowHistory() {
    try {
      const transcripts = await ListFlowTranscripts();
      flowHistory = (transcripts || []).map(t => ({
        id: t.id,
        title: t.title || 'Untitled recording',
        duration: t.duration || 0,
        wordCount: t.wordCount || 0,
        timestamp: t.timestamp,
      }));
    } catch (e) {
      console.error('Failed to load flow history:', e);
    }
  }

  async function handleSelectFlow(e) {
    const flowId = e.detail.id;
    if (flowId === activeFlowId && viewingTranscript) return;

    try {
      const t = await LoadFlowTranscript(flowId);
      activeFlowId = flowId;
      viewingTranscript = {
        id: t.id,
        text: t.text,
        duration: t.duration,
        wordCount: t.wordCount,
        timestamp: t.timestamp,
      };
    } catch (e) {
      console.error('Failed to load flow transcript:', e);
    }
  }

  async function handleDeleteFlow(e) {
    const flowId = e.detail.id;
    try {
      await DeleteFlowTranscript(flowId);
      if (flowId === activeFlowId) {
        activeFlowId = null;
        viewingTranscript = null;
      }
      await refreshFlowHistory();
    } catch (e) {
      console.error('Failed to delete flow transcript:', e);
    }
  }

  function handleNewFlow() {
    activeFlowId = null;
    viewingTranscript = null;
  }

  async function handleSaveFlow(e) {
    const { text, duration } = e.detail;
    try {
      await SaveFlowTranscript(text, duration);
      await refreshFlowHistory();
    } catch (e) {
      console.error('Failed to save flow transcript:', e);
    }
  }

  function handleClearFlowView() {
    activeFlowId = null;
    viewingTranscript = null;
  }

  // ─── Streaming event handler ───
  function handleStreamEvent(data) {
    if (streamingIndex < 0) return;

    switch (data.type) {
      case 'thinking_start':
        currentThinkingIdx = messages[streamingIndex].steps.length;
        messages[streamingIndex].steps = [
          ...messages[streamingIndex].steps,
          { type: 'thinking', content: '' },
        ];
        messages = messages;
        break;

      case 'thinking':
        if (currentThinkingIdx >= 0 && messages[streamingIndex].steps[currentThinkingIdx]) {
          messages[streamingIndex].steps[currentThinkingIdx].content += data.content;
          messages = messages;
        }
        break;

      case 'text':
        messages[streamingIndex].content += data.content;
        messages = messages;
        break;

      case 'tool_call':
        currentThinkingIdx = -1;
        messages[streamingIndex].steps = [
          ...messages[streamingIndex].steps,
          { type: 'tool_call', tool_name: data.tool_name, tool_input: data.tool_input },
        ];
        messages = messages;
        break;

      case 'tool_result':
        messages[streamingIndex].steps = [
          ...messages[streamingIndex].steps,
          { type: 'tool_result', tool_name: data.tool_name, content: data.content },
        ];
        messages = messages;
        break;

      case 'done':
        messages[streamingIndex] = {
          role: 'assistant',
          content: data.final_text,
          steps: data.steps || [],
        };
        messages = messages;
        finishStream();
        break;

      case 'error':
        messages[streamingIndex] = {
          role: 'assistant',
          content: `Something went wrong: ${data.error}`,
          isError: true,
        };
        messages = messages;
        finishStream();
        break;
    }
  }

  function finishStream() {
    if (streamCleanup) {
      streamCleanup();
      streamCleanup = null;
    }
    streamingIndex = -1;
    currentThinkingIdx = -1;
    loading = false;
    tick().then(() => chatInputRef?.focus());

    // Refresh sidebar after message completes (adds new chats, updates titles).
    refreshChatHistory();
  }

  // ─── Event handlers ───
  async function handleSend(e) {
    const { text, files } = e.detail;
    if ((!text && (!files || files.length === 0)) || loading) return;

    // Build the user message for display (with optional file metadata).
    const userMessage = {
      role: 'user',
      content: text || '',
      files: files && files.length > 0
        ? files.map(f => ({ name: f.name, type: f.type, size: f.size, dataUrl: f.dataUrl }))
        : undefined,
    };

    // Add user message and a streaming placeholder for the assistant response.
    messages = [...messages, userMessage, {
      role: 'assistant',
      content: '',
      steps: [],
      isStreaming: true,
    }];
    streamingIndex = messages.length - 1;
    currentThinkingIdx = -1;
    loading = true;
    chatInputRef?.clear();

    // Register event listener BEFORE starting the stream.
    streamCleanup = EventsOn('stream:event', handleStreamEvent);

    try {
      if (files && files.length > 0) {
        const attachments = files.map(f => ({
          name: f.name,
          mime_type: f.type,
          data: f.data,
        }));
        await SendMessageStreamWithFiles(text || '', attachments);
      } else {
        await SendMessageStream(text);
      }
    } catch (err) {
      // Error starting the stream.
      if (streamingIndex >= 0) {
        messages[streamingIndex] = {
          role: 'assistant',
          content: `Something went wrong: ${err}`,
          isError: true,
        };
        messages = messages;
      }
      finishStream();
    }
  }

  async function handleCancel() {
    try {
      await CancelStream();
    } catch (err) {
      console.error('Cancel failed:', err);
    }
    // Update the streaming message to show it was cancelled.
    if (streamingIndex >= 0) {
      messages[streamingIndex] = {
        role: 'assistant',
        content: messages[streamingIndex].content || '_Request cancelled._',
        steps: messages[streamingIndex].steps || [],
        isError: !messages[streamingIndex].content,
      };
      messages = messages;
    }
    finishStream();
  }

  async function handleNewSession() {
    if (loading) return;
    const newId = await NewSession();
    activeChatId = newId;
    messages = [];
    isFirstMessage = true;
    chatInputRef?.focus();
  }

  async function handleSelectChat(e) {
    if (loading) return;
    const sessionId = e.detail.id;
    if (sessionId === activeChatId && messages.length > 0) return; // Already viewing this chat

    try {
      const loaded = await LoadSession(sessionId);
      activeChatId = sessionId;
      isFirstMessage = false;

      // Convert loaded messages to frontend format.
      messages = (loaded || []).map(m => ({
        role: m.role,
        content: m.content,
        steps: m.steps || [],
      }));
    } catch (e) {
      console.error('Failed to load session:', e);
    }
  }

  async function handleDeleteChat(e) {
    const sessionId = e.detail.id;
    try {
      await DeleteSession(sessionId);

      // If we deleted the active chat, start a new one.
      if (sessionId === activeChatId) {
        await handleNewSession();
      }

      // Refresh the sidebar.
      await refreshChatHistory();
    } catch (e) {
      console.error('Failed to delete session:', e);
    }
  }

  async function handleToggleTelegram() {
    telegramToggling = true;
    try {
      const newStatus = await ToggleTelegram();
      telegramStatus = newStatus;
    } catch (e) {
      telegramStatus = 'error';
      console.error('Telegram toggle failed:', e);
    } finally {
      telegramToggling = false;
    }
  }

  function handleSettingsStatusUpdate(e) {
    applyStatus(e.detail);
  }

  function handleTabChange(e) {
    activeTab = e.detail.tab;
  }
</script>

<div class="app">
  <Sidebar
    {chatHistory}
    {activeChatId}
    {activeTab}
    {flowHistory}
    {activeFlowId}
    on:newChat={handleNewSession}
    on:selectChat={handleSelectChat}
    on:deleteChat={handleDeleteChat}
    on:newFlow={handleNewFlow}
    on:selectFlow={handleSelectFlow}
    on:deleteFlow={handleDeleteFlow}
    on:openSettings={() => (showSettings = true)}
  />

  <div class="main-panel">
    <Header
      {agentName}
      {activeTab}
      {telegramStatus}
      {telegramToggling}
      on:toggleTelegram={handleToggleTelegram}
      on:tabChange={handleTabChange}
    />

    {#if activeTab === 'flow'}
      <FlowPanel
        {viewingTranscript}
        on:save={handleSaveFlow}
        on:clearView={handleClearFlowView}
      />
    {:else}
      <main class="chat-area" bind:this={chatContainer}>
        <div class="chat-container">
          {#if !ready && initError}
            <ErrorBanner message={initError} />
          {/if}

          {#if messages.length === 0}
            <WelcomeScreen {agentName} />
          {:else}
            {#each messages as message (message)}
              <ChatMessage {message} {agentName} />
            {/each}
          {/if}
        </div>
      </main>

      <ChatInput
        bind:this={chatInputRef}
        disabled={loading || !ready}
        {loading}
        {agentName}
        on:send={handleSend}
        on:cancel={handleCancel}
      />
    {/if}
  </div>

  {#if showSettings}
    <SettingsModal
      {telegramStatus}
      on:close={() => (showSettings = false)}
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
