<script lang="ts">
	import { api } from '$lib/api';
	import { activeServer, servers, channels, activeChannel, dmConversations, activeDM, currentUser, showProfileModal } from '$lib/stores';
	import type { Channel } from '$lib/api';
	import Avatar from './Avatar.svelte';
	import AdminPanel from './AdminPanel.svelte';

	let showAdmin = $state(false);

	let showNewChannel = $state(false);
	let newChannelName = $state('');
	let showDeleteConfirm = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);

	function selectChannel(ch: Channel) {
		activeDM.set(null);
		activeChannel.set(ch);
	}

	async function createChannel() {
		if (!newChannelName.trim() || !$activeServer) return;
		const ch = await api.createChannel($activeServer.id, { name: newChannelName, description: '' });
		channels.update((prev) => [...prev, ch]);
		selectChannel(ch);
		showNewChannel = false;
		newChannelName = '';
	}

	async function openDM(userId: string) {
		const conv = await api.getOrCreateDM(userId);
		dmConversations.update((prev) => {
			if (prev.find((c) => c.id === conv.id)) return prev;
			return [conv, ...prev];
		});
		activeDM.set(conv);
	}

	async function deleteServer() {
		if (!$activeServer || deleteConfirmName !== $activeServer.name || deleting) return;
		deleting = true;
		try {
			await api.deleteServer($activeServer.id);
			servers.update((prev) => prev.filter((s) => s.id !== $activeServer!.id));
			activeServer.set(null);
			activeChannel.set(null);
			channels.set([]);
			showDeleteConfirm = false;
			deleteConfirmName = '';
		} finally {
			deleting = false;
		}
	}

	async function logout() {
		await api.logout();
		location.href = '/login';
	}
</script>

<aside class="sidebar">
	{#if $activeServer}
		<div class="server-header">
			<span>{$activeServer.name}</span>
			{#if $activeServer.role === 'owner'}
				<button class="settings-btn" onclick={() => { showDeleteConfirm = true; deleteConfirmName = ''; }} title="Space settings">⚙</button>
			{/if}
		</div>

		<div class="section-label">
			<span>Channels</span>
			{#if $activeServer.role === 'owner' || $activeServer.role === 'admin'}
				<button class="add-btn" onclick={() => (showNewChannel = !showNewChannel)}>+</button>
			{/if}
		</div>

		{#if showNewChannel}
			<div class="new-channel">
				<input
					bind:value={newChannelName}
					placeholder="channel-name"
					onkeydown={(e) => e.key === 'Enter' && createChannel()}
				/>
				<button onclick={createChannel}>Add</button>
			</div>
		{/if}

		{#each $channels as ch}
			<button
				class="channel-item"
				class:active={$activeChannel?.id === ch.id}
				onclick={() => selectChannel(ch)}
			>
				<span># {ch.name}</span>
				{#if ch.unread_count > 0}
					<span class="badge">{ch.unread_count}</span>
				{/if}
			</button>
		{/each}
	{:else}
		<div class="server-header"><span>Direct Messages</span></div>
	{/if}

	<div class="section-label" style="margin-top: auto">Direct Messages</div>
	{#each $dmConversations as conv}
		<button
			class="channel-item"
			class:active={$activeDM?.id === conv.id}
			onclick={() => activeDM.set(conv)}
		>
			<Avatar url={conv.other_user.avatar_url} name={conv.other_user.display_name} userId={conv.other_user.id} size={20} />
			{conv.other_user.display_name}
		</button>
	{/each}

	<div class="user-bar">
		{#if $currentUser}
			<button class="user-info" onclick={() => showProfileModal.set(true)} title="Edit profile">
				<Avatar url={$currentUser.avatar_url} name={$currentUser.display_name} userId={$currentUser.id} size={32} />
				<span class="username">{$currentUser.display_name}</span>
			</button>
			{#if $currentUser?.is_instance_admin}
				<button class="admin-btn" onclick={() => (showAdmin = true)} title="Instance admin">⚙</button>
			{/if}
			<button class="logout-btn" onclick={logout} title="Logout">⏻</button>
		{/if}
	</div>
</aside>

{#if showDeleteConfirm && $activeServer}
	<div class="modal-overlay" onclick={() => (showDeleteConfirm = false)}>
		<div class="modal" onclick={(e) => e.stopPropagation()}>
			<h3>Delete Space</h3>
			<p>This will permanently delete <strong>{$activeServer.name}</strong> and all its channels and messages. This cannot be undone.</p>
			<p class="confirm-label">Type the space name to confirm:</p>
			<input
				bind:value={deleteConfirmName}
				placeholder={$activeServer.name}
				onkeydown={(e) => e.key === 'Enter' && deleteServer()}
			/>
			<div class="modal-actions">
				<button class="cancel-btn" onclick={() => (showDeleteConfirm = false)}>Cancel</button>
				<button
					class="delete-btn"
					onclick={deleteServer}
					disabled={deleteConfirmName !== $activeServer.name || deleting}
				>
					{deleting ? 'Deleting…' : 'Delete Space'}
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showAdmin}
	<AdminPanel onclose={() => (showAdmin = false)} />
{/if}

<style>
	.sidebar {
		width: 240px;
		background: #19191d;
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		overflow-y: auto;
	}
	.server-header {
		padding: 0.875rem 1rem;
		font-weight: 700;
		border-bottom: 1px solid #0e0e10;
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 0.95rem;
	}
	.section-label {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 0.75rem 0.25rem;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: #8b8b99;
		letter-spacing: 0.05em;
	}
	.add-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 1rem;
		padding: 0 0.25rem;
	}
	.add-btn:hover { color: #f0eff4; }
	.new-channel {
		display: flex;
		gap: 0.25rem;
		padding: 0.25rem 0.75rem;
	}
	.new-channel input {
		flex: 1;
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.new-channel button {
		background: #e8541e;
		border: none;
		color: white;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.85rem;
	}
	.channel-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: none;
		border: none;
		color: #8b8b99;
		padding: 0.375rem 0.75rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
		margin: 0 0.25rem;
		width: calc(100% - 0.5rem);
		font-size: 0.9rem;
	}
	.channel-item:hover, .channel-item.active {
		background: rgba(255,255,255,0.07);
		color: #f0eff4;
	}
	.badge {
		margin-left: auto;
		background: #e04545;
		color: white;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.4rem;
	}
	.user-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 0.75rem;
		background: #0e0e10;
		margin-top: auto;
		flex-shrink: 0;
	}
	.username {
		font-size: 0.85rem;
		font-weight: 600;
		color: #f0eff4;
		flex: 1;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.logout-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 1rem;
	}
	.logout-btn:hover { color: #e04545; }
	.admin-btn {
		background: none;
		border: none;
		color: #e8541e;
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.15rem 0.25rem;
		border-radius: 3px;
		opacity: 0.8;
	}
	.admin-btn:hover { opacity: 1; background: rgba(232,84,30,0.15); }
	.user-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: 1;
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.25rem;
		border-radius: 4px;
		min-width: 0;
	}
	.user-info:hover { background: rgba(255,255,255,0.07); }
	.settings-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.1rem 0.25rem;
		border-radius: 3px;
		opacity: 0;
		transition: opacity 0.15s;
	}
	.server-header:hover .settings-btn { opacity: 1; }
	.settings-btn:hover { color: #f0eff4; background: rgba(255,255,255,0.1); }

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.75);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}
	.modal {
		background: #1c1c21;
		border: 1px solid #2e2e38;
		border-radius: 8px;
		padding: 1.5rem;
		width: 420px;
		display: flex;
		flex-direction: column;
		gap: 0.875rem;
	}
	.modal h3 { color: #e04545; font-size: 1.1rem; }
	.modal p { color: #aaa; font-size: 0.9rem; line-height: 1.5; }
	.modal strong { color: #f0eff4; }
	.confirm-label { color: #8b8b99 !important; font-size: 0.8rem !important; margin-bottom: -0.5rem; }
	.modal input {
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.5rem;
		border-radius: 4px;
		font-size: 0.9rem;
		width: 100%;
		outline: none;
	}
	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	.cancel-btn {
		background: none;
		border: none;
		color: #aaa;
		cursor: pointer;
		padding: 0.5rem 1rem;
		border-radius: 4px;
	}
	.cancel-btn:hover { color: #f0eff4; }
	.delete-btn {
		background: #e04545;
		border: none;
		color: white;
		padding: 0.5rem 1.25rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.9rem;
		font-weight: 600;
	}
	.delete-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.delete-btn:not(:disabled):hover { background: #c43333; }
</style>
