<script lang="ts">
	import { api, type Channel, type ChannelPerm } from '$lib/api';

	let { serverId, channel, onclose }: { serverId: string; channel: Channel; onclose: () => void } = $props();

	let perms = $state<ChannelPerm[]>([]);
	let loading = $state(true);
	let saving = $state<Record<string, boolean>>({});

	$effect(() => {
		if (serverId && channel.id) load();
	});

	async function load() {
		loading = true;
		try {
			perms = await api.listChannelPerms(serverId, channel.id);
		} finally {
			loading = false;
		}
	}

	async function setOverride(p: ChannelPerm, canView: boolean, canWrite: boolean) {
		saving = { ...saving, [p.role_id]: true };
		try {
			await api.setChannelPerm(serverId, channel.id, p.role_id, { can_view: canView, can_write: canWrite });
			perms = perms.map((x) => x.role_id === p.role_id
				? { ...x, can_view: canView, can_write: canWrite, has_override: true }
				: x
			);
		} finally {
			saving = { ...saving, [p.role_id]: false };
		}
	}

	async function clearOverride(p: ChannelPerm) {
		saving = { ...saving, [p.role_id]: true };
		try {
			await api.deleteChannelPerm(serverId, channel.id, p.role_id);
			perms = perms.map((x) => x.role_id === p.role_id
				? { ...x, can_view: true, can_write: true, has_override: false }
				: x
			);
		} finally {
			saving = { ...saving, [p.role_id]: false };
		}
	}

	function toggleView(p: ChannelPerm) {
		const newView = !p.can_view;
		const newWrite = newView ? p.can_write : false;
		setOverride(p, newView, newWrite);
	}

	function toggleWrite(p: ChannelPerm) {
		setOverride(p, p.can_view, !p.can_write);
	}
</script>

<div class="overlay" onclick={onclose} role="presentation">
	<div class="panel" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Channel permissions">
		<div class="header">
			<div>
				<h2># {channel.name} — Permissions</h2>
				<p class="subtitle">Roles without an override can view and write by default.</p>
			</div>
			<button class="close" onclick={onclose}>✕</button>
		</div>

		<div class="body">
			{#if loading}
				<p class="hint">Loading…</p>
			{:else}
				<div class="perm-table">
					<div class="table-head">
						<span class="col-role">Role</span>
						<span class="col-toggle">Can View</span>
						<span class="col-toggle">Can Write</span>
						<span class="col-reset"></span>
					</div>
					{#each perms as p}
						<div class="table-row" class:overridden={p.has_override}>
							<span class="col-role">
								<span class="role-dot" style={p.color ? `background:${p.color}` : ''}></span>
								<span class="role-name" style={p.color ? `color:${p.color}` : ''}>{p.role_name}</span>
								{#if p.is_everyone}<span class="everyone-tag">default</span>{/if}
							</span>
							<span class="col-toggle">
								<button
									class="toggle"
									class:on={p.can_view}
									onclick={() => toggleView(p)}
									disabled={saving[p.role_id]}
									aria-label={p.can_view ? 'Deny view' : 'Allow view'}
								>
									{p.can_view ? '✓' : '✕'}
								</button>
							</span>
							<span class="col-toggle">
								<button
									class="toggle"
									class:on={p.can_write}
									disabled={!p.can_view || saving[p.role_id]}
									onclick={() => toggleWrite(p)}
									aria-label={p.can_write ? 'Deny write' : 'Allow write'}
									title={!p.can_view ? 'Must have view access to write' : ''}
								>
									{p.can_write ? '✓' : '✕'}
								</button>
							</span>
							<span class="col-reset">
								{#if p.has_override}
									<button class="reset-btn" onclick={() => clearOverride(p)} disabled={saving[p.role_id]} title="Reset to default">Reset</button>
								{/if}
							</span>
						</div>
					{/each}
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
		width: 540px;
		max-width: calc(100vw - 2rem);
		max-height: 80vh;
		display: flex;
		flex-direction: column;
	}
	.header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
		gap: 1rem;
	}
	h2 { color: var(--text); font-size: 1rem; margin: 0 0 0.2rem; }
	.subtitle { color: var(--text-muted); font-size: 0.78rem; margin: 0; }
	.close { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.9rem; flex-shrink: 0; }
	.close:hover { color: var(--text); }
	.body { flex: 1; overflow-y: auto; padding: 0.75rem 1.5rem 1.25rem; }
	.hint { color: var(--text-muted); font-size: 0.9rem; text-align: center; padding: 1rem 0; }
	.perm-table { display: flex; flex-direction: column; gap: 0.25rem; }
	.table-head {
		display: grid;
		grid-template-columns: 1fr 80px 80px 64px;
		padding: 0.3rem 0.5rem;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.05em;
	}
	.col-role { display: flex; align-items: center; gap: 0.4rem; }
	.col-toggle { text-align: center; }
	.col-reset { text-align: right; }
	.table-row {
		display: grid;
		grid-template-columns: 1fr 80px 80px 64px;
		align-items: center;
		padding: 0.45rem 0.5rem;
		border-radius: 6px;
		border: 1px solid transparent;
	}
	.table-row:hover { background: var(--bg-input); }
	.table-row.overridden { border-color: var(--border); }
	.role-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: var(--text-muted);
		flex-shrink: 0;
	}
	.role-name { font-size: 0.875rem; font-weight: 600; color: var(--text); }
	.everyone-tag {
		font-size: 0.65rem;
		color: var(--text-muted);
		background: var(--bg-sidebar);
		padding: 0.1rem 0.3rem;
		border-radius: 3px;
		margin-left: 0.25rem;
	}
	.toggle {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		border: none;
		cursor: pointer;
		font-size: 0.8rem;
		font-weight: 700;
		background: rgba(224,69,69,0.2);
		color: #e04545;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		margin: 0 auto;
	}
	.toggle.on { background: rgba(68,201,125,0.2); color: #44c97d; }
	.toggle:disabled { opacity: 0.4; cursor: not-allowed; }
	.toggle:not(:disabled):hover { filter: brightness(1.2); }
	.reset-btn {
		font-size: 0.75rem;
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.2rem 0.4rem;
		border-radius: 4px;
		cursor: pointer;
	}
	.reset-btn:hover { color: var(--text); }
	.reset-btn:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
