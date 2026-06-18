<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ServerMember, type JoinRequest } from '$lib/api';
	import { activeServer, currentUser, activeDM, activeChannel, dmConversations, friends, pendingJoinRequests } from '$lib/stores';
	import { socket } from '$lib/socket';
	import Avatar from './Avatar.svelte';

	let { serverId, onDmStarted }: { serverId: string; onDmStarted?: () => void } = $props();

	let members: ServerMember[] = $state([]);
	let menuMember = $state<ServerMember | null>(null);
	let saving = $state(false);

	const isAdmin = $derived($activeServer?.role === 'owner' || $activeServer?.role === 'admin');
	const isOwner = $derived($activeServer?.role === 'owner');
	const isInstanceAdmin = $derived($currentUser?.is_instance_admin ?? false);

	onMount(load);

	// Subscribe to server-level room and reload on membership changes
	$effect(() => {
		const id = serverId;
		if (!id) return;
		const room = 'server:' + id;
		socket.subscribe(room);
		load();
		const unsub = socket.on((event) => {
			if (
				(event.type === 'member.join' || event.type === 'member.leave') &&
				event.payload.server_id === id
			) {
				load();
			}
			if (event.type === 'join_request.new' && event.payload.server_id === id && isAdmin) {
				pendingJoinRequests.update((prev) => {
					const exists = prev.find((r) => r.user?.id === event.payload.user?.id);
					if (exists) return prev;
					return [...prev, { id: event.payload.request_id, server_id: id, user: event.payload.user, status: 'pending', created_at: new Date().toISOString() }];
				});
			}
		});
		return () => {
			unsub();
			socket.unsubscribe(room);
		};
	});

	async function load() {
		members = await api.getMembers(serverId);
	}

	async function setRole(member: ServerMember, role: 'admin' | 'member') {
		if (saving) return;
		saving = true;
		try {
			await api.updateMemberRole(serverId, member.user.id, role);
			members = members.map((m) =>
				m.user.id === member.user.id ? { ...m, role } : m
			);
			// Update activeServer role if we changed our own (shouldn't happen — owners can't change own role)
		} finally {
			saving = false;
			menuMember = null;
		}
	}

	const roleOrder: Record<string, number> = { owner: 0, admin: 1, member: 2 };

	function grouped() {
		return ['owner', 'admin', 'member']
			.map((role) => ({ role, members: members.filter((m) => m.role === role) }))
			.filter((g) => g.members.length > 0);
	}

	const friendIds = $derived(new Set($friends.map((f) => f.user.id)));

	// Load join requests when current user is admin
	$effect(() => {
		if (isAdmin && serverId) {
			api.listJoinRequests(serverId).then((reqs) => {
				pendingJoinRequests.set(reqs ?? []);
			}).catch(() => pendingJoinRequests.set([]));
		} else {
			pendingJoinRequests.set([]);
		}
	});

	async function reviewRequest(req: JoinRequest, action: 'approve' | 'decline') {
		await api.reviewJoinRequest(serverId, req.id, action);
		pendingJoinRequests.update((prev) => prev.filter((r) => r.id !== req.id));
		if (action === 'approve') load();
	}

	async function kickMember(m: ServerMember) {
		if (saving) return;
		saving = true;
		try {
			await api.kickMember(serverId, m.user.id);
			members = members.filter((mem) => mem.user.id !== m.user.id);
		} finally {
			saving = false;
			menuMember = null;
		}
	}

	async function banMember(m: ServerMember) {
		if (saving) return;
		saving = true;
		try {
			await api.banMember(serverId, m.user.id);
			members = members.filter((mem) => mem.user.id !== m.user.id);
		} finally {
			saving = false;
			menuMember = null;
		}
	}

	async function banFromInstance(m: ServerMember) {
		if (saving) return;
		saving = true;
		try {
			await api.banInstanceUser(m.user.id);
			members = members.filter((mem) => mem.user.id !== m.user.id);
		} finally {
			saving = false;
			menuMember = null;
		}
	}

	let addFriendState = $state<Record<string, 'sending' | 'sent' | 'error'>>({});

	async function addFriend(userId: string) {
		addFriendState = { ...addFriendState, [userId]: 'sending' };
		try {
			const result = await api.sendFriendRequest(userId);
			addFriendState = { ...addFriendState, [userId]: 'sent' };
			if (result.status === 'accepted') {
				const fr = await api.listFriends();
				friends.set(fr ?? []);
			}
		} catch (e: any) {
			// "request already sent" means it worked before — show sent, not error
			if (e?.message?.includes('already sent') || e?.message?.includes('already friends')) {
				addFriendState = { ...addFriendState, [userId]: 'sent' };
			} else {
				console.error('addFriend failed:', e);
				addFriendState = { ...addFriendState, [userId]: 'error' };
				setTimeout(() => {
					const { [userId]: _, ...rest } = addFriendState;
					addFriendState = rest;
				}, 2000);
			}
		}
	}

	async function startDM(member: ServerMember) {
		const conv = await api.getOrCreateDM(member.user.id);
		dmConversations.update((prev) => {
			if (prev.find((c) => c.id === conv.id)) return prev;
			return [conv, ...prev];
		});
		activeChannel.set(null);
		activeDM.set(conv);
		onDmStarted?.();
	}
</script>

<!-- close role menu on outside click -->
{#if menuMember}
	<div class="overlay" onclick={() => (menuMember = null)}></div>
{/if}

<aside class="member-list">
	<div class="list-header">Members</div>

	{#if isAdmin && $pendingJoinRequests.length > 0}
		<div class="requests-section">
			<div class="requests-header">Join Requests — {$pendingJoinRequests.length}</div>
			{#each $pendingJoinRequests as req}
				<div class="request-row">
					<Avatar url={req.user.avatar_url} name={req.user.display_name} size={28} />
					<span class="request-name">{req.user.display_name}</span>
					<button class="req-btn approve" onclick={() => reviewRequest(req, 'approve')} title="Approve">✓</button>
					<button class="req-btn decline" onclick={() => reviewRequest(req, 'decline')} title="Decline">✕</button>
				</div>
			{/each}
		</div>
	{/if}

	{#each grouped() as group}
		<div class="role-header">
			{group.role === 'owner' ? '👑' : group.role === 'admin' ? '⚡' : ''}
			{group.role}s — {group.members.length}
		</div>
		{#each group.members as m}
			<div class="member">
				<Avatar url={m.user.avatar_url} name={m.user.display_name} userId={m.user.id} size={32} showPresence />
				<div class="member-info">
					<span class="member-name">{m.user.display_name}</span>
					<span class="role-badge role-{m.role}">
						{m.role === 'owner' ? '👑 Owner' : m.role === 'admin' ? '⚡ Admin' : 'Member'}
					</span>
				</div>
				{#if m.user.id !== $currentUser?.id}
					{@const reqState = addFriendState[m.user.id]}
					<div class="member-actions">
						{#if friendIds.has(m.user.id)}
							<button class="action-btn" onclick={() => startDM(m)} title="Send message">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
							</button>
						{/if}
						{#if !friendIds.has(m.user.id)}
							<button
								class="action-btn"
								onclick={() => addFriend(m.user.id)}
								disabled={reqState === 'sending' || reqState === 'sent'}
								data-state={reqState ?? 'idle'}
								title={reqState === 'sent' ? 'Request sent' : reqState === 'error' ? 'Failed — try again' : 'Add friend'}
							>
								{#if reqState === 'sent'}
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#44c97d" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
								{:else if reqState === 'error'}
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#e04545" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								{:else}
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" y1="8" x2="19" y2="14"/><line x1="22" y1="11" x2="16" y2="11"/></svg>
								{/if}
							</button>
						{/if}
						{#if (isAdmin && m.role !== 'owner') || isInstanceAdmin}
							<button class="action-btn" onclick={(e) => { e.stopPropagation(); menuMember = menuMember?.user.id === m.user.id ? null : m; }} title="Manage">
								⋯
							</button>
						{/if}
					</div>
					{#if menuMember?.user.id === m.user.id}
						<div class="role-menu">
							{#if isOwner && m.role !== 'owner'}
								{#if m.role === 'member'}
									<button onclick={() => setRole(m, 'admin')} disabled={saving}>⚡ Promote to Admin</button>
								{:else if m.role === 'admin'}
									<button onclick={() => setRole(m, 'member')} disabled={saving}>Remove Admin</button>
								{/if}
							{/if}
							{#if isAdmin && m.role !== 'owner' && !(isAdmin && !isOwner && m.role === 'admin')}
								<div class="role-menu-divider"></div>
								<button onclick={() => kickMember(m)} disabled={saving}>Kick from Space</button>
								<button class="danger" onclick={() => banMember(m)} disabled={saving}>Ban from Space</button>
							{/if}
							{#if isInstanceAdmin}
								<div class="role-menu-divider"></div>
								<button class="danger" onclick={() => banFromInstance(m)} disabled={saving}>Ban from Instance</button>
							{/if}
						</div>
					{/if}
				{/if}
			</div>
		{/each}
	{/each}
</aside>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 49;
	}
	.member-list {
		width: 240px;
		background: var(--bg-sidebar);
		flex-shrink: 0;
		overflow-y: auto;
		padding: 0;
		display: flex;
		flex-direction: column;
	}
	.list-header {
		padding: 0.875rem 1rem;
		font-weight: 700;
		font-size: 0.95rem;
		border-bottom: 1px solid #0e0e10;
	}
	.role-header {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.05em;
		padding: 0.875rem 0.75rem 0.3rem;
	}
	.member {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.375rem 0.75rem;
		border-radius: 4px;
		margin: 0 0.25rem;
		position: relative;
	}
	.member:hover { background: rgba(255,255,255,0.05); }
	.member-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}
	.member-name {
		font-size: 0.875rem;
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.role-badge {
		font-size: 0.65rem;
		font-weight: 600;
	}
	.role-owner { color: #f0a020; }
	.role-admin { color: var(--accent); }
	.role-member { color: var(--text-muted); }

	.member-actions {
		display: flex;
		gap: 0.125rem;
		opacity: 0;
		transition: opacity 0.1s;
		flex-shrink: 0;
	}
	.member:hover .member-actions { opacity: 1; }
	/* always show sent/error feedback even when not hovering */
	.member-actions:has(.action-btn[data-state="sent"]),
	.member-actions:has(.action-btn[data-state="error"]) { opacity: 1; }
	.action-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		line-height: 1;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.action-btn:hover { background: rgba(255,255,255,0.1); color: var(--text); }
	@media (max-width: 767px) {
		.member-actions { opacity: 1; }
		.action-btn { padding: 0.35rem; }
	}

	.requests-section {
		border-bottom: 1px solid #0e0e10;
		padding-bottom: 0.5rem;
		margin-bottom: 0.25rem;
	}
	.requests-header {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--accent);
		letter-spacing: 0.05em;
		padding: 0.875rem 0.75rem 0.3rem;
	}
	.request-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0.75rem;
	}
	.request-name {
		flex: 1;
		font-size: 0.875rem;
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.req-btn {
		background: none;
		border: 1px solid var(--border);
		border-radius: 3px;
		cursor: pointer;
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.75rem;
		flex-shrink: 0;
	}
	.req-btn.approve { color: #3ba55c; border-color: #3ba55c; }
	.req-btn.approve:hover { background: rgba(59,165,92,0.15); }
	.req-btn.decline { color: #e04545; border-color: #e04545; }
	.req-btn.decline:hover { background: rgba(224,69,69,0.15); }
	.role-menu {
		position: absolute;
		right: 0.5rem;
		top: 100%;
		background: #222228;
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.3rem;
		z-index: 50;
		min-width: 180px;
		box-shadow: 0 4px 16px rgba(0,0,0,0.4);
	}
	.role-menu button {
		display: block;
		width: 100%;
		background: none;
		border: none;
		color: var(--text);
		padding: 0.5rem 0.625rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
		font-size: 0.875rem;
	}
	.role-menu button.danger { color: #e04545; }
	.role-menu button:hover:not(:disabled) { background: rgba(255,255,255,0.08); }
	.role-menu button.danger:hover:not(:disabled) { background: rgba(224,69,69,0.1); }
	.role-menu button:disabled { opacity: 0.5; cursor: not-allowed; }
	.role-menu-divider {
		height: 1px;
		background: var(--border);
		margin: 0.2rem 0;
	}
</style>
