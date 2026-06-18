<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ServerMember } from '$lib/api';
	import { activeServer, currentUser, activeDM, activeChannel, dmConversations, friends } from '$lib/stores';
	import { socket } from '$lib/socket';
	import Avatar from './Avatar.svelte';

	let { serverId, onDmStarted }: { serverId: string; onDmStarted?: () => void } = $props();

	let members: ServerMember[] = $state([]);
	let menuMember = $state<ServerMember | null>(null);
	let saving = $state(false);

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

	const isOwner = $derived($activeServer?.role === 'owner');

	let addFriendState = $state<Record<string, 'idle' | 'sending' | 'sent' | 'friends'>>({});

	$effect(() => {
		const ids = new Set($friends.map((f) => f.user.id));
		const next: Record<string, 'idle' | 'sending' | 'sent' | 'friends'> = {};
		members.forEach((m) => {
			if (ids.has(m.user.id)) next[m.user.id] = 'friends';
		});
		addFriendState = next;
	});

	async function addFriend(userId: string) {
		addFriendState = { ...addFriendState, [userId]: 'sending' };
		try {
			await api.sendFriendRequest(userId);
			addFriendState = { ...addFriendState, [userId]: 'sent' };
			const fr = await api.listFriends();
			friends.set(fr ?? []);
		} catch {
			addFriendState = { ...addFriendState, [userId]: 'idle' };
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

	{#each grouped() as group}
		<div class="role-header">
			{group.role === 'owner' ? '👑' : group.role === 'admin' ? '⚡' : ''}
			{group.role}s — {group.members.length}
		</div>
		{#each group.members as m}
			<div class="member">
				<Avatar url={m.user.avatar_url} name={m.user.display_name} userId={m.user.id} size={32} />
				<div class="member-info">
					<span class="member-name">{m.user.display_name}</span>
					<span class="role-badge role-{m.role}">
						{m.role === 'owner' ? '👑 Owner' : m.role === 'admin' ? '⚡ Admin' : 'Member'}
					</span>
				</div>
				{#if m.user.id !== $currentUser?.id}
					{@const fs = addFriendState[m.user.id] ?? 'idle'}
					<div class="member-actions">
						<button class="action-btn" onclick={() => startDM(m)} title="Send message">
							<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
						</button>
						{#if fs !== 'friends'}
							<button
								class="action-btn"
								onclick={() => addFriend(m.user.id)}
								disabled={fs === 'sending' || fs === 'sent'}
								title={fs === 'sent' ? 'Request sent' : 'Add friend'}
							>
								{#if fs === 'sent'}
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#44c97d" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
								{:else}
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" y1="8" x2="19" y2="14"/><line x1="22" y1="11" x2="16" y2="11"/></svg>
								{/if}
							</button>
						{/if}
						{#if isOwner && m.role !== 'owner'}
							<button class="action-btn" onclick={(e) => { e.stopPropagation(); menuMember = menuMember?.user.id === m.user.id ? null : m; }} title="Manage role">
								⋯
							</button>
						{/if}
					</div>
					{#if menuMember?.user.id === m.user.id}
						<div class="role-menu">
							{#if m.role === 'member'}
								<button onclick={() => setRole(m, 'admin')} disabled={saving}>
									⚡ Promote to Admin
								</button>
							{:else if m.role === 'admin'}
								<button onclick={() => setRole(m, 'member')} disabled={saving}>
									Remove Admin
								</button>
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
		background: #19191d;
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
		color: #8b8b99;
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
		color: #f0eff4;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.role-badge {
		font-size: 0.65rem;
		font-weight: 600;
	}
	.role-owner { color: #f0a020; }
	.role-admin { color: #e8541e; }
	.role-member { color: #8b8b99; }

	.member-actions {
		display: flex;
		gap: 0.125rem;
		opacity: 0;
		transition: opacity 0.1s;
		flex-shrink: 0;
	}
	.member:hover .member-actions { opacity: 1; }
	.action-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		line-height: 1;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.action-btn:hover { background: rgba(255,255,255,0.1); color: #f0eff4; }
	@media (max-width: 767px) {
		.member-actions { opacity: 1; }
		.action-btn { padding: 0.35rem; }
	}

	.role-menu {
		position: absolute;
		right: 0.5rem;
		top: 100%;
		background: #222228;
		border: 1px solid #2e2e38;
		border-radius: 6px;
		padding: 0.3rem;
		z-index: 50;
		min-width: 160px;
		box-shadow: 0 4px 16px rgba(0,0,0,0.4);
	}
	.role-menu button {
		display: block;
		width: 100%;
		background: none;
		border: none;
		color: #f0eff4;
		padding: 0.5rem 0.625rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
		font-size: 0.875rem;
	}
	.role-menu button:hover:not(:disabled) { background: rgba(255,255,255,0.08); }
	.role-menu button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
