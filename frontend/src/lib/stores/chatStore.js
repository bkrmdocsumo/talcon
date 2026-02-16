import { writable, get } from 'svelte/store';
import {
  SendMessageStream,
  SendMessageStreamWithFiles,
  GetStatus,
  NewSession,
  ListSessions,
  LoadSession,
  DeleteSession,
  CancelStream,
  ChangeModel,
  ToggleTelegram,
  ListChatFiles,
  ListTelegramSessions,
  LoadTelegramSession,
} from '../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';

// ─── Core chat state ───
export const messages = writable([]);
export const loading = writable(false);
export const agentName = writable('Talon');
export const userName = writable('');
export const ready = writable(false);
export const initError = writable('');
export const activeTab = writable('chat');

// Streaming state
export const streamingIndex = writable(-1);
export const currentThinkingIdx = writable(-1);

// Telegram state
export const telegramStatus = writable('stopped');
export const telegramToggling = writable(false);

// Settings modal
export const showSettings = writable(false);

// Chat history & session tracking
export const chatHistory = writable([]);
export const activeChatId = writable(null);
export const isFirstMessage = writable(true);

// Files produced during the chat session
export const chatCreatedFiles = writable([]);

// Configured providers (which API keys are set)
export const configuredProviders = writable([]);

// Telegram chat state
export const telegramChats = writable([]);
export const activeTelegramChatId = writable(null);
export const telegramMessages = writable([]);
export const viewingTelegram = writable(false);

// Internal — stream cleanup function reference
let _streamCleanup = null;

// Sequence-based dedup: the backend attaches a monotonically increasing `seq`
// number to each event. If the Wails macOS WebKit bridge fires the same event
// twice, the duplicate will carry the same `seq` and be dropped.
let _lastSeenSeq = 0;

// ─── Background stream support ───
// Caches state for sessions whose streams are still running but the user has
// navigated away from. Events keep updating the cache; when the user returns,
// the cached state is restored into the main stores.
const _bgChatStreams = new Map();
// Map<sessionId, { messages, streamingIndex, currentThinkingIdx, lastSeenSeq, createdFiles }>

// Exported store: set of session IDs that are streaming in the background.
// The sidebar uses this to show a subtle activity indicator.
export const backgroundStreamingSessions = writable(new Set());

// Whether the global chat event listener is currently registered.
let _chatListenerRegistered = false;

function ensureChatListener() {
  if (!_chatListenerRegistered) {
    EventsOff('chat:stream:event');
    if (_streamCleanup) { _streamCleanup(); _streamCleanup = null; }
    _streamCleanup = EventsOn('chat:stream:event', handleStreamEvent);
    _chatListenerRegistered = true;
  }
}

function cleanupChatListenerIfNeeded() {
  if (get(streamingIndex) < 0 && _bgChatStreams.size === 0) {
    EventsOff('chat:stream:event');
    if (_streamCleanup) { _streamCleanup(); _streamCleanup = null; }
    _chatListenerRegistered = false;
  }
}

// Save current foreground streaming state into the background cache.
function saveCurrentChatToBackground() {
  const currentId = get(activeChatId);
  if (!currentId || !get(loading)) return;
  _bgChatStreams.set(currentId, {
    messages: get(messages),
    streamingIndex: get(streamingIndex),
    currentThinkingIdx: get(currentThinkingIdx),
    lastSeenSeq: _lastSeenSeq,
    createdFiles: get(chatCreatedFiles),
  });
  backgroundStreamingSessions.update(s => { s.add(currentId); return new Set(s); });
  // Reset foreground streaming indicators (backend keeps running).
  streamingIndex.set(-1);
  currentThinkingIdx.set(-1);
  loading.set(false);
}

// ─── Methods ───

export function applyStatus(status) {
  ready.set(status.ready);
  agentName.set(status.agentName || 'Talon');
  userName.set(status.userName || '');
  telegramStatus.set(status.telegramStatus || 'stopped');
  configuredProviders.set(status.configuredProviders || []);
  if (!status.ready && status.error) {
    initError.set(status.error);
  } else {
    initError.set('');
  }
}

export async function checkBackendStatus() {
  try {
    const status = await GetStatus();
    applyStatus(status);

    if (!status.ready) {
      console.log('Backend not ready yet, retrying in 500ms...');
      return setTimeout(checkBackendStatus, 500);
    }

    // Start with no active session — a session will be created lazily
    // when the user sends their first message.
    activeChatId.set(null);
    await refreshChatHistory();
  } catch (e) {
    console.error('Status check failed:', e);
    return setTimeout(checkBackendStatus, 500);
  }
}

export async function refreshChatHistory() {
  try {
    const sessions = await ListSessions();
    chatHistory.set(
      (sessions || []).map(s => ({
        id: s.id,
        title: s.title || 'Untitled',
        timestamp: s.timestamp,
      }))
    );
  } catch (e) {
    console.error('Failed to load chat history:', e);
  }
}

export function handleStreamEvent(data) {
  const activeId = get(activeChatId);
  const sessionId = data.session_id;

  // Route events for background sessions to the background handler.
  if (sessionId && sessionId !== activeId) {
    if (_bgChatStreams.has(sessionId)) {
      handleBgChatStreamEvent(sessionId, data);
    }
    return;
  }

  const idx = get(streamingIndex);
  if (idx < 0) return;

  // Sequence-based dedup: the backend sends a monotonically increasing `seq`
  // with each event. Reject any event whose seq we've already processed.
  // This reliably catches duplicates from the Wails macOS WebKit bridge
  // regardless of timing.
  if (data.seq) {
    if (data.seq <= _lastSeenSeq) return;
    _lastSeenSeq = data.seq;
  }

  messages.update(msgs => {
    const updated = [...msgs];
    const msg = { ...updated[idx] };

    switch (data.type) {
      case 'thinking_start': {
        const steps = [...(msg.steps || [])];
        const thinkIdx = get(currentThinkingIdx);
        if (steps.length > 0 && steps[steps.length - 1].type === 'thinking') {
          currentThinkingIdx.set(steps.length - 1);
          steps[steps.length - 1] = { ...steps[steps.length - 1], content: steps[steps.length - 1].content + '\n\n' };
        } else {
          currentThinkingIdx.set(steps.length);
          steps.push({ type: 'thinking', content: '' });
        }
        msg.steps = steps;
        break;
      }

      case 'thinking': {
        const thinkIdx = get(currentThinkingIdx);
        if (thinkIdx >= 0 && msg.steps[thinkIdx]) {
          const steps = [...msg.steps];
          steps[thinkIdx] = { ...steps[thinkIdx], content: steps[thinkIdx].content + data.content };
          msg.steps = steps;
        }
        break;
      }

      case 'text':
        msg.content = (msg.content || '') + data.content;
        break;

      case 'tool_call':
        currentThinkingIdx.set(-1);
        msg.steps = [...(msg.steps || []), { type: 'tool_call', tool_name: data.tool_name, tool_input: data.tool_input }];
        break;

      case 'tool_result':
        msg.steps = [...(msg.steps || []), { type: 'tool_result', tool_name: data.tool_name, content: data.content }];
        // Reset accumulated text so the next LLM iteration starts fresh.
        // Without this, text from the previous iteration gets concatenated
        // with text from the new iteration, causing visible repetition.
        msg.content = '';
        break;

      case 'file_created':
        if (data.path) {
          const fName = data.name || data.path.split('/').pop();
          chatCreatedFiles.update(files => {
            if (files.find(f => f.path === data.path)) return files;
            return [...files, { name: fName, path: data.path }];
          });
        }
        return msgs;

      case 'done':
        msg.role = 'assistant';
        msg.content = data.final_text;
        msg.steps = data.steps || [];
        delete msg.isStreaming;
        finishStream();
        break;

      case 'error':
        msg.role = 'assistant';
        msg.content = `Something went wrong: ${data.error}`;
        msg.isError = true;
        delete msg.isStreaming;
        finishStream();
        break;
    }

    updated[idx] = msg;
    return updated;
  });
}

// Handle stream events for sessions running in the background.
function handleBgChatStreamEvent(sessionId, data) {
  const state = _bgChatStreams.get(sessionId);
  if (!state || state.streamingIndex < 0) return;

  // Sequence-based dedup.
  if (data.seq) {
    if (data.seq <= state.lastSeenSeq) return;
    state.lastSeenSeq = data.seq;
  }

  const idx = state.streamingIndex;
  const msg = { ...state.messages[idx] };

  switch (data.type) {
    case 'thinking_start': {
      const steps = [...(msg.steps || [])];
      if (steps.length > 0 && steps[steps.length - 1].type === 'thinking') {
        state.currentThinkingIdx = steps.length - 1;
        steps[state.currentThinkingIdx] = { ...steps[state.currentThinkingIdx], content: steps[state.currentThinkingIdx].content + '\n\n' };
      } else {
        state.currentThinkingIdx = steps.length;
        steps.push({ type: 'thinking', content: '' });
      }
      msg.steps = steps;
      break;
    }

    case 'thinking':
      if (state.currentThinkingIdx >= 0 && msg.steps[state.currentThinkingIdx]) {
        const steps = [...msg.steps];
        steps[state.currentThinkingIdx] = { ...steps[state.currentThinkingIdx], content: steps[state.currentThinkingIdx].content + data.content };
        msg.steps = steps;
      }
      break;

    case 'text':
      msg.content = (msg.content || '') + data.content;
      break;

    case 'tool_call':
      state.currentThinkingIdx = -1;
      msg.steps = [...(msg.steps || []), { type: 'tool_call', tool_name: data.tool_name, tool_input: data.tool_input }];
      break;

    case 'tool_result':
      msg.steps = [...(msg.steps || []), { type: 'tool_result', tool_name: data.tool_name, content: data.content }];
      msg.content = '';
      break;

    case 'file_created':
      if (data.path) {
        const fName = data.name || data.path.split('/').pop();
        if (!state.createdFiles.find(f => f.path === data.path)) {
          state.createdFiles.push({ name: fName, path: data.path });
        }
      }
      return; // no message update

    case 'done':
      msg.role = 'assistant';
      msg.content = data.final_text;
      msg.steps = data.steps || [];
      delete msg.isStreaming;
      state.messages[idx] = msg;
      // Clean up background state — session is done.
      _bgChatStreams.delete(sessionId);
      backgroundStreamingSessions.update(s => { s.delete(sessionId); return new Set(s); });
      cleanupChatListenerIfNeeded();
      refreshChatHistory();
      return;

    case 'error':
      msg.role = 'assistant';
      msg.content = `Something went wrong: ${data.error}`;
      msg.isError = true;
      delete msg.isStreaming;
      state.messages[idx] = msg;
      _bgChatStreams.delete(sessionId);
      backgroundStreamingSessions.update(s => { s.delete(sessionId); return new Set(s); });
      cleanupChatListenerIfNeeded();
      refreshChatHistory();
      return;
  }

  state.messages[idx] = msg;
}

export function finishStream() {
  streamingIndex.set(-1);
  currentThinkingIdx.set(-1);
  loading.set(false);
  cleanupChatListenerIfNeeded();
  refreshChatHistory();
}

export async function sendMessage(text, files) {
  if ((!text && (!files || files.length === 0)) || get(loading)) return;

  // Lazy session creation: if the user types directly without clicking "New Chat",
  // create a session on the fly and add an "Untitled" entry to the sidebar.
  let sid = get(activeChatId);
  if (!sid) {
    sid = await NewSession();
    activeChatId.set(sid);
    chatHistory.update(history => [
      { id: sid, title: 'Untitled', timestamp: Date.now() },
      ...history,
    ]);
  }

  // Immediately update the sidebar title from the user's message.
  if (text) {
    const title = text.length > 60 ? text.slice(0, 60) + '...' : text;
    chatHistory.update(history =>
      history.map(h => (h.id === sid ? { ...h, title } : h))
    );
  }

  const userMessage = {
    role: 'user',
    content: text || '',
    files: files && files.length > 0
      ? files.map(f => ({ name: f.name, type: f.type, size: f.size, dataUrl: f.dataUrl }))
      : undefined,
  };

  messages.update(msgs => [
    ...msgs,
    userMessage,
    { role: 'assistant', content: '', steps: [], isStreaming: true },
  ]);

  const msgList = get(messages);
  streamingIndex.set(msgList.length - 1);
  currentThinkingIdx.set(-1);
  loading.set(true);

  // Reset sequence counter for the new stream.
  _lastSeenSeq = 0;

  // Ensure the global chat event listener is registered.
  ensureChatListener();

  try {
    if (files && files.length > 0) {
      const attachments = files.map(f => ({
        name: f.name,
        mime_type: f.type,
        data: f.data,
      }));
      await SendMessageStreamWithFiles(text || '', attachments, sid);
    } else {
      await SendMessageStream(text, sid);
    }
  } catch (err) {
    const idx = get(streamingIndex);
    if (idx >= 0) {
      messages.update(msgs => {
        const updated = [...msgs];
        updated[idx] = {
          role: 'assistant',
          content: `Something went wrong: ${err}`,
          isError: true,
        };
        return updated;
      });
    }
    finishStream();
  }
}

export async function cancelStream() {
  try {
    await CancelStream(get(activeChatId) || '');
  } catch (err) {
    console.error('Cancel failed:', err);
  }
  const idx = get(streamingIndex);
  if (idx >= 0) {
    messages.update(msgs => {
      const updated = [...msgs];
      const msg = updated[idx];
      updated[idx] = {
        role: 'assistant',
        content: msg.content || '_Request cancelled._',
        steps: msg.steps || [],
        isError: !msg.content,
      };
      return updated;
    });
  }
  finishStream();
}

export async function newSession() {
  // If a stream is active, move it to the background so it keeps running.
  saveCurrentChatToBackground();

  // Refresh from backend first to purge any previous unsaved "Untitled" entries.
  await refreshChatHistory();

  const newId = await NewSession();
  activeChatId.set(newId);
  messages.set([]);
  chatCreatedFiles.set([]);
  isFirstMessage.set(true);

  // Immediately show the new chat as "Untitled" in the sidebar.
  chatHistory.update(history => [
    { id: newId, title: 'Untitled', timestamp: Date.now() },
    ...history,
  ]);
}

export async function selectChat(sessionId) {
  if (sessionId === get(activeChatId) && get(messages).length > 0) return;

  // If the current session is streaming, move it to the background.
  saveCurrentChatToBackground();

  // If the target session is streaming in the background, restore it.
  if (_bgChatStreams.has(sessionId)) {
    const state = _bgChatStreams.get(sessionId);
    _bgChatStreams.delete(sessionId);
    backgroundStreamingSessions.update(s => { s.delete(sessionId); return new Set(s); });

    activeChatId.set(sessionId);
    isFirstMessage.set(false);
    messages.set(state.messages);
    streamingIndex.set(state.streamingIndex);
    currentThinkingIdx.set(state.currentThinkingIdx);
    _lastSeenSeq = state.lastSeenSeq;
    chatCreatedFiles.set(state.createdFiles);
    loading.set(state.streamingIndex >= 0);
    return;
  }

  // Otherwise load from the backend.
  try {
    const loaded = await LoadSession(sessionId);
    activeChatId.set(sessionId);
    isFirstMessage.set(false);
    messages.set(
      (loaded || []).map(m => ({
        role: m.role,
        content: m.content,
        steps: m.steps || [],
        files: m.files && m.files.length > 0
          ? m.files.map(f => ({ name: f.name, type: f.type }))
          : undefined,
      }))
    );

    // Load any files produced during this chat session.
    chatCreatedFiles.set([]);
    try {
      const sessionFiles = await ListChatFiles(sessionId);
      if (sessionFiles && sessionFiles.length > 0) {
        chatCreatedFiles.set(sessionFiles.map(f => ({ name: f.name, path: f.path })));
      }
    } catch (_) {}
  } catch (e) {
    console.error('Failed to load session:', e);
  }
}

export async function deleteChat(sessionId) {
  try {
    // If the session has a background stream, cancel it.
    if (_bgChatStreams.has(sessionId)) {
      try { await CancelStream(sessionId); } catch (_) {}
      _bgChatStreams.delete(sessionId);
      backgroundStreamingSessions.update(s => { s.delete(sessionId); return new Set(s); });
      cleanupChatListenerIfNeeded();
    }

    await DeleteSession(sessionId);
    if (sessionId === get(activeChatId)) {
      await newSession();
    }
    await refreshChatHistory();
  } catch (e) {
    console.error('Failed to delete session:', e);
  }
}

export async function toggleTelegram() {
  telegramToggling.set(true);
  try {
    const newStatus = await ToggleTelegram();
    telegramStatus.set(newStatus);
  } catch (e) {
    telegramStatus.set('error');
    console.error('Telegram toggle failed:', e);
  } finally {
    telegramToggling.set(false);
  }
}

// ─── Telegram chat methods ───

export async function refreshTelegramChats() {
  try {
    const sessions = await ListTelegramSessions();
    telegramChats.set(
      (sessions || []).map(s => ({
        id: s.id,
        title: s.title || 'Telegram chat',
        timestamp: s.timestamp,
      }))
    );
  } catch (e) {
    console.error('Failed to load telegram chats:', e);
  }
}

export async function selectTelegramChat(sessionId) {
  if (get(loading)) return;
  if (sessionId === get(activeTelegramChatId) && get(telegramMessages).length > 0) return;

  try {
    const loaded = await LoadTelegramSession(sessionId);
    activeTelegramChatId.set(sessionId);
    viewingTelegram.set(true);
    telegramMessages.set(
      (loaded || []).map(m => ({
        role: m.role,
        content: m.content,
        steps: m.steps || [],
      }))
    );
  } catch (e) {
    console.error('Failed to load telegram session:', e);
  }
}

export function closeTelegramView() {
  viewingTelegram.set(false);
  activeTelegramChatId.set(null);
  telegramMessages.set([]);
}

export async function deleteTelegramChat(sessionId) {
  try {
    await DeleteSession(sessionId);
    if (sessionId === get(activeTelegramChatId)) {
      closeTelegramView();
    }
    await refreshTelegramChats();
  } catch (e) {
    console.error('Failed to delete telegram session:', e);
  }
}

export async function changeModel(id, label) {
  try {
    await ChangeModel(id);
    console.log(`Model switched to ${label} (${id})`);
  } catch (err) {
    console.error('Failed to change model:', err);
    initError.set(`Model switch failed: ${err}`);
    setTimeout(() => {
      const current = get(initError);
      if (current.startsWith('Model switch')) initError.set('');
    }, 4000);
  }
}
