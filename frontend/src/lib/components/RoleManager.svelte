<script lang="ts">
	import { api, type SpaceRole } from '$lib/api';

	let { serverId, onclose }: { serverId: string; onclose: () => void } = $props();

	let roles = $state<SpaceRole[]>([]);
	let loading = $state(true);
	let editingId = $state<string | null>(null);
	let editName = $state('');
	let editColor = $state('');
	let newName = $state('');
	let newColor = $state('#5865f2');
	let saving = $state(false);
	let error = $state('');

	$effect(() => {
		if (serverId) load();
	});

	async function load() {
		loading = true;
		try {
			roles = await api.listRoles(serverId);
		} finally {
			loading = false;
		}
	}

	async function createRole() {
		if (!newName.trim() || saving) return;
		saving = true;
		error = '';
		try {
			const r = await api.createRole(serverId, { name: newName.trim(), color: newColor });
			roles = [...roles, r];
			newName = '';
			newColor = '#5865f2';
		} catch (e: any) {
			error = e.message ?? 'Failed to create role';
		} finally {
			saving = false;
		}
	}

	function startEdit(r: SpaceRole) {
		editingId = r.id;
		editName = r.name;
		editColor = r.color || '#5865f2';
	}

	async function saveEdit(r: SpaceRole) {
		if (saving) return;
		saving = true;
		error = '';
		try {
			const updated = await api.updateRole(serverId, r.id, { name: editName.trim() || r.name, color: editColor });
			roles = roles.map((x) => x.id === updated.id ? updated : x);
			editingId = null;
		} catch (e: any) {
			error = e.message ?? 'Failed to update role';
		} finally {
			saving = false;
		}
	}

	async function deleteRole(r: SpaceRole) {
		if (!confirm(`Delete role "${r.name}"? Members with this role will lose it.`)) return;
		try {
			await api.deleteRole(serverId, r.id);
			roles = roles.filter((x) => x.id !== r.id);
		} catch (e: any) {
			error = e.message ?? 'Failed to delete role';
		}
	}
</script>

<div class="overlay" onclick={onclose} role="presentation">
	<div class="panel" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Manage roles">
		<div class="header">
			<h2>Manage Roles</h2>
			<button class="close" onclick={onclose}>✕</button>
		</div>

		{#if error}
			<div class="error-bar">{error}</div>
		{/if}

		<div class="body">
			{#if loading}
				<p class="hint">Loading…</p>
			{:else}
				<div class="role-list">
					{#each roles as r}
						<div class="role-row">
							{#if editingId === r.id}
								<input
									class="edit-name"
									bind:value={editName}
									onkeydown={(e) => e.key === 'Enter' && saveEdit(r)}
								/>
								<input type="color" class="color-pick" bind:value={editColor} />
								<button class="btn-save" onclick={() => saveEdit(r)} disabled={saving}>Save</button>
								<button class="btn-cancel" onclick={() => (editingId = null)}>Cancel</button>
							{:else}
								<span
									class="role-pill"
									style={r.color ? `color:${r.color}; border-color:${r.color}40` : ''}
								>{r.name}{r.is_everyone ? ' (everyone)' : ''}</span>
								{#if !r.is_everyone}
									<button class="btn-edit" onclick={() => startEdit(r)}>Edit</button>
									<button class="btn-del" onclick={() => deleteRole(r)}>Delete</button>
								{/if}
							{/if}
						</div>
					{/each}
				</div>

				<div class="create-row">
					<input
						class="new-name"
						type="text"
						placeholder="New role name…"
						bind:value={newName}
						onkeydown={(e) => e.key === 'Enter' && createRole()}
					/>
					<input type="color" class="color-pick" bind:value={newColor} title="Role colour" />
					<button class="btn-create" onclick={createRole} disabled={!newName.trim() || saving}>
						Create
					</button>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.75);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}
	.panel {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: 480px;
		max-width: calc(100vw - 2rem);
		max-height: 80vh;
		display: flex;
		flex-direction: column;
	}
	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	h2 { color: var(--text); font-size: 1.05rem; margin: 0; }
	.close { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.9rem; }
	.close:hover { color: var(--text); }
	.error-bar {
		background: rgba(224,69,69,0.15);
		color: #e04545;
		font-size: 0.85rem;
		padding: 0.5rem 1.5rem;
		flex-shrink: 0;
	}
	.body {
		flex: 1;
		overflow-y: auto;
		padding: 1rem 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.hint { color: var(--text-muted); font-size: 0.9rem; text-align: center; padding: 1rem 0; }
	.role-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.role-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 6px;
		min-height: 2.5rem;
	}
	.role-pill {
		flex: 1;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--text);
		border: 1px solid transparent;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
	}
	.edit-name {
		flex: 1;
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		font-size: 0.875rem;
		outline: none;
	}
	.color-pick {
		width: 32px;
		height: 28px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: none;
		cursor: pointer;
		padding: 1px;
	}
	.btn-edit, .btn-del, .btn-save, .btn-cancel, .btn-create {
		font-size: 0.8rem;
		padding: 0.25rem 0.6rem;
		border-radius: 4px;
		cursor: pointer;
		border: none;
		white-space: nowrap;
	}
	.btn-edit { background: var(--bg-sidebar); color: var(--text); }
	.btn-edit:hover { background: var(--border); }
	.btn-del { background: rgba(224,69,69,0.15); color: #e04545; }
	.btn-del:hover { background: rgba(224,69,69,0.3); }
	.btn-save { background: var(--accent); color: white; }
	.btn-save:disabled { opacity: 0.5; cursor: not-allowed; }
	.btn-cancel { background: none; color: var(--text-muted); }
	.btn-cancel:hover { color: var(--text); }
	.create-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--border);
	}
	.new-name {
		flex: 1;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.4rem 0.6rem;
		border-radius: 6px;
		font-size: 0.875rem;
		outline: none;
	}
	.new-name:focus { border-color: var(--accent); }
	.btn-create {
		background: var(--accent);
		color: white;
		font-weight: 600;
		padding: 0.4rem 0.9rem;
		border-radius: 6px;
		font-size: 0.875rem;
	}
	.btn-create:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
