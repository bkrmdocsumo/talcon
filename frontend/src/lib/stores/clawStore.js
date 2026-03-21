import { writable, derived, get } from 'svelte/store';
import {
  GetClawEvents,
  GetClawConfig,
  SetClawConfig,
  GetClawStats,
  PushManualEvent,
  FireHeartbeatNow,
  AddClawCron,
  ListClawCrons,
  RemoveClawCron,
  GetClawEventChat,
  SendClawFollowUp,
  CancelStream,
  ListChatFiles,
} from '../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';

// ─── Claw event feed ───
export const clawEvents = writable([]);
export const clawConfig = writable({
  heartbeat_enabled: false,
  heartbeat_interval_minutes: 30,
  hooks_enabled: true,
  webhooks_enabled: true,
  crons_enabled: true,
});
export const clawStats = writable({
  total_events: 0,
  events_today: 0,
  last_heartbeat: '',
  counts_by_type: {},
  gateway_active: false,
});

// ─── Claw cron tasks ───
export const clawCrons = writable([]);

// ─── Shared sub-tab state (synced between ClawPanel + Sidebar) ───
export const clawActiveSubTab = writable('overview');

// ─── Focused event ID (set when clicking a sidebar activity item) ───
export const clawFocusedEventId = writable(null);

// Derived: only non-hidden events for the feed
export const visibleClawEvents = derived(clawEvents, ($events) =>
  $events.filter((e) => !(e.hidden && e.status === 'suppressed'))
);

// ─── Real-time event listener ───
let _clawCleanup = null;
let _cronCleanup = null;

export function initClawListener() {
  if (_clawCleanup) return;
  _clawCleanup = EventsOn('claw:event', (data) => {
    if (!data || !data.id) return;

    clawEvents.update((events) => {
      const idx = events.findIndex((e) => e.id === data.id);
      if (idx >= 0) {
        // Update existing event (e.g. processing -> completed).
        const updated = [...events];
        updated[idx] = { ...updated[idx], ...data };
        return updated;
      }
      // Prepend new event.
      return [data, ...events];
    });

    // Refresh stats after each event.
    refreshClawStats();
  });

  // Listen for cron list changes (e.g. agent added/removed a cron via tool).
  _cronCleanup = EventsOn('claw:crons:updated', () => {
    refreshClawCrons();
  });
}

export function destroyClawListener() {
  if (_clawCleanup) {
    _clawCleanup();
    _clawCleanup = null;
  }
  if (_cronCleanup) {
    _cronCleanup();
    _cronCleanup = null;
  }
}

// ─── Data fetching ───

export async function refreshClawEvents() {
  try {
    const events = await GetClawEvents();
    clawEvents.set(events || []);
  } catch (err) {
    console.error('Failed to load claw events:', err);
  }
}

export async function refreshClawConfig() {
  try {
    const cfg = await GetClawConfig();
    clawConfig.set(cfg);
  } catch (err) {
    console.error('Failed to load claw config:', err);
  }
}

export async function refreshClawStats() {
  try {
    const stats = await GetClawStats();
    clawStats.set(stats);
  } catch (err) {
    console.error('Failed to load claw stats:', err);
  }
}

// ─── Actions ───

export async function updateClawConfig(cfg) {
  try {
    await SetClawConfig(cfg);
    clawConfig.set(cfg);
  } catch (err) {
    console.error('Failed to update claw config:', err);
  }
}

export async function pushManualClawEvent(eventType, message) {
  try {
    const eventId = await PushManualEvent(eventType, message);
    return eventId;
  } catch (err) {
    console.error('Failed to push manual event:', err);
    return null;
  }
}

export async function fireHeartbeat() {
  try {
    await FireHeartbeatNow();
  } catch (err) {
    console.error('Failed to fire heartbeat:', err);
  }
}

// ─── Cron task actions ───

export async function refreshClawCrons() {
  try {
    const crons = await ListClawCrons();
    clawCrons.set(crons || []);
  } catch (err) {
    console.error('Failed to load claw crons:', err);
  }
}

export async function addClawCron(schedule, message) {
  try {
    await AddClawCron(schedule, message);
    await refreshClawCrons();
  } catch (err) {
    console.error('Failed to add claw cron:', err);
    throw err;
  }
}

export async function removeClawCron(id) {
  try {
    await RemoveClawCron(id);
    await refreshClawCrons();
  } catch (err) {
    console.error('Failed to remove claw cron:', err);
    throw err;
  }
}

// ─── Event chat loading ───

export async function loadClawEventChat(sessionID) {
  try {
    const messages = await GetClawEventChat(sessionID);
    return messages || [];
  } catch (err) {
    console.error('Failed to load claw event chat:', err);
    return [];
  }
}

// ─── Event conversation view ───

export const viewingClawEvent = writable(false);
export const activeClawEvent = writable(null);
export const clawEventMessages = writable([]);
export const clawEventFiles = writable([]);
export const clawEventLoading = writable(false);
export const clawEventStreamingIndex = writable(-1);

let _clawStreamCleanup = null;
let _clawLastSeenSeq = 0;
let _clawPendingContentReset = false;
let _clawCurrentThinkingIdx = -1;

export async function selectClawEvent(event) {
  const raw = await loadClawEventChat(event.session_id);
  clawEventMessages.set(
    (raw || []).map((m) => ({
      role: m.role,
      content: m.content,
      steps: m.steps || [],
      files:
        m.files && m.files.length > 0
          ? m.files.map((f) => ({ name: f.name, type: f.type }))
          : undefined,
    }))
  );

  try {
    const files = await ListChatFiles(event.session_id);
    clawEventFiles.set(
      (files || []).map((f) => ({ name: f.name, path: f.path }))
    );
  } catch (_) {
    clawEventFiles.set([]);
  }

  activeClawEvent.set(event);
  viewingClawEvent.set(true);
}

export function closeClawEventView() {
  viewingClawEvent.set(false);
  activeClawEvent.set(null);
  clawEventMessages.set([]);
  clawEventFiles.set([]);
  clawEventLoading.set(false);
  clawEventStreamingIndex.set(-1);
  _clawCurrentThinkingIdx = -1;
  if (_clawStreamCleanup) {
    _clawStreamCleanup();
    _clawStreamCleanup = null;
  }
}

export async function sendClawFollowUp(text) {
  if (!text || get(clawEventLoading)) return;
  const event = get(activeClawEvent);
  if (!event) return;

  clawEventMessages.update((msgs) => [
    ...msgs,
    { role: 'user', content: text },
    { role: 'assistant', content: '', steps: [], isStreaming: true },
  ]);

  const msgList = get(clawEventMessages);
  clawEventStreamingIndex.set(msgList.length - 1);
  _clawCurrentThinkingIdx = -1;
  _clawPendingContentReset = false;
  _clawLastSeenSeq = 0;
  clawEventLoading.set(true);

  EventsOff('claw:stream:event');
  if (_clawStreamCleanup) { _clawStreamCleanup(); _clawStreamCleanup = null; }
  _clawStreamCleanup = EventsOn('claw:stream:event', handleClawStreamEvent);

  try {
    await SendClawFollowUp(event.session_id, text);
  } catch (err) {
    const idx = get(clawEventStreamingIndex);
    if (idx >= 0) {
      clawEventMessages.update((msgs) => {
        const updated = [...msgs];
        updated[idx] = {
          role: 'assistant',
          content: `Something went wrong: ${err}`,
          isError: true,
        };
        return updated;
      });
    }
    finishClawStream();
  }
}

export async function cancelClawStream() {
  const event = get(activeClawEvent);
  if (!event) return;
  try {
    await CancelStream(event.session_id);
  } catch (_) {}

  const idx = get(clawEventStreamingIndex);
  if (idx >= 0) {
    clawEventMessages.update((msgs) => {
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
  finishClawStream();
}

function finishClawStream() {
  clawEventStreamingIndex.set(-1);
  _clawCurrentThinkingIdx = -1;
  _clawPendingContentReset = false;
  clawEventLoading.set(false);
  EventsOff('claw:stream:event');
  if (_clawStreamCleanup) { _clawStreamCleanup(); _clawStreamCleanup = null; }

  const event = get(activeClawEvent);
  if (event) {
    ListChatFiles(event.session_id)
      .then((files) => {
        clawEventFiles.set(
          (files || []).map((f) => ({ name: f.name, path: f.path }))
        );
      })
      .catch(() => {});
  }
}

function handleClawStreamEvent(data) {
  const idx = get(clawEventStreamingIndex);
  if (idx < 0) return;

  if (data.seq) {
    if (data.seq <= _clawLastSeenSeq) return;
    _clawLastSeenSeq = data.seq;
  }

  clawEventMessages.update((msgs) => {
    const updated = [...msgs];
    const msg = { ...updated[idx] };

    switch (data.type) {
      case 'thinking_start': {
        const steps = [...(msg.steps || [])];
        if (steps.length > 0 && steps[steps.length - 1].type === 'thinking') {
          _clawCurrentThinkingIdx = steps.length - 1;
          steps[_clawCurrentThinkingIdx] = { ...steps[_clawCurrentThinkingIdx], content: steps[_clawCurrentThinkingIdx].content + '\n\n' };
        } else {
          _clawCurrentThinkingIdx = steps.length;
          steps.push({ type: 'thinking', content: '' });
        }
        msg.steps = steps;
        break;
      }

      case 'thinking':
        if (_clawCurrentThinkingIdx >= 0 && msg.steps[_clawCurrentThinkingIdx]) {
          const steps = [...msg.steps];
          steps[_clawCurrentThinkingIdx] = { ...steps[_clawCurrentThinkingIdx], content: steps[_clawCurrentThinkingIdx].content + data.content };
          msg.steps = steps;
        }
        break;

      case 'text':
        if (_clawPendingContentReset) {
          msg.content = data.content;
          _clawPendingContentReset = false;
        } else {
          msg.content = (msg.content || '') + data.content;
        }
        break;

      case 'tool_call':
        _clawCurrentThinkingIdx = -1;
        if (_clawPendingContentReset) {
          msg.content = '';
          _clawPendingContentReset = false;
        }
        msg.steps = [...(msg.steps || []), { type: 'tool_call', tool_name: data.tool_name, tool_input: data.tool_input }];
        break;

      case 'tool_result':
        msg.steps = [...(msg.steps || []), { type: 'tool_result', tool_name: data.tool_name, content: data.content }];
        _clawPendingContentReset = true;
        break;

      case 'file_created':
        if (data.path) {
          const fName = data.name || data.path.split('/').pop();
          clawEventFiles.update((files) => {
            if (files.find((f) => f.path === data.path)) return files;
            return [...files, { name: fName, path: data.path }];
          });
        }
        return msgs;

      case 'done':
        msg.role = 'assistant';
        msg.content = data.final_text;
        msg.steps = data.steps || [];
        delete msg.isStreaming;
        finishClawStream();
        break;

      case 'error':
        msg.role = 'assistant';
        msg.content = `Something went wrong: ${data.error}`;
        msg.isError = true;
        delete msg.isStreaming;
        finishClawStream();
        break;
    }

    updated[idx] = msg;
    return updated;
  });
}

// ─── Initialise on first load ───
export async function initClawData() {
  const { initAccessData } = await import('./accessStore.js');
  await Promise.all([refreshClawEvents(), refreshClawConfig(), refreshClawStats(), refreshClawCrons(), initAccessData()]);
  initClawListener();
}
