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
	updateServer: (id: string, data: { name?: string; description?: string; rules?: string; is_public?: boolean; show_in_discovery?: boolean; member_invites_enabled?: boolean; member_invite_expiry_days?: number; welcome_channel_id?: string | null; welcome_message?: string }) =>
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
	getInviteInfo: (code: string) => req<{ server_name: string; rules: string }>('GET', `/invites/${code}`),
	joinByInvite: (code: string) => req<{ server_id: string }>('POST', `/invites/${code}/join`),

	// channels
	listChannels: (serverId: string) => req<Channel[]>('GET', `/servers/${serverId}/channels`),
	createChannel: (serverId: string, data: { name: string; description: string; type?: 'text' | 'voice' | 'threads' }) =>
		req<Channel>('POST', `/servers/${serverId}/channels`, data),
	getVoiceState: (serverId: string) =>
		req<Record<string, VoicePeer[]>>('GET', `/servers/${serverId}/voice`),
	listThreads: (serverId: string, channelId: string) =>
		req<Thread[]>('GET', `/servers/${serverId}/channels/${channelId}/threads`),
	createThread: (serverId: string, channelId: string, title: string, initialMessage?: string) =>
		req<Thread>('POST', `/servers/${serverId}/channels/${channelId}/threads`, { title, initial_message: initialMessage || undefined }),
	listThreadMessages: (threadId: string) =>
		req<ThreadMessage[]>('GET', `/threads/${threadId}/messages`),
	sendThreadMessage: (threadId: string, content: string, replyToId?: string) =>
		req<ThreadMessage>('POST', `/threads/${threadId}/messages`, { content, reply_to_id: replyToId }),
	editThreadMessage: (threadId: string, messageId: string, content: string) =>
		req<ThreadMessage>('PATCH', `/threads/${threadId}/messages/${messageId}`, { content }),
	deleteThreadMessage: (threadId: string, messageId: string) =>
		req<void>('DELETE', `/threads/${threadId}/messages/${messageId}`),
	setThreadLocked: (threadId: string, locked: boolean) =>
		req<void>('PATCH', `/threads/${threadId}/lock`, { locked }),
	updateChannel: (serverId: string, channelId: string, data: { name?: string; description?: string }) =>
		req<Channel>('PATCH', `/servers/${serverId}/channels/${channelId}`, data),
	deleteChannel: (serverId: string, channelId: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}`),
	markRead: (serverId: string, channelId: string) =>
		req<void>('POST', `/servers/${serverId}/channels/${channelId}/read`),

	// messages
	listMessages: (serverId: string, channelId: string, before?: string) =>
		req<Message[]>('GET', `/servers/${serverId}/channels/${channelId}/messages${before ? `?before=${before}` : ''}`),
	sendMessage: (serverId: string, channelId: string, content: string, replyToId?: string) =>
		req<Message>('POST', `/servers/${serverId}/channels/${channelId}/messages`, { content, reply_to_id: replyToId }),
	editMessage: (serverId: string, channelId: string, messageId: string, content: string) =>
		req<Message>('PATCH', `/servers/${serverId}/channels/${channelId}/messages/${messageId}`, { content }),
	deleteMessage: (serverId: string, channelId: string, messageId: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}/messages/${messageId}`),
	addReaction: (serverId: string, channelId: string, messageId: string, emoji: string) =>
		req<void>('PUT', `/servers/${serverId}/channels/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`),
	removeReaction: (serverId: string, channelId: string, messageId: string, emoji: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`),
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
	editDM: (convId: string, messageId: string, content: string) =>
		req<DirectMessage>('PATCH', `/dms/conversations/${convId}/messages/${messageId}`, { content }),
	addDMReaction: (convId: string, messageId: string, emoji: string) =>
		req<void>('PUT', `/dms/conversations/${convId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`),
	removeDMReaction: (convId: string, messageId: string, emoji: string) =>
		req<void>('DELETE', `/dms/conversations/${convId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`),
	markDMRead: (convId: string) =>
		req<void>('POST', `/dms/conversations/${convId}/read`),

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

	// public config
	getConfig: () => req<InstanceConfig>('GET', '/config'),

	// space discovery & join requests
	discoverServers: (q: string) =>
		req<ServerDiscovery[]>('GET', `/servers/discover?q=${encodeURIComponent(q)}`),
	requestJoinServer: (serverId: string) =>
		req<{ id: string }>('POST', `/servers/${serverId}/join-request`),
	listJoinRequests: (serverId: string) =>
		req<JoinRequest[]>('GET', `/servers/${serverId}/join-requests`),
	reviewJoinRequest: (serverId: string, requestId: string, action: 'approve' | 'decline') =>
		req<void>('PATCH', `/servers/${serverId}/join-requests/${requestId}`, { action }),
	getPresence: (serverId: string) => req<Record<string, string>>('GET', `/servers/${serverId}/presence`),

	// space roles
	listRoles: (serverId: string) => req<SpaceRole[]>('GET', `/servers/${serverId}/roles`),
	createRole: (serverId: string, data: { name: string; color: string }) =>
		req<SpaceRole>('POST', `/servers/${serverId}/roles`, data),
	updateRole: (serverId: string, roleId: string, data: { name?: string; color?: string }) =>
		req<SpaceRole>('PATCH', `/servers/${serverId}/roles/${roleId}`, data),
	deleteRole: (serverId: string, roleId: string) =>
		req<void>('DELETE', `/servers/${serverId}/roles/${roleId}`),
	assignRole: (serverId: string, userId: string, roleId: string) =>
		req<void>('POST', `/servers/${serverId}/members/${userId}/roles/${roleId}`),
	removeRole: (serverId: string, userId: string, roleId: string) =>
		req<void>('DELETE', `/servers/${serverId}/members/${userId}/roles/${roleId}`),
	listChannelPerms: (serverId: string, channelId: string) =>
		req<ChannelPerm[]>('GET', `/servers/${serverId}/channels/${channelId}/permissions`),
	setChannelPerm: (serverId: string, channelId: string, roleId: string, data: { can_view: boolean; can_write: boolean }) =>
		req<void>('PUT', `/servers/${serverId}/channels/${channelId}/permissions/${roleId}`, data),
	deleteChannelPerm: (serverId: string, channelId: string, roleId: string) =>
		req<void>('DELETE', `/servers/${serverId}/channels/${channelId}/permissions/${roleId}`),

	// space moderation
	kickMember: (serverId: string, userId: string) =>
		req<void>('DELETE', `/servers/${serverId}/members/${userId}`),
	banMember: (serverId: string, userId: string) =>
		req<void>('POST', `/servers/${serverId}/members/${userId}/ban`),
	unbanMember: (serverId: string, userId: string) =>
		req<void>('DELETE', `/servers/${serverId}/bans/${userId}`),
	listBans: (serverId: string) =>
		req<BannedUser[]>('GET', `/servers/${serverId}/bans`),

	// local auth
	register: (data: { username: string; password: string; invite_code?: string }) =>
		req<User>('POST', '/auth/register', data),
	localLogin: (data: { username: string; password: string }) =>
		req<User>('POST', '/auth/local-login', data),
	generateRegistrationInvite: () =>
		req<RegistrationInvite>('POST', '/registration-invite'),

	// instance admin
	getAdminSettings: () => req<AdminSettings>('GET', '/admin/settings'),
	updateAdminSettings: (data: Partial<AdminSettings>) => req<void>('PATCH', '/admin/settings', data),
	runRetention: () => req<{ status: string }>('POST', '/admin/retention/run'),
	listInstanceUsers: () => req<InstanceUser[]>('GET', '/admin/users'),
	banInstanceUser: (userId: string) => req<void>('POST', `/admin/users/${userId}/ban`),
	unbanInstanceUser: (userId: string) => req<void>('DELETE', `/admin/users/${userId}/ban`),
	listRegistrationInvites: () => req<RegistrationInvite[]>('GET', '/admin/registration-invites'),
	createRegistrationInvite: (data: { max_uses?: number; expires_in_days?: number }) =>
		req<RegistrationInvite>('POST', '/admin/registration-invites', data),
	deleteRegistrationInvite: (id: string) => req<void>('DELETE', `/admin/registration-invites/${id}`),

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
	role_color?: string;
	is_instance_admin?: boolean;
	created_at: string;
}

export interface AdminSettings {
	message_retention_days: string;
	inactive_space_retention_days: string;
	allow_user_space_creation?: string;
	max_video_size_mb?: string;
	[key: string]: string | undefined;
}

export interface InstanceConfig {
	allow_user_space_creation: boolean;
	max_video_size_mb: number;
	google_auth_enabled: boolean;
	local_auth_enabled: boolean;
	registration_mode: 'open' | 'invite' | 'closed';
}

export interface RegistrationInvite {
	id: string;
	code: string;
	max_uses: number | null;
	use_count: number;
	expires_at: string | null;
	created_at: string;
}

export interface ServerDiscovery {
	id: string;
	name: string;
	description: string;
	rules: string;
	icon_url: string;
	member_count: number;
	is_member: boolean;
	requires_request: boolean;
	has_pending_request: boolean;
}

export interface JoinRequest {
	id: string;
	server_id: string;
	user: User;
	status: string;
	created_at: string;
}

export interface BannedUser {
	user: User;
	banned_at: string;
}

export interface InstanceUser {
	id: string;
	display_name: string;
	email: string;
	avatar_url: string;
	instance_banned: boolean;
	created_at: string;
}

export interface Server {
	id: string;
	name: string;
	description: string;
	rules: string;
	icon_url: string;
	owner_id: string;
	is_public: boolean;
	show_in_discovery: boolean;
	invite_code: string;
	member_invites_enabled: boolean;
	member_invite_expiry_days: number;
	welcome_channel_id: string | null;
	welcome_message: string;
	role?: string;
	created_at: string;
}

export interface SpaceRole {
	id: string;
	server_id: string;
	name: string;
	color: string;
	is_everyone: boolean;
	position: number;
	created_at: string;
}

export interface ChannelPerm {
	role_id: string;
	role_name: string;
	color: string;
	is_everyone: boolean;
	can_view: boolean;
	can_write: boolean;
	has_override: boolean;
}

export interface Channel {
	id: string;
	server_id: string;
	name: string;
	description: string;
	type: 'text' | 'voice' | 'threads';
	position: number;
	unread_count: number;
	can_write: boolean;
	created_at: string;
}

export interface Thread {
	id: string;
	channel_id: string;
	title: string;
	created_by: User;
	locked: boolean;
	created_at: string;
	last_message_at: string;
	message_count: number;
}

export interface ThreadMessage {
	id: string;
	thread_id: string;
	author: User;
	content: string;
	reply_to?: { id: string; content: string; author_name: string };
	created_at: string;
	edited_at?: string;
}

export interface VoicePeer {
	user_id: string;
	display_name: string;
	avatar_url: string;
}

export interface MessageReply {
	id: string;
	content: string;
	author_name: string;
}

export interface Reaction {
	emoji: string;
	count: number;
	mine: boolean;
}

export interface Message {
	id: string;
	channel_id: string;
	author: User;
	content: string;
	reply_to?: MessageReply;
	reactions: Reaction[];
	edited_at?: string;
	created_at: string;
}

export interface DMConversation {
	id: string;
	other_user: User;
	unread_count: number;
	created_at: string;
	last_message_at: string;
}

export interface DirectMessage {
	id: string;
	conversation_id: string;
	sender: User;
	content: string;
	edited_at?: string;
	reactions: Reaction[];
	created_at: string;
}

export interface ServerMember {
	user: User;
	role: string;
	joined_at: string;
	space_roles: SpaceRole[];
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
