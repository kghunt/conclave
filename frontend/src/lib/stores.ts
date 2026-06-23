import { writable } from 'svelte/store';
import type { User, Server, Channel, DMConversation, FriendEntry, InstanceConfig, ServerMember, VoicePeer } from './api';

export interface NotifPrefs {
	messageSound: boolean;
	mentionSound: boolean;
	dmSound: boolean;
}

function loadNotifPrefs(): NotifPrefs {
	try {
		const raw = typeof localStorage !== 'undefined' ? localStorage.getItem('notifPrefs') : null;
		if (raw) return { messageSound: true, mentionSound: true, dmSound: true, ...JSON.parse(raw) };
	} catch {}
	return { messageSound: true, mentionSound: true, dmSound: true };
}

function makeNotifPrefs() {
	const store = writable<NotifPrefs>(loadNotifPrefs());
	store.subscribe((v) => {
		try { localStorage.setItem('notifPrefs', JSON.stringify(v)); } catch {}
	});
	return store;
}

export const notifPrefs = makeNotifPrefs();

export const currentUser = writable<User | null>(null);
export const servers = writable<Server[]>([]);
export const activeServer = writable<Server | null>(null);
export const channels = writable<Channel[]>([]);
export const activeChannel = writable<Channel | null>(null);
export const dmConversations = writable<DMConversation[]>([]);
export const activeDM = writable<DMConversation | null>(null);
export const showProfileModal = writable(false);
export const friends = writable<FriendEntry[]>([]);
export const friendRequests = writable<FriendEntry[]>([]);
export const friendRequestsSent = writable<FriendEntry[]>([]);
export const instanceConfig = writable<InstanceConfig>({ allow_user_space_creation: true, max_video_size_mb: 50, google_auth_enabled: true, local_auth_enabled: true, registration_mode: 'invite' });
export const serverMembers = writable<ServerMember[]>([]);
export const mentionedChannels = writable<Set<string>>(new Set());
export const presenceMap = writable<Record<string, string>>({}); // userId → 'online'|'away'|'offline'
export const pendingJoinRequests = writable<import('./api').JoinRequest[]>([]); // pending join requests for current space
export const voiceParticipants = writable<Record<string, VoicePeer[]>>({}); // channelId → current voice participants
export const serverUnread = writable<Record<string, boolean>>({}); // serverId → has unread
export const joinRequestPending = writable<Set<string>>(new Set()); // serverIds with new join requests
export const homeMode = writable(false); // true when viewing DMs/Friends panel
export const gameStatus = writable<Record<string, string>>({}); // userId → game name
