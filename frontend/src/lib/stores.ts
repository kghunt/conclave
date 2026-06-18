import { writable } from 'svelte/store';
import type { User, Server, Channel, DMConversation, FriendEntry } from './api';

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
