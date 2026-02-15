import { writable, get } from 'svelte/store';
import {
  ListFlowTranscripts,
  LoadFlowTranscript,
  SaveFlowTranscript,
  DeleteFlowTranscript,
} from '../../../wailsjs/go/main/App';

// ─── Flow state ───
export const flowHistory = writable([]);
export const activeFlowId = writable(null);
export const viewingTranscript = writable(null);

// ─── Methods ───

export async function refreshFlowHistory() {
  try {
    const transcripts = await ListFlowTranscripts();
    flowHistory.set(
      (transcripts || []).map(t => ({
        id: t.id,
        title: t.title || 'Untitled recording',
        duration: t.duration || 0,
        wordCount: t.wordCount || 0,
        timestamp: t.timestamp,
      }))
    );
  } catch (e) {
    console.error('Failed to load flow history:', e);
  }
}

export async function selectFlow(flowId) {
  if (flowId === get(activeFlowId) && get(viewingTranscript)) return;

  try {
    const t = await LoadFlowTranscript(flowId);
    activeFlowId.set(flowId);
    viewingTranscript.set({
      id: t.id,
      text: t.text,
      duration: t.duration,
      wordCount: t.wordCount,
      timestamp: t.timestamp,
    });
  } catch (e) {
    console.error('Failed to load flow transcript:', e);
  }
}

export async function deleteFlow(flowId) {
  try {
    await DeleteFlowTranscript(flowId);
    if (flowId === get(activeFlowId)) {
      activeFlowId.set(null);
      viewingTranscript.set(null);
    }
    await refreshFlowHistory();
  } catch (e) {
    console.error('Failed to delete flow transcript:', e);
  }
}

export function newFlow() {
  activeFlowId.set(null);
  viewingTranscript.set(null);
}

export async function saveFlow(text, duration) {
  try {
    await SaveFlowTranscript(text, duration);
    await refreshFlowHistory();
  } catch (e) {
    console.error('Failed to save flow transcript:', e);
  }
}

export function clearFlowView() {
  activeFlowId.set(null);
  viewingTranscript.set(null);
}
