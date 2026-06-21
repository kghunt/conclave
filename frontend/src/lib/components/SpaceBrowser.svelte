<script lang="ts">
	import { api, type ServerDiscovery } from '$lib/api';
	import { servers, activeServer, channels, activeChannel } from '$lib/stores';
	import { socket } from '$lib/socket';

	let { onclose }: { onclose: () => void } = $props();

	let query = $state('');
	let results = $state<ServerDiscovery[]>([]);
	let loading = $state(false);
	let busy = $state<Record<string, boolean>>({});
	let requestSent = $state<string | null>(null); // space id that just got a request sent
	let debounceTimer: ReturnType<typeof setTimeout>;

	// Rules acceptance modal
	let rulesSpace = $state<ServerDiscovery | null>(null);
	let rulesAccepted = $state(false);

	async function search(q: string) {
		loading = true;
		try {
			results = await api.discoverServers(q);
		} catch {
			results = [];
		} finally {
			loading = false;
		}
	}

	function onInput() {
		clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => search(query), 300);
	}

	function tryJoin(space: ServerDiscovery) {
		if (busy[space.id] || space.is_member) return;
		if (space.rules) { rulesSpace = space; rulesAccepted = false; return; }
		doJoin(space);
	}

	function tryRequest(space: ServerDiscovery) {
		if (busy[space.id] || space.has_pending_request) return;
		if (space.rules) { rulesSpace = space; rulesAccepted = false; return; }
		doRequest(space);
	}

	function acceptRules() {
		if (!rulesSpace || !rulesAccepted) return;
		const space = rulesSpace;
		rulesSpace = null;
		if (space.requires_request) doRequest(space);
		else doJoin(space);
	}

	async function doJoin(space: ServerDiscovery) {
		busy = { ...busy, [space.id]: true };
		try {
			await api.joinServer(space.id);
			results = results.map((s) => s.id === space.id ? { ...s, is_member: true } : s);
			const updated = await api.listServers();
			servers.set(updated);
			const joined = updated.find((s) => s.id === space.id);
			if (joined) {
				activeServer.set(joined);
				activeChannel.set(null);
				channels.set([]);
			}
			onclose();
		} catch {
			// ignore
		} finally {
			busy = { ...busy, [space.id]: false };
		}
	}

	async function doRequest(space: ServerDiscovery) {
		busy = { ...busy, [space.id]: true };
		try {
			await api.requestJoinServer(space.id);
			results = results.map((s) => s.id === space.id ? { ...s, has_pending_request: true } : s);
			requestSent = space.id;
			setTimeout(() => { if (requestSent === space.id) requestSent = null; }, 4000);
		} catch {
			// ignore
		} finally {
			busy = { ...busy, [space.id]: false };
		}
	}

	// When a join request is approved, reload servers and navigate there
	const unsub = socket.on((event) => {
		if (event.type === 'join_request.reviewed' && event.payload.action === 'approve') {
			api.listServers().then((updated) => {
				servers.set(updated);
				const joined = updated.find((s: any) => s.id === event.payload.server_id);
				if (joined) {
					activeServer.set(joined);
					activeChannel.set(null);
					channels.set([]);
					onclose();
				}
			});
		}
	});

	$effect(() => () => unsub());

	// Load all discoverable spaces on open
	search('');
</script>

{#if rulesSpace}
	<div class="overlay" role="presentation">
		<div class="rules-modal" role="dialog" aria-label="Space rules">
			<div class="rules-header">
				<h2>Rules for {rulesSpace.name}</h2>
			</div>
			<div class="rules-body">
				<pre class="rules-text">{rulesSpace.rules}</pre>
			</div>
			<div class="rules-footer">
				<label class="rules-accept">
					<input type="checkbox" bind:checked={rulesAccepted} />
					I have read and agree to these rules
				</label>
				<div class="rules-actions">
					<button class="cancel-btn" onclick={() => (rulesSpace = null)}>Cancel</button>
					<button class="accept-btn" disabled={!rulesAccepted} onclick={acceptRules}>
						{rulesSpace.requires_request ? 'Accept & Request' : 'Accept & Join'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<div class="overlay" onclick={onclose} role="presentation">
	<div class="panel" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Browse public spaces">
		<div class="header">
			<h2>Browse Spaces</h2>
			<button class="close" onclick={onclose}>✕</button>
		</div>

		<div class="search-row">
			<input
				class="search-input"
				type="search"
				placeholder="Search public spaces…"
				bind:value={query}
				oninput={onInput}
				autofocus
			/>
		</div>

		{#if requestSent}
			{@const sentSpace = results.find(s => s.id === requestSent)}
			<div class="request-toast">
				✓ Request sent to <strong>{sentSpace?.name ?? 'space'}</strong> — admins will review it shortly.
			</div>
		{/if}

		<div class="results">
			{#if loading}
				<p class="hint">Loading…</p>
			{:else if results.length === 0}
				<p class="hint">{query ? 'No spaces found.' : 'No public spaces yet.'}</p>
			{:else}
				{#each results as space}
					<div class="space-card">
						<div class="space-icon">
							{#if space.icon_url}
								<img src={space.icon_url} alt={space.name} />
							{:else}
								{space.name.slice(0, 2).toUpperCase()}
							{/if}
						</div>
						<div class="space-info">
							<div class="space-name">
								{space.name}
								{#if space.requires_request}<span class="private-badge">🔒 Private</span>{/if}
							</div>
							{#if space.description}
								<div class="space-desc">{space.description}</div>
							{/if}
							<div class="space-meta">{space.member_count} {space.member_count === 1 ? 'member' : 'members'}</div>
						</div>
						{#if space.is_member}
							<button class="join-btn joined" disabled>Joined</button>
						{:else if space.requires_request}
							<button
								class="join-btn"
								class:requested={space.has_pending_request}
								disabled={space.has_pending_request || busy[space.id]}
								onclick={() => tryRequest(space)}
							>
								{#if busy[space.id]}Requesting…{:else if space.has_pending_request}Requested{:else}Request{/if}
							</button>
						{:else}
							<button
								class="join-btn"
								disabled={busy[space.id]}
								onclick={() => tryJoin(space)}
							>
								{busy[space.id] ? 'Joining…' : 'Join'}
							</button>
						{/if}
					</div>
				{/each}
			{/if}
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.75);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}
	.panel {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: 520px;
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
	h2 { color: var(--text); font-size: 1.1rem; margin: 0; }
	.close {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.25rem;
	}
	.close:hover { color: var(--text); }
	.search-row {
		padding: 1rem 1.5rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.search-input {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.6rem 0.75rem;
		border-radius: 6px;
		font-size: 0.9rem;
		box-sizing: border-box;
	}
	.search-input:focus { outline: none; border-color: var(--accent); }
	.results {
		overflow-y: auto;
		flex: 1;
		padding: 0.5rem;
	}
	.hint {
		color: var(--text-muted);
		font-size: 0.9rem;
		text-align: center;
		padding: 2rem 1rem;
	}
	.space-card {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem;
		border-radius: 8px;
		transition: background 0.1s;
	}
	.space-card:hover { background: var(--bg-input); }
	.space-icon {
		width: 48px;
		height: 48px;
		border-radius: 30%;
		background: var(--bg-sidebar);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.85rem;
		font-weight: 700;
		color: var(--text);
		flex-shrink: 0;
		overflow: hidden;
	}
	.space-icon img { width: 100%; height: 100%; object-fit: cover; }
	.space-info {
		flex: 1;
		min-width: 0;
	}
	.space-name {
		color: var(--text);
		font-weight: 600;
		font-size: 0.95rem;
	}
	.space-desc {
		color: var(--text-muted);
		font-size: 0.8rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		margin-top: 0.1rem;
	}
	.space-meta {
		color: var(--text-muted);
		font-size: 0.75rem;
		margin-top: 0.2rem;
	}
	.join-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.4rem 1rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.85rem;
		font-weight: 600;
		flex-shrink: 0;
		transition: opacity 0.15s;
	}
	.join-btn:hover { opacity: 0.85; }
	.join-btn.joined {
		background: var(--bg-input);
		color: var(--text-muted);
		cursor: default;
	}
	.join-btn.requested {
		background: var(--bg-input);
		color: var(--text-muted);
		cursor: default;
	}
	.join-btn:disabled { cursor: not-allowed; opacity: 0.6; }
	.join-btn.joined:disabled, .join-btn.requested:disabled { opacity: 1; }
	.request-toast {
		margin: 0.5rem 1rem 0;
		padding: 0.6rem 0.9rem;
		background: color-mix(in srgb, #3ba55c 15%, transparent);
		border: 1px solid #3ba55c;
		border-radius: 6px;
		color: #3ba55c;
		font-size: 0.85rem;
	}
	.request-toast strong { font-weight: 600; }
	.private-badge {
		font-size: 0.7rem;
		color: var(--text-muted);
		margin-left: 0.4rem;
		font-weight: 400;
	}
	.rules-modal {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: 520px;
		max-width: calc(100vw - 2rem);
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		z-index: 310;
	}
	.rules-header {
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.rules-header h2 { color: var(--text); font-size: 1.05rem; margin: 0; }
	.rules-body {
		flex: 1;
		overflow-y: auto;
		padding: 1.25rem 1.5rem;
	}
	.rules-text {
		white-space: pre-wrap;
		word-break: break-word;
		font-family: inherit;
		font-size: 0.875rem;
		color: var(--text);
		line-height: 1.6;
		margin: 0;
	}
	.rules-footer {
		border-top: 1px solid var(--border);
		padding: 1rem 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		flex-shrink: 0;
	}
	.rules-accept {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: var(--text);
		cursor: pointer;
	}
	.rules-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.cancel-btn {
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.45rem 0.9rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
	}
	.cancel-btn:hover { color: var(--text); }
	.accept-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.45rem 1rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
	}
	.accept-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.accept-btn:not(:disabled):hover { filter: brightness(1.1); }
</style>
