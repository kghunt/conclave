<script lang="ts">
	import { api, type Server, type Invite } from '$lib/api';
	import { servers, activeServer, channels, activeChannel } from '$lib/stores';

	let {
		server,
		x,
		y,
		onclose
	}: { server: Server; x: number; y: number; onclose: () => void } = $props();

	const isAdmin = server.role === 'owner' || server.role === 'admin';

	// Edit modal state
	let showEdit = $state(false);
	let editName = $state(server.name);
	let editDesc = $state(server.description);
	let editPublic = $state(server.is_public);
	let saving = $state(false);

	// Invite state
	let generatedInvite = $state<Invite | null>(null);
	let copied = $state(false);

	// Icon upload
	let iconInput: HTMLInputElement;

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);

	async function saveEdit() {
		if (saving) return;
		saving = true;
		try {
			const updated = await api.updateServer(server.id, {
				name: editName,
				description: editDesc,
				is_public: editPublic
			});
			servers.update((prev) => prev.map((s) => s.id === updated.id ? { ...s, ...updated } : s));
			if ($activeServer?.id === updated.id) activeServer.set({ ...$activeServer, ...updated });
			showEdit = false;
			onclose();
		} finally {
			saving = false;
		}
	}

	async function generateInvite() {
		const invite = await api.createInvite(server.id);
		generatedInvite = invite;
	}

	async function copyInvite() {
		if (!generatedInvite) return;
		const link = `${location.origin}/invite/${generatedInvite.code}`;
		await navigator.clipboard.writeText(link);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	async function uploadIcon(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		const { icon_url } = await api.uploadServerIcon(server.id, file);
		servers.update((prev) => prev.map((s) => s.id === server.id ? { ...s, icon_url } : s));
		if ($activeServer?.id === server.id) activeServer.update((s) => s ? { ...s, icon_url } : s);
		onclose();
	}

	async function leaveServer() {
		await api.leaveServer(server.id);
		servers.update((prev) => prev.filter((s) => s.id !== server.id));
		if ($activeServer?.id === server.id) {
			activeServer.set(null);
			activeChannel.set(null);
			channels.set([]);
		}
		onclose();
	}

	async function deleteServer() {
		if (deleteConfirmName !== server.name || deleting) return;
		deleting = true;
		try {
			await api.deleteServer(server.id);
			servers.update((prev) => prev.filter((s) => s.id !== server.id));
			if ($activeServer?.id === server.id) {
				activeServer.set(null);
				activeChannel.set(null);
				channels.set([]);
			}
			onclose();
		} finally {
			deleting = false;
		}
	}
</script>

<!-- click-outside overlay -->
<div class="overlay" onclick={onclose}></div>

{#if showDeleteConfirm}
	<div class="menu edit-menu" style="left:{x}px; top:{y}px">
		<div class="menu-header">Delete Space</div>
		<p class="delete-warning">Permanently deletes <strong>{server.name}</strong> and all its channels and messages. This cannot be undone.</p>
		<label>
			Type the space name to confirm
			<input
				bind:value={deleteConfirmName}
				placeholder={server.name}
				onkeydown={(e) => e.key === 'Enter' && deleteServer()}
			/>
		</label>
		<div class="edit-actions">
			<button class="cancel" onclick={() => (showDeleteConfirm = false)}>Back</button>
			<button
				class="delete-btn"
				onclick={deleteServer}
				disabled={deleteConfirmName !== server.name || deleting}
			>
				{deleting ? 'Deleting…' : 'Delete'}
			</button>
		</div>
	</div>
{:else if showEdit}
	<div class="menu edit-menu" style="left:{x}px; top:{y}px">
		<div class="menu-header">Edit Space</div>
		<label>
			Name
			<input bind:value={editName} />
		</label>
		<label>
			Description
			<textarea bind:value={editDesc} rows="2"></textarea>
		</label>
		<label class="checkbox-label">
			<input type="checkbox" bind:checked={editPublic} />
			Public (anyone can join)
		</label>
		<div class="edit-actions">
			<button class="cancel" onclick={() => (showEdit = false)}>Back</button>
			<button class="save" onclick={saveEdit} disabled={saving || !editName.trim()}>
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>
	</div>
{:else}
	<div class="menu" style="left:{x}px; top:{y}px">
		<div class="menu-header">{server.name}</div>

		{#if isAdmin}
			<button onclick={() => (showEdit = true)}>Edit Space</button>
			<button onclick={() => iconInput.click()}>Upload Icon</button>
			<input bind:this={iconInput} type="file" accept="image/*" onchange={uploadIcon} style="display:none" />
		{/if}

		{#if isAdmin}
			{#if !generatedInvite}
				<button onclick={generateInvite}>Generate Invite Link</button>
			{:else}
				<div class="invite-row">
					<span class="invite-code">{generatedInvite.code}</span>
					<button class="copy-btn" onclick={copyInvite}>{copied ? '✓ Copied' : 'Copy'}</button>
				</div>
			{/if}
		{/if}

		{#if server.role !== 'owner'}
			<button class="danger" onclick={leaveServer}>Leave Space</button>
		{/if}

		{#if server.role === 'owner'}
			<div class="separator"></div>
			<button class="danger" onclick={() => { deleteConfirmName = ''; showDeleteConfirm = true; }}>Delete Space</button>
		{/if}
	</div>
{/if}

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 199;
	}
	.menu {
		position: fixed;
		z-index: 200;
		background: #222228;
		border: 1px solid #2e2e38;
		border-radius: 6px;
		padding: 0.375rem;
		min-width: 200px;
		box-shadow: 0 8px 24px rgba(0,0,0,0.5);
	}
	.menu-header {
		padding: 0.375rem 0.625rem 0.5rem;
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		color: #8b8b99;
		letter-spacing: 0.05em;
		border-bottom: 1px solid #2e2e38;
		margin-bottom: 0.25rem;
	}
	.menu button {
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
	.menu button:hover { background: rgba(255,255,255,0.08); }
	.menu button.danger { color: #e04545; }
	.menu button.danger:hover { background: rgba(224,69,69,0.1); }
	.separator {
		height: 1px;
		background: #2e2e38;
		margin: 0.25rem 0;
	}

	.invite-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.625rem;
	}
	.invite-code {
		flex: 1;
		font-family: monospace;
		font-size: 0.85rem;
		color: #e8541e;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.copy-btn {
		background: #e8541e !important;
		color: white !important;
		padding: 0.2rem 0.5rem !important;
		font-size: 0.75rem !important;
		border-radius: 3px !important;
	}

	/* Edit / delete forms */
	.edit-menu { min-width: 260px; }
	label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		padding: 0.375rem 0.625rem;
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		color: #8b8b99;
		letter-spacing: 0.05em;
	}
	label input:not([type="checkbox"]), label textarea {
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.4rem 0.5rem;
		border-radius: 4px;
		font-size: 0.875rem;
		font-family: inherit;
		resize: none;
		outline: none;
	}
	.checkbox-label {
		flex-direction: row !important;
		align-items: center;
		gap: 0.5rem !important;
		cursor: pointer;
	}
	.delete-warning {
		padding: 0.375rem 0.625rem;
		font-size: 0.8rem;
		color: #aaa;
		line-height: 1.5;
	}
	.delete-warning strong { color: #f0eff4; }
	.edit-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		padding: 0.5rem 0.625rem 0.25rem;
	}
	.cancel {
		background: none !important;
		border: none;
		color: #8b8b99 !important;
		cursor: pointer;
		padding: 0.4rem 0.75rem !important;
		border-radius: 4px;
	}
	.save {
		background: #e8541e !important;
		border: none;
		color: white !important;
		padding: 0.4rem 0.875rem !important;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.875rem;
	}
	.save:disabled { opacity: 0.5; cursor: not-allowed; }
	.delete-btn {
		background: #e04545 !important;
		border: none;
		color: white !important;
		padding: 0.4rem 0.875rem !important;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
	}
	.delete-btn:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
