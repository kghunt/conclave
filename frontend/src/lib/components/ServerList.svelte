<script lang="ts">
	import { api, type Server } from '$lib/api';
	import { servers, activeServer, channels, activeChannel, activeDM, instanceConfig, currentUser, serverUnread, homeMode } from '$lib/stores';
	import ServerContextMenu from './ServerContextMenu.svelte';
	import SpaceBrowser from './SpaceBrowser.svelte';

	const canCreateSpace = $derived(
		$instanceConfig.allow_user_space_creation || $currentUser?.is_instance_admin
	);

	let showCreate = $state(false);
	let showBrowse = $state(false);
	let newName = $state('');
	let newDesc = $state('');
	let isPublic = $state(false);
	let inviteCode = $state('');
	let submitting = $state(false);
	let error = $state('');

	// Context menu
	let contextServer = $state<Server | null>(null);
	let menuX = $state(0);
	let menuY = $state(0);

	function openContextMenu(e: MouseEvent, s: Server) {
		e.preventDefault();
		contextServer = s;
		// Keep menu on screen
		menuX = Math.min(e.clientX, window.innerWidth - 220);
		menuY = Math.min(e.clientY, window.innerHeight - 300);
	}

	async function createServer() {
		if (!newName.trim() || submitting) return;
		submitting = true;
		error = '';
		try {
			const s = await api.createServer({ name: newName, description: newDesc, is_public: isPublic });
			servers.update((prev) => [...(prev ?? []), s]);
			selectServer(s.id);
			showCreate = false;
			newName = '';
			newDesc = '';
			isPublic = false;
		} catch (e: any) {
			error = e.message ?? 'Failed to create space';
		} finally {
			submitting = false;
		}
	}

	async function joinByInvite() {
		if (!inviteCode.trim() || submitting) return;
		submitting = true;
		error = '';
		try {
			const { server_id } = await api.joinByInvite(inviteCode.trim());
			const updated = await api.listServers();
			servers.set(updated);
			selectServer(server_id);
			inviteCode = '';
			showCreate = false;
		} catch (e: any) {
			error = e.message ?? 'Invalid or expired invite';
		} finally {
			submitting = false;
		}
	}

	async function selectServer(id: string) {
		const s = $servers.find((s) => s.id === id);
		if (!s) return;
		homeMode.set(false);
		activeDM.set(null);
		activeServer.set(s);
		activeChannel.set(null);
		channels.set([]);
	}

	function goHome() {
		homeMode.set(true);
		activeServer.set(null);
		activeChannel.set(null);
	}
</script>

<nav class="server-list">
	<button class="server-icon home-btn" class:active={$homeMode} title="Messages & Friends" onclick={goHome}>
		<svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
	</button>
	<div class="divider"></div>
	{#each $servers as s}
		<div class="server-wrap">
			<button
				class="server-icon"
				class:active={$activeServer?.id === s.id}
				title={s.name}
				onclick={() => selectServer(s.id)}
				oncontextmenu={(e) => openContextMenu(e, s)}
			>
				{#if s.icon_url}
					<img src={s.icon_url} alt={s.name} />
				{:else}
					{s.name.slice(0, 2).toUpperCase()}
				{/if}
			</button>
			{#if $serverUnread[s.id] && $activeServer?.id !== s.id}
				<span class="unread-dot"></span>
			{/if}
		</div>
	{/each}

	{#if canCreateSpace}
		<button class="server-icon add" title="Create or join a space" onclick={() => { showCreate = !showCreate; showBrowse = false; }}>
			+
		</button>
	{/if}
	<button class="server-icon browse" title="Browse public spaces" onclick={() => { showBrowse = true; showCreate = false; }}>
		⊕
	</button>
</nav>

{#if contextServer}
	<ServerContextMenu
		server={contextServer}
		x={menuX}
		y={menuY}
		onclose={() => (contextServer = null)}
	/>
{/if}

{#if showBrowse}
	<SpaceBrowser onclose={() => (showBrowse = false)} />
{/if}

{#if showCreate}
	<div class="create-panel">
		{#if error}
			<p class="error">{error}</p>
		{/if}

		<h3>Create Space</h3>
		<input bind:value={newName} placeholder="Space name" onkeydown={(e) => e.key === 'Enter' && createServer()} />
		<input bind:value={newDesc} placeholder="Description (optional)" />
		<label><input type="checkbox" bind:checked={isPublic} /> Public (anyone can join)</label>
		<button onclick={createServer} disabled={submitting || !newName.trim()}>
			{submitting ? 'Creating…' : 'Create'}
		</button>

		<h3>Join a Space</h3>
		<input bind:value={inviteCode} placeholder="Invite code" onkeydown={(e) => e.key === 'Enter' && joinByInvite()} />
		<button onclick={joinByInvite} disabled={submitting || !inviteCode.trim()}>
			{submitting ? 'Joining…' : 'Join'}
		</button>
	</div>
{/if}

<style>
	.server-wrap {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.unread-dot {
		position: absolute;
		bottom: -3px;
		left: 50%;
		transform: translateX(-50%);
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #e04545;
		pointer-events: none;
	}
	.server-list {
		width: 72px;
		background: #0e0e10;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 0.75rem 0;
		gap: 0.5rem;
		flex-shrink: 0;
		overflow-y: auto;
	}
	.server-icon {
		width: 48px;
		height: 48px;
		border-radius: 50%;
		background: var(--bg-panel);
		border: 2px solid transparent;
		color: var(--text);
		font-size: 0.85rem;
		font-weight: 700;
		cursor: pointer;
		transition: border-radius 0.15s, border-color 0.15s;
		overflow: hidden;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.server-icon img { width: 100%; height: 100%; object-fit: cover; }
	.server-icon:hover, .server-icon.active {
		border-radius: 30%;
		border-color: var(--accent);
	}
	.server-icon.add { background: #1a2d1a; color: #44c97d; font-size: 1.5rem; }
	.server-icon.browse { background: var(--bg-panel); color: var(--accent); font-size: 1.4rem; }
	.home-btn { background: var(--bg-panel); color: var(--text-muted); }
	.home-btn:hover, .home-btn.active { color: var(--accent); border-color: var(--accent); }
	.divider {
		width: 32px;
		height: 2px;
		background: var(--border);
		border-radius: 1px;
		margin: 0.25rem 0;
	}
	.create-panel {
		position: fixed;
		left: 80px;
		top: 80px;
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1rem;
		z-index: 100;
		width: 280px;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	@media (max-width: 767px) {
		.server-list {
			width: 100%;
			height: auto;
			flex-direction: row;
			padding: 0.5rem 0.75rem;
			gap: 0.5rem;
			overflow-x: auto;
			overflow-y: hidden;
			scrollbar-width: none;
		}
		.server-list::-webkit-scrollbar { display: none; }
		.server-icon { width: 44px; height: 44px; flex-shrink: 0; }
		.divider {
			width: 2px;
			height: 28px;
			align-self: center;
		}
		.create-panel {
			left: 0;
			right: 0;
			top: 60px;
			width: 100%;
			border-radius: 0 0 8px 8px;
			max-height: calc(100dvh - 60px);
			overflow-y: auto;
		}
	}
	.create-panel h3 { color: var(--text); margin-top: 0.5rem; font-size: 0.9rem; }
	.create-panel input:not([type="checkbox"]) {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem;
		border-radius: 4px;
		font-size: 0.9rem;
		width: 100%;
	}
	.create-panel label { color: #aaa; font-size: 0.85rem; display: flex; gap: 0.5rem; align-items: center; }
	.create-panel button {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.5rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.9rem;
	}
	.create-panel button:disabled { opacity: 0.5; cursor: not-allowed; }
	.error { color: #e04545; font-size: 0.8rem; background: rgba(224,69,69,0.1); padding: 0.4rem 0.5rem; border-radius: 4px; }
</style>
