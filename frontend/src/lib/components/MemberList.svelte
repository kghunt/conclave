<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type ServerMember } from '$lib/api';
	import { activeServer, currentUser } from '$lib/stores';
	import Avatar from './Avatar.svelte';

	let { serverId }: { serverId: string } = $props();

	let members: ServerMember[] = $state([]);
	let menuMember = $state<ServerMember | null>(null);
	let saving = $state(false);

	onMount(load);

	// Reload when server changes
	$effect(() => {
		serverId;
		load();
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
			<div class="member" class:clickable={isOwner && m.role !== 'owner' && m.user.id !== $currentUser?.id}>
				<Avatar url={m.user.avatar_url} name={m.user.display_name} userId={m.user.id} size={32} />
				<div class="member-info">
					<span class="member-name">{m.user.display_name}</span>
					<span class="role-badge role-{m.role}">
						{m.role === 'owner' ? '👑 Owner' : m.role === 'admin' ? '⚡ Admin' : 'Member'}
					</span>
				</div>
				{#if isOwner && m.role !== 'owner' && m.user.id !== $currentUser?.id}
					<button class="role-menu-btn" onclick={(e) => { e.stopPropagation(); menuMember = menuMember?.user.id === m.user.id ? null : m; }} title="Manage role">
						⋯
					</button>
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

	.role-menu-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 1rem;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		opacity: 0;
		transition: opacity 0.1s;
		line-height: 1;
	}
	.member:hover .role-menu-btn { opacity: 1; }
	.role-menu-btn:hover { background: rgba(255,255,255,0.1); color: #f0eff4; }

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
