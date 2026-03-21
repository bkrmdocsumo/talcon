import { writable } from 'svelte/store';
import {
  GetAccessPolicy,
  SetAccessPolicy,
  GeneratePairingCode,
  ListAccessSenders,
  ApproveSender,
  BlockSender,
  RemoveAccessSender,
  GetChannelAgents,
  SetChannelAgent,
  RemoveChannelAgent,
} from '../../../wailsjs/go/main/App';

// ─── Access control stores ───

export const accessPolicy = writable({
  dm_policy: 'open',
  group_policy: 'open',
  mention_gating: false,
  pairing_code: '',
});

export const accessSenders = writable([]);
export const channelAgents = writable([]);

// ─── Data fetching ───

export async function refreshAccessPolicy() {
  try {
    const policy = await GetAccessPolicy();
    accessPolicy.set(policy);
  } catch (err) {
    console.error('Failed to load access policy:', err);
  }
}

export async function refreshAccessSenders() {
  try {
    const senders = await ListAccessSenders();
    accessSenders.set(senders || []);
  } catch (err) {
    console.error('Failed to load access senders:', err);
  }
}

export async function refreshChannelAgents() {
  try {
    const agents = await GetChannelAgents();
    channelAgents.set(agents || []);
  } catch (err) {
    console.error('Failed to load channel agents:', err);
  }
}

// ─── Actions ───

export async function updateAccessPolicy(policy) {
  try {
    await SetAccessPolicy(policy);
    accessPolicy.set(policy);
  } catch (err) {
    console.error('Failed to update access policy:', err);
  }
}

export async function generateNewPairingCode() {
  try {
    const code = await GeneratePairingCode();
    accessPolicy.update((p) => ({ ...p, pairing_code: code }));
    return code;
  } catch (err) {
    console.error('Failed to generate pairing code:', err);
    return null;
  }
}

export async function approveSender(id) {
  try {
    await ApproveSender(id);
    await refreshAccessSenders();
  } catch (err) {
    console.error('Failed to approve sender:', err);
  }
}

export async function blockSender(id) {
  try {
    await BlockSender(id);
    await refreshAccessSenders();
  } catch (err) {
    console.error('Failed to block sender:', err);
  }
}

export async function removeSender(id) {
  try {
    await RemoveAccessSender(id);
    await refreshAccessSenders();
  } catch (err) {
    console.error('Failed to remove sender:', err);
  }
}

export async function setChannelAgentMapping(channelID, agentName) {
  try {
    await SetChannelAgent(channelID, agentName);
    await refreshChannelAgents();
  } catch (err) {
    console.error('Failed to set channel agent:', err);
  }
}

export async function removeChannelAgentMapping(channelID) {
  try {
    await RemoveChannelAgent(channelID);
    await refreshChannelAgents();
  } catch (err) {
    console.error('Failed to remove channel agent:', err);
  }
}

// ─── Initialize ───

export async function initAccessData() {
  await Promise.all([
    refreshAccessPolicy(),
    refreshAccessSenders(),
    refreshChannelAgents(),
  ]);
}
