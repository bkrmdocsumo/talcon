import { writable, get } from 'svelte/store';
import { formatToolName } from '../utils/formatters.js';
import {
  NewAgentSession,
  ListAgentSessions,
  LoadSession,
  DeleteSession,
  SendAgentTaskStream,
  CancelStream,
  ListTaskFiles,
} from '../../../wailsjs/go/main/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

// ─── Agent task state ───
export const agentPhase = writable('welcome');       // 'welcome' | 'workspace'
export const agentTaskHistory = writable([]);         // Array of { id, title, timestamp }
export const activeAgentTaskId = writable(null);
export const agentTaskTitle = writable('');
export const agentMessages = writable([]);            // Array of { role, content, steps?, isStreaming? }
export const agentStreamingIdx = writable(-1);
export const agentProgressSteps = writable([]);       // Derived progress steps
export const agentCreatedFiles = writable([]);        // Files created during the task
export const agentContextTools = writable([]);        // Distinct tool names used
export const agentLoading = writable(false);
export const agentIsStreaming = writable(false);

// Internal state
let _agentStreamCleanup = null;
let _agentCurrentThinkingIdx = -1;

// ─── Methods ───

export async function refreshAgentTaskHistory() {
  try {
    const sessions = await ListAgentSessions();
    agentTaskHistory.set(
      (sessions || []).map(s => ({
        id: s.id,
        title: s.title || 'Untitled task',
        timestamp: s.timestamp,
      }))
    );
  } catch (e) {
    console.error('Failed to load agent task history:', e);
  }
}

function handleAgentStreamEvent(data) {
  if (!get(agentIsStreaming)) return;
  const idx = get(agentStreamingIdx);
  if (idx < 0) return;

  agentMessages.update(msgs => {
    const updated = [...msgs];
    const msg = { ...updated[idx] };
    if (!msg) return msgs;

    switch (data.type) {
      case 'thinking_start': {
        const steps = [...(msg.steps || [])];
        if (steps.length > 0 && steps[steps.length - 1].type === 'thinking') {
          _agentCurrentThinkingIdx = steps.length - 1;
          steps[_agentCurrentThinkingIdx] = {
            ...steps[_agentCurrentThinkingIdx],
            content: steps[_agentCurrentThinkingIdx].content + '\n\n',
          };
        } else {
          _agentCurrentThinkingIdx = steps.length;
          steps.push({ type: 'thinking', content: '' });
        }
        msg.steps = steps;
        break;
      }

      case 'thinking':
        if (_agentCurrentThinkingIdx >= 0 && msg.steps[_agentCurrentThinkingIdx]) {
          const steps = [...msg.steps];
          steps[_agentCurrentThinkingIdx] = {
            ...steps[_agentCurrentThinkingIdx],
            content: steps[_agentCurrentThinkingIdx].content + data.content,
          };
          msg.steps = steps;
        }
        break;

      case 'text':
        msg.content = (msg.content || '') + data.content;
        break;

      case 'tool_call': {
        _agentCurrentThinkingIdx = -1;
        msg.steps = [...(msg.steps || []), { type: 'tool_call', tool_name: data.tool_name, tool_input: data.tool_input }];

        // Track context tools (deduplicated), skip todo_write as it's a planning meta-tool.
        if (data.tool_name !== 'todo_write') {
          const toolLabel = formatToolName(data.tool_name);
          agentContextTools.update(tools => {
            if (tools.includes(toolLabel)) return tools;
            return [...tools, toolLabel];
          });
        }
        break;
      }

      case 'tool_result':
        msg.steps = [...(msg.steps || []), { type: 'tool_result', tool_name: data.tool_name, content: data.content }];
        break;

      case 'todo_update':
        if (data.todo_items && Array.isArray(data.todo_items)) {
          agentProgressSteps.set(
            data.todo_items.map(item => ({
              id: item.id,
              label: item.content,
              status: item.status,
            }))
          );
        }
        // Return early — no message update needed.
        return msgs;

      case 'file_created':
        if (data.path) {
          const fName = data.name || data.path.split('/').pop();
          agentCreatedFiles.update(files => {
            if (files.find(f => f.path === data.path)) return files;
            return [...files, { name: fName, path: data.path }];
          });
        }
        // Return early — no message update needed.
        return msgs;

      case 'done':
        msg.role = 'assistant';
        msg.content = data.final_text || msg.content;
        msg.steps = data.steps || msg.steps;
        delete msg.isStreaming;
        // Mark any remaining in_progress steps as completed.
        agentProgressSteps.update(steps =>
          steps.some(s => s.status === 'in_progress')
            ? steps.map(s => s.status === 'in_progress' ? { ...s, status: 'completed' } : s)
            : steps
        );
        finishAgentStream();
        break;

      case 'error':
        msg.role = 'assistant';
        msg.content = msg.content || `Something went wrong: ${data.error}`;
        msg.steps = msg.steps || [];
        msg.isError = true;
        delete msg.isStreaming;
        finishAgentStream();
        break;
    }

    updated[idx] = msg;
    return updated;
  });
}

function finishAgentStream() {
  if (_agentStreamCleanup) {
    _agentStreamCleanup();
    _agentStreamCleanup = null;
  }

  const idx = get(agentStreamingIdx);
  if (idx >= 0) {
    agentMessages.update(msgs => {
      const updated = [...msgs];
      if (updated[idx]) {
        updated[idx] = { ...updated[idx], isStreaming: false };
      }
      return updated;
    });
  }

  agentIsStreaming.set(false);
  agentLoading.set(false);
  _agentCurrentThinkingIdx = -1;
  agentStreamingIdx.set(-1);
  refreshAgentTaskHistory();
}

export async function startAgentTask(text, readyFlag) {
  if (!text || get(agentLoading) || !readyFlag) return;

  const newId = await NewAgentSession();
  activeAgentTaskId.set(newId);

  let title = text.trim();
  if (title.length > 50) title = title.substring(0, 50) + '...';
  agentTaskTitle.set(title);

  agentMessages.set([
    { role: 'user', content: text },
    { role: 'assistant', content: '', steps: [], isStreaming: true },
  ]);
  agentStreamingIdx.set(1);
  agentProgressSteps.set([]);
  agentCreatedFiles.set([]);
  agentContextTools.set([]);
  agentLoading.set(true);
  agentIsStreaming.set(true);
  _agentCurrentThinkingIdx = -1;
  agentPhase.set('workspace');

  _agentStreamCleanup = EventsOn('stream:event', handleAgentStreamEvent);

  try {
    await SendAgentTaskStream(text);
  } catch (err) {
    agentMessages.update(msgs => {
      const updated = [...msgs];
      const idx = get(agentStreamingIdx);
      if (updated[idx]) {
        updated[idx] = { ...updated[idx], content: `Something went wrong: ${err}` };
      }
      return updated;
    });
    finishAgentStream();
  }
}

export async function sendAgentFollowUp(text, readyFlag) {
  if (!text || get(agentLoading) || !readyFlag) return;

  agentMessages.update(msgs => [
    ...msgs,
    { role: 'user', content: text },
    { role: 'assistant', content: '', steps: [], isStreaming: true },
  ]);

  const msgList = get(agentMessages);
  agentStreamingIdx.set(msgList.length - 1);
  agentLoading.set(true);
  agentIsStreaming.set(true);
  _agentCurrentThinkingIdx = -1;

  _agentStreamCleanup = EventsOn('stream:event', handleAgentStreamEvent);

  try {
    await SendAgentTaskStream(text);
  } catch (err) {
    agentMessages.update(msgs => {
      const updated = [...msgs];
      const idx = get(agentStreamingIdx);
      if (updated[idx]) {
        updated[idx] = { ...updated[idx], content: `Something went wrong: ${err}` };
      }
      return updated;
    });
    finishAgentStream();
  }
}

export async function cancelAgent() {
  try {
    await CancelStream();
  } catch (err) {
    console.error('Agent cancel failed:', err);
  }
  finishAgentStream();
}

export function newAgentTask() {
  agentPhase.set('welcome');
  agentTaskTitle.set('');
  agentMessages.set([]);
  agentStreamingIdx.set(-1);
  agentProgressSteps.set([]);
  agentCreatedFiles.set([]);
  agentContextTools.set([]);
  agentLoading.set(false);
  agentIsStreaming.set(false);
  activeAgentTaskId.set(null);
}

export async function selectAgentTask(sessionId) {
  if (get(agentLoading)) return;
  if (sessionId === get(activeAgentTaskId) && get(agentPhase) === 'workspace') return;

  try {
    const loaded = await LoadSession(sessionId);
    activeAgentTaskId.set(sessionId);

    const msgs = (loaded || [])
      .filter(m => m.role === 'user' || m.role === 'assistant')
      .map(m => ({
        role: m.role,
        content: m.content || '',
        steps: (m.steps || []).map(s => ({
          type: s.type,
          content: s.content,
          tool_name: s.tool_name,
          tool_input: s.tool_input,
        })),
      }));

    agentMessages.set(msgs);

    // Derive title from first user message.
    const firstUser = msgs.find(m => m.role === 'user');
    let title = (firstUser?.content || '').trim();
    if (title.length > 50) title = title.substring(0, 50) + '...';
    agentTaskTitle.set(title);

    // Rebuild progress from todo_write tool calls.
    const allSteps = msgs.flatMap(m => m.steps || []);
    const todoSteps = allSteps.filter(s => s.type === 'tool_call' && s.tool_name === 'todo_write');
    if (todoSteps.length > 0) {
      try {
        const lastTodo = JSON.parse(todoSteps[todoSteps.length - 1].tool_input);
        if (lastTodo.todos && Array.isArray(lastTodo.todos)) {
          agentProgressSteps.set(
            lastTodo.todos.map(item => ({
              id: item.id,
              label: item.content,
              status: item.status || 'completed',
            }))
          );
        }
      } catch (_) {
        agentProgressSteps.set([]);
      }
    } else {
      agentProgressSteps.set([]);
    }

    agentContextTools.set([
      ...new Set(
        allSteps
          .filter(s => s.type === 'tool_call' && s.tool_name !== 'todo_write')
          .map(s => formatToolName(s.tool_name))
      ),
    ]);
    agentCreatedFiles.set([]);

    // Try loading files from the backend.
    try {
      const taskFiles = await ListTaskFiles(sessionId);
      if (taskFiles && taskFiles.length > 0) {
        agentCreatedFiles.set(taskFiles.map(f => ({ name: f.name, path: f.path })));
      }
    } catch (_) {}

    agentStreamingIdx.set(-1);
    agentPhase.set('workspace');
    agentLoading.set(false);
    agentIsStreaming.set(false);
  } catch (e) {
    console.error('Failed to load agent task:', e);
  }
}

export async function deleteAgentTask(sessionId) {
  try {
    await DeleteSession(sessionId);
    if (sessionId === get(activeAgentTaskId)) {
      newAgentTask();
    }
    await refreshAgentTaskHistory();
  } catch (e) {
    console.error('Failed to delete agent task:', e);
  }
}
