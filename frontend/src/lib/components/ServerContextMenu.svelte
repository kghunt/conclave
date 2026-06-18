<script lang="ts">
	import { api, type Server, type Invite } from '$lib/api';
	import { servers, activeServer, channels, activeChannel, currentUser } from '$lib/stores';
	import RoleManager from './RoleManager.svelte';

	let {
		server,
		x,
		y,
		onclose
	}: { server: Server; x: number; y: number; onclose: () => void } = $props();

	const isAdmin = server.role === 'owner' || server.role === 'admin';
	const canDeleteSpace = $derived(server.role === 'owner' || $currentUser?.is_instance_admin);

	// Edit modal state
	let showEdit = $state(false);
	let editName = $state(server.name);
	let editDesc = $state(server.description);
	let editRules = $state(server.rules ?? '');
	let editPublic = $state(server.is_public);
	let editShowInDiscovery = $state(server.show_in_discovery ?? false);
	let editMemberInvites = $state(server.member_invites_enabled);
	let editExpiryDays = $state(server.member_invite_expiry_days ?? 7);
	let saving = $state(false);

	// Invite state
	let generatedInvite = $state<Invite | null>(null);
	let copied = $state(false);

	// Icon upload
	let iconInput: HTMLInputElement;

	// Delete state
	let showRoleManager = $state(false);
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
				rules: editRules,
				is_public: editPublic,
				show_in_discovery: editShowInDiscovery,
				member_invites_enabled: editMemberInvites,
				member_invite_expiry_days: editExpiryDays
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

	function formatExpiry(expiresAt: string): string {
		const days = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 86400000);
		if (days <= 0) return 'soon';
		return `in ${days} day${days === 1 ? '' : 's'}`;
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

{#if showRoleManager}
	<RoleManager serverId={server.id} onclose={() => { showRoleManager = false; onclose(); }} />
{/if}

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
		<div class="section-divider">Space Rules</div>
		<label>
			Rules (shown to users before they join)
			<textarea bind:value={editRules} rows="5" placeholder="Enter your space rules here…"></textarea>
		</label>
		<label class="checkbox-label">
			<input type="checkbox" bind:checked={editPublic} />
			Public (anyone can join)
		</label>
		{#if !editPublic}
			<label class="checkbox-label">
				<input type="checkbox" bind:checked={editShowInDiscovery} />
				Show in space browser (invite-only, users can request to join)
			</label>
		{/if}
		<div class="section-divider">Member Invites</div>
		<label class="checkbox-label">
			<input type="checkbox" bind:checked={editMemberInvites} />
			Allow members to create invite links
		</label>
		{#if editMemberInvites}
			<label>
				Member invite expires after
				<select bind:value={editExpiryDays}>
					<option value={1}>1 day</option>
					<option value={3}>3 days</option>
					<option value={7}>7 days</option>
					<option value={14}>14 days</option>
					<option value={30}>30 days</option>
				</select>
			</label>
		{/if}
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
			<button onclick={() => (showRoleManager = true)}>Manage Roles</button>
			<button onclick={() => iconInput.click()}>Upload Icon</button>
			<input bind:this={iconInput} type="file" accept="image/*" onchange={uploadIcon} style="display:none" />
		{/if}

		{#if isAdmin || server.member_invites_enabled}
			{#if !generatedInvite}
				<button onclick={generateInvite}>Generate Invite Link</button>
			{:else}
				<div class="invite-row">
					<span class="invite-code">{generatedInvite.code}</span>
					<button class="copy-btn" onclick={copyInvite}>{copied ? '✓ Copied' : 'Copy'}</button>
				</div>
				<div class="invite-meta">
					{#if generatedInvite.expires_at}
						Expires {formatExpiry(generatedInvite.expires_at)}
					{:else}
						Permanent
					{/if}
				</div>
			{/if}
		{/if}

		{#if server.role !== 'owner' && !$currentUser?.is_instance_admin}
			<button class="danger" onclick={leaveServer}>Leave Space</button>
		{/if}

		{#if canDeleteSpace}
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
		border: 1px solid var(--border);
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
		color: var(--text-muted);
		letter-spacing: 0.05em;
		border-bottom: 1px solid var(--border);
		margin-bottom: 0.25rem;
	}
	.menu button {
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
	.menu button:hover { background: rgba(255,255,255,0.08); }
	.menu button.danger { color: #e04545; }
	.menu button.danger:hover { background: rgba(224,69,69,0.1); }
	.separator {
		height: 1px;
		background: var(--border);
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
		color: var(--accent);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.copy-btn {
		background: var(--accent) !important;
		color: white !important;
		padding: 0.2rem 0.5rem !important;
		font-size: 0.75rem !important;
		border-radius: 3px !important;
	}
	.invite-meta {
		padding: 0 0.625rem 0.375rem;
		font-size: 0.7rem;
		color: var(--text-muted);
	}
	.section-divider {
		padding: 0.625rem 0.625rem 0.25rem;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.05em;
		border-top: 1px solid var(--border);
		margin-top: 0.25rem;
	}
	label select {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.4rem 0.5rem;
		border-radius: 4px;
		font-size: 0.875rem;
		font-family: inherit;
		outline: none;
		width: 100%;
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
		color: var(--text-muted);
		letter-spacing: 0.05em;
	}
	label input:not([type="checkbox"]), label textarea {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
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
	.delete-warning strong { color: var(--text); }
	.edit-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		padding: 0.5rem 0.625rem 0.25rem;
	}
	.cancel {
		background: none !important;
		border: none;
		color: var(--text-muted) !important;
		cursor: pointer;
		padding: 0.4rem 0.75rem !important;
		border-radius: 4px;
	}
	.save {
		background: var(--accent) !important;
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
