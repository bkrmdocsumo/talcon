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

// ─── Initialise on first load ───
export async function initClawData() {
  await Promise.all([refreshClawEvents(), refreshClawConfig(), refreshClawStats(), refreshClawCrons()]);
  initClawListener();
}
