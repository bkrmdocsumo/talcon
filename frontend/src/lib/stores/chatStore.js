import { writable, get } from 'svelte/store';
import {
  SendMessageStream,
  SendMessageStreamWithFiles,
  GetStatus,
  NewSession,
  GetSessionID,
  ListSessions,
  LoadSession,
  DeleteSession,
  CancelStream,
  ChangeModel,
  ToggleTelegram,
} from '../../../wailsjs/go/main/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

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

// Internal — stream cleanup function reference
let _streamCleanup = null;

// ─── Methods ───

export function applyStatus(status) {
  ready.set(status.ready);
  agentName.set(status.agentName || 'Talon');
  userName.set(status.userName || '');
  telegramStatus.set(status.telegramStatus || 'stopped');
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

    activeChatId.set(await GetSessionID());
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
        title: s.title || 'Untitled chat',
        timestamp: s.timestamp,
      }))
    );
  } catch (e) {
    console.error('Failed to load chat history:', e);
  }
}

export function handleStreamEvent(data) {
  const idx = get(streamingIndex);
  if (idx < 0) return;

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
        break;

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

export function finishStream() {
  if (_streamCleanup) {
    _streamCleanup();
    _streamCleanup = null;
  }
  streamingIndex.set(-1);
  currentThinkingIdx.set(-1);
  loading.set(false);
  refreshChatHistory();
}

export async function sendMessage(text, files) {
  if ((!text && (!files || files.length === 0)) || get(loading)) return;

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

  _streamCleanup = EventsOn('stream:event', handleStreamEvent);

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
    await CancelStream();
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
  if (get(loading)) return;
  const newId = await NewSession();
  activeChatId.set(newId);
  messages.set([]);
  isFirstMessage.set(true);
}

export async function selectChat(sessionId) {
  if (get(loading)) return;
  if (sessionId === get(activeChatId) && get(messages).length > 0) return;

  try {
    const loaded = await LoadSession(sessionId);
    activeChatId.set(sessionId);
    isFirstMessage.set(false);
    messages.set(
      (loaded || []).map(m => ({
        role: m.role,
        content: m.content,
        steps: m.steps || [],
      }))
    );
  } catch (e) {
    console.error('Failed to load session:', e);
  }
}

export async function deleteChat(sessionId) {
  try {
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
