const BASE = '/api';

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(BASE + path, {
		method,
		headers: body ? { 'Content-Type': 'application/json' } : {},
		body: body ? JSON.stringify(body) : undefined,
		credentials: 'include'
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(err.error ?? res.statusText);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

export const api = {
	// auth
	logout: () => req<void>('POST', '/auth/logout'),

	// users
	me: () => req<User>('GET', '/users/me'),
	updateMe: (data: { display_name: string; bio: string }) => req<User>('PATCH', '/users/me', data),
	getUser: (id: string) => req<User>('GET', `/users/${id}`),

	// servers
	listServers: () => req<Server[]>('GET', '/servers'),
	createServer: (data: { name: string; description: string; is_public: boolean }) =>
		req<Server>('POST', '/servers', data),
	getServer: (id: string) => req<Server>('GET', `/servers/${id}`),
	joinServer: (id: string) => req<void>('POST', `/servers/${id}/join`),
	updateServer: (id: string, data: { name?: string; description?: string; is_public?: boolean; member_invites_enabled?: boolean; member_invite_expiry_days?: number }) =>
		req<Server>('PATCH', `/servers/${id}`, data),
	uploadServerIcon: async (id: string, file: File) => {
		const form = new FormData();
		form.append('icon', file);
		const res = await fetch(`/api/servers/${id}/icon`, { method: 'POST', body: form, credentials: 'include' });
		if (!res.ok) throw new Error('Upload failed');
		return res.json() as Promise<{ icon_url: string }>;
	},
	deleteServer: (id: string) => req<void>('DELETE', `/servers/${id}`),
	leaveServer: (id: string) => req<void>('DELETE', `/servers/${id}/leave`),
	getMembers: (id: string) => req<ServerMember[]>('GET', `/servers/${id}/members`),
	updateMemberRole: (serverId: string, userId: string, role: 'admin' | 'member') =>
		req<{ role: string }>('PATCH', `/servers/${serverId}/members/${userId}`, { role }),
	createInvite: (serverId: string) => req<Invite>('POST', `/servers/${serverId}/invites`),
	joinByInvite: (code: string) => req<{ server_id: string }>('POST', `/invites/${code}/join`),

	// channels
	listChannels: (serverId: string) => req<Channel[]>('GET', `/servers/${serverId}/channels`),
	createChannel: (serverId: string, data: { name: string; description: string }) =>
		req<Channel>('POST', `/servers/${serverId}/channels`, data),
	deleteChannel: (serverId: string, channelId: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}`),
	markRead: (serverId: string, channelId: string) =>
		req<void>('POST', `/servers/${serverId}/channels/${channelId}/read`),

	// messages
	listMessages: (serverId: string, channelId: string, before?: string) =>
		req<Message[]>('GET', `/servers/${serverId}/channels/${channelId}/messages${before ? `?before=${before}` : ''}`),
	sendMessage: (serverId: string, channelId: string, content: string) =>
		req<Message>('POST', `/servers/${serverId}/channels/${channelId}/messages`, { content }),
	editMessage: (serverId: string, channelId: string, messageId: string, content: string) =>
		req<Message>('PATCH', `/servers/${serverId}/channels/${channelId}/messages/${messageId}`, { content }),
	deleteMessage: (serverId: string, channelId: string, messageId: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}/messages/${messageId}`),
	deleteDM: (convId: string, messageId: string) =>
		req<void>('DELETE', `/dms/conversations/${convId}/messages/${messageId}`),
	uploadFile: async (file: File) => {
		const form = new FormData();
		form.append('file', file);
		const res = await fetch('/api/upload', { method: 'POST', body: form, credentials: 'include' });
		if (!res.ok) throw new Error('Upload failed');
		return res.json() as Promise<{ url: string }>;
	},

	// DMs
	listConversations: () => req<DMConversation[]>('GET', '/dms'),
	getOrCreateDM: (userId: string) => req<DMConversation>('POST', `/dms/${userId}`),
	listDMMessages: (convId: string) => req<DirectMessage[]>('GET', `/dms/conversations/${convId}/messages`),
	sendDM: (convId: string, content: string) =>
		req<DirectMessage>('POST', `/dms/conversations/${convId}/messages`, { content }),

	// friends
	listFriends: () => req<FriendEntry[]>('GET', '/friends'),
	listFriendRequests: () => req<FriendEntry[]>('GET', '/friends/requests'),
	listFriendRequestsSent: () => req<FriendEntry[]>('GET', '/friends/sent'),
	sendFriendRequest: (userId: string) => req<{ status: string }>('POST', `/friends/request/${userId}`),
	acceptFriendRequest: (userId: string) => req<void>('POST', `/friends/accept/${userId}`),
	removeFriend: (userId: string) => req<void>('DELETE', `/friends/${userId}`),
	searchUsers: (q: string) => req<User[]>('GET', `/users/search?q=${encodeURIComponent(q)}`),

	// push notifications
	getPushKey: () => req<{ public_key: string }>('GET', '/push/key'),
	pushSubscribe: (sub: { endpoint: string; p256dh: string; auth: string }) =>
		req<void>('POST', '/push/subscribe', sub),
	pushUnsubscribe: (endpoint: string) =>
		req<void>('DELETE', '/push/subscribe', { endpoint }),

	// instance admin
	getAdminSettings: () => req<AdminSettings>('GET', '/admin/settings'),
	updateAdminSettings: (data: Partial<AdminSettings>) => req<void>('PATCH', '/admin/settings', data),
	runRetention: () => req<{ status: string }>('POST', '/admin/retention/run'),

	// avatar upload
	uploadAvatar: async (file: File) => {
		const form = new FormData();
		form.append('avatar', file);
		const res = await fetch('/api/users/me/avatar', {
			method: 'POST',
			body: form,
			credentials: 'include'
		});
		if (!res.ok) throw new Error('Upload failed');
		return res.json() as Promise<{ avatar_url: string }>;
	}
};

// Types
export interface User {
	id: string;
	email: string;
	display_name: string;
	bio: string;
	avatar_url: string;
	is_instance_admin?: boolean;
	created_at: string;
}

export interface AdminSettings {
	message_retention_days: string;
	inactive_space_retention_days: string;
	[key: string]: string;
}

export interface Server {
	id: string;
	name: string;
	description: string;
	icon_url: string;
	owner_id: string;
	is_public: boolean;
	invite_code: string;
	member_invites_enabled: boolean;
	member_invite_expiry_days: number;
	role?: string;
	created_at: string;
}

export interface Channel {
	id: string;
	server_id: string;
	name: string;
	description: string;
	position: number;
	unread_count: number;
	created_at: string;
}

export interface Message {
	id: string;
	channel_id: string;
	author: User;
	content: string;
	edited_at?: string;
	created_at: string;
}

export interface DMConversation {
	id: string;
	other_user: User;
	created_at: string;
}

export interface DirectMessage {
	id: string;
	conversation_id: string;
	sender: User;
	content: string;
	created_at: string;
}

export interface ServerMember {
	user: User;
	role: string;
	joined_at: string;
}

export interface FriendEntry {
	user: User;
	since: string;
}

export interface Invite {
	id: string;
	server_id: string;
	code: string;
	expires_at?: string;
	max_uses?: number;
	use_count: number;
	created_at: string;
}
