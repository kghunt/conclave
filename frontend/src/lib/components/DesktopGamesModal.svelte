<script lang="ts">
	let { onclose }: { onclose: () => void } = $props();

	interface GameEntry { name: string; processes: string[] }

	let games = $state<GameEntry[]>([]);
	let loading = $state(true);
	let saving = $state(false);

	// New game form
	let newName = $state('');
	let newProcesses = $state('');

	const tauri = (window as any).__TAURI__?.core;

	async function load() {
		games = await tauri.invoke('get_games');
		loading = false;
	}

	async function save() {
		saving = true;
		try { await tauri.invoke('save_games', { games }); }
		finally { saving = false; }
	}

	function addGame() {
		const name = newName.trim();
		const processes = newProcesses.split(',').map((p: string) => p.trim().toLowerCase()).filter(Boolean);
		if (!name || !processes.length) return;
		games = [...games, { name, processes }];
		newName = '';
		newProcesses = '';
		save();
	}

	function removeGame(i: number) {
		games = games.filter((_, idx) => idx !== i);
		save();
	}

	function updateProcesses(i: number, val: string) {
		games = games.map((g, idx) => idx === i
			? { ...g, processes: val.split(',').map((p: string) => p.trim().toLowerCase()).filter(Boolean) }
			: g
		);
	}

	load();
</script>

<div class="overlay" onclick={onclose} role="presentation">
	<div class="panel" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="Game detection settings">
		<div class="header">
			<h2>Game Detection</h2>
			<p class="sub">Games are checked every 30 seconds against running processes.</p>
			<button class="close" onclick={onclose}>✕</button>
		</div>

		<div class="body">
			{#if loading}
				<p class="hint">Loading…</p>
			{:else}
				<div class="add-form">
					<input bind:value={newName} placeholder="Game name" class="inp" />
					<input bind:value={newProcesses} placeholder="Process names, comma-separated (e.g. game.exe)" class="inp wide" />
					<button class="add-btn" onclick={addGame} disabled={!newName.trim() || !newProcesses.trim()}>Add</button>
				</div>

				<div class="game-list">
					{#each games as game, i}
						<div class="game-row">
							<span class="game-name">{game.name}</span>
							<input
								class="proc-input"
								value={game.processes.join(', ')}
								onchange={(e) => { updateProcesses(i, (e.target as HTMLInputElement).value); save(); }}
							/>
							<button class="remove-btn" onclick={() => removeGame(i)} title="Remove">✕</button>
						</div>
					{/each}
				</div>

				<p class="hint">{saving ? 'Saving…' : `${games.length} games`}</p>
			{/if}
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.6);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
	}
	.panel {
		background: var(--bg-sidebar);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: 600px;
		max-width: 95vw;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
	}
	.header {
		padding: 1.25rem 1.5rem 0.75rem;
		border-bottom: 1px solid var(--border);
		position: relative;
	}
	.header h2 { font-size: 1rem; font-weight: 700; margin: 0 0 0.2rem; }
	.sub { font-size: 0.78rem; color: var(--text-muted); margin: 0; }
	.close { position: absolute; top: 1rem; right: 1rem; background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.9rem; }
	.close:hover { color: var(--text); }
	.body { flex: 1; overflow-y: auto; padding: 1rem 1.5rem; display: flex; flex-direction: column; gap: 0.75rem; }
	.add-form { display: flex; gap: 0.5rem; flex-wrap: wrap; }
	.inp { background: var(--bg-input); border: 1px solid var(--border); border-radius: 5px; color: var(--text); font-size: 0.85rem; padding: 0.4rem 0.6rem; font-family: inherit; outline: none; min-width: 140px; }
	.inp:focus { border-color: var(--accent); }
	.wide { flex: 1; }
	.add-btn { background: var(--accent); border: none; border-radius: 5px; color: white; font-size: 0.85rem; font-weight: 600; padding: 0.4rem 0.9rem; cursor: pointer; font-family: inherit; white-space: nowrap; }
	.add-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.game-list { display: flex; flex-direction: column; gap: 0.3rem; }
	.game-row { display: flex; align-items: center; gap: 0.5rem; }
	.game-name { font-size: 0.85rem; font-weight: 600; min-width: 160px; color: var(--text); }
	.proc-input { flex: 1; background: var(--bg-input); border: 1px solid var(--border); border-radius: 4px; color: var(--text-muted); font-size: 0.78rem; padding: 0.3rem 0.5rem; font-family: monospace; outline: none; }
	.proc-input:focus { border-color: var(--accent); color: var(--text); }
	.remove-btn { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.8rem; padding: 0.2rem 0.4rem; border-radius: 3px; flex-shrink: 0; }
	.remove-btn:hover { color: #e04545; background: rgba(224,69,69,0.1); }
	.hint { font-size: 0.78rem; color: var(--text-muted); margin: 0; }
</style>
