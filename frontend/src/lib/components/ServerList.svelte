<script lang="ts">
	import { api, type Server } from '$lib/api';
	import { servers, activeServer, channels, activeChannel } from '$lib/stores';
	import ServerContextMenu from './ServerContextMenu.svelte';

	let showCreate = $state(false);
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
			servers.update((prev) => [...prev, s]);
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
		activeServer.set(s);
		activeChannel.set(null);
		channels.set([]);
	}
</script>

<nav class="server-list">
	{#each $servers as s}
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
	{/each}

	<div class="divider"></div>

	<button class="server-icon add" title="Create or join a space" onclick={() => (showCreate = !showCreate)}>
		+
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
		background: #1c1c21;
		border: 2px solid transparent;
		color: #f0eff4;
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
		border-color: #e8541e;
	}
	.server-icon.add { background: #1a2d1a; color: #44c97d; font-size: 1.5rem; }
	.divider {
		width: 32px;
		height: 2px;
		background: #2e2e38;
		border-radius: 1px;
		margin: 0.25rem 0;
	}
	.create-panel {
		position: fixed;
		left: 80px;
		top: 80px;
		background: #1c1c21;
		border: 1px solid #2e2e38;
		border-radius: 8px;
		padding: 1rem;
		z-index: 100;
		width: 280px;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.create-panel h3 { color: #f0eff4; margin-top: 0.5rem; font-size: 0.9rem; }
	.create-panel input:not([type="checkbox"]) {
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.5rem;
		border-radius: 4px;
		font-size: 0.9rem;
		width: 100%;
	}
	.create-panel label { color: #aaa; font-size: 0.85rem; display: flex; gap: 0.5rem; align-items: center; }
	.create-panel button {
		background: #e8541e;
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
