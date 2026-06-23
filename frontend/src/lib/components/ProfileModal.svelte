<script lang="ts">
	import { api } from '$lib/api';
	import { currentUser, notifPrefs, instanceConfig } from '$lib/stores';
	import { playMessageSound, playMentionSound, playDMSound } from '$lib/sounds';
	import { defaultAvatarUrl } from '$lib/avatar';

	let { onclose }: { onclose: () => void } = $props();

	// Keep form state derived from store so it stays fresh after avatar upload
	let displayName = $state($currentUser?.display_name ?? '');
	let bio = $state($currentUser?.bio ?? '');
	let customStatus = $state($currentUser?.custom_status ?? '');
	let saving = $state(false);
	let uploading = $state(false);
	let fileInput: HTMLInputElement;

	// Desktop-only game detection
	const isDesktop = typeof window !== 'undefined' && !!(window as any).__TAURI_DESKTOP__;
	const tauri = isDesktop ? (window as any).__TAURI__?.core : null;

	interface GameEntry { name: string; processes: string[] }
	let games = $state<GameEntry[]>([]);
	let gamesLoaded = $state(false);
	let gamesSaving = $state(false);
	let newGameName = $state('');
	let newGameProcs = $state('');

	if (isDesktop) {
		tauri?.invoke('get_games')
			.then((g: GameEntry[]) => { games = g; })
			.catch(() => { games = []; })
			.finally(() => { gamesLoaded = true; });
	}

	async function saveGames() {
		gamesSaving = true;
		try { await tauri.invoke('save_games', { games }); }
		finally { gamesSaving = false; }
	}

	function addGame() {
		const name = newGameName.trim();
		const processes = newGameProcs.split(',').map((p: string) => p.trim().toLowerCase()).filter(Boolean);
		if (!name || !processes.length) return;
		games = [...games, { name, processes }];
		newGameName = '';
		newGameProcs = '';
		saveGames();
	}

	function removeGame(i: number) {
		games = games.filter((_, idx) => idx !== i);
		saveGames();
	}

	function updateGameProcesses(i: number, val: string) {
		games = games.map((g, idx) => idx === i
			? { ...g, processes: val.split(',').map((p: string) => p.trim().toLowerCase()).filter(Boolean) }
			: g
		);
	}

	// Sync form fields if store changes (e.g. after avatar upload re-fetch)
	$effect(() => {
		displayName = $currentUser?.display_name ?? '';
		bio = $currentUser?.bio ?? '';
		customStatus = $currentUser?.custom_status ?? '';
	});

	async function save() {
		if (saving) return;
		saving = true;
		try {
			const updated = await api.updateMe({ display_name: displayName, bio, custom_status: customStatus });
			currentUser.set(updated);
			onclose();
		} finally {
			saving = false;
		}
	}

	async function uploadAvatar(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		uploading = true;
		try {
			await api.uploadAvatar(file);
			// Re-fetch to get the server's canonical URL and keep everything in sync
			const updated = await api.me();
			currentUser.set(updated);
		} finally {
			uploading = false;
		}
	}

	function avatarSrc(user: typeof $currentUser) {
		if (!user) return '';
		return user.avatar_url || defaultAvatarUrl(user.id, user.display_name);
	}
</script>

<div class="overlay" onclick={onclose}>
	<div class="modal" onclick={(e) => e.stopPropagation()}>
		<h2>Edit Profile</h2>

		<div class="avatar-section">
			<div class="avatar-wrapper">
				<img src={avatarSrc($currentUser)} alt="avatar" class="avatar" />
				{#if uploading}
					<div class="avatar-uploading">…</div>
				{/if}
			</div>
			<div class="avatar-actions">
				<button onclick={() => fileInput.click()} disabled={uploading}>
					{uploading ? 'Uploading…' : 'Change avatar'}
				</button>
				{#if $currentUser?.avatar_url}
					<button class="remove" onclick={async () => {
						// Clear avatar — send empty string, backend keeps existing if empty so we need a dedicated approach
						// For now just re-upload a blank; TODO: add clear endpoint
					}}>Remove</button>
				{/if}
			</div>
			<input bind:this={fileInput} type="file" accept="image/*" onchange={uploadAvatar} style="display:none" />
		</div>

		<label>
			Display name
			<input bind:value={displayName} />
		</label>
		<label>
			Custom status
			<input bind:value={customStatus} placeholder="What are you up to?" maxlength="128" />
		</label>
		<label>
			Bio
			<textarea bind:value={bio} rows="3"></textarea>
		</label>

		<div class="sounds-section">
			<div class="sounds-title">Notification Sounds</div>
			<div class="sound-row">
				<div class="sound-label">
					<span>Message sounds</span>
					<span class="sound-hint">When a message arrives in the active channel</span>
				</div>
				<button
					class="toggle"
					class:on={$notifPrefs.messageSound}
					onclick={() => {
						notifPrefs.update(p => ({ ...p, messageSound: !p.messageSound }));
						if (!$notifPrefs.messageSound) playMessageSound();
					}}
					aria-label="Toggle message sounds"
				>
					<span class="knob"></span>
				</button>
			</div>
			<div class="sound-row">
				<div class="sound-label">
					<span>Mention sounds</span>
					<span class="sound-hint">When someone @mentions you</span>
				</div>
				<button
					class="toggle"
					class:on={$notifPrefs.mentionSound}
					onclick={() => {
						notifPrefs.update(p => ({ ...p, mentionSound: !p.mentionSound }));
						if (!$notifPrefs.mentionSound) playMentionSound();
					}}
					aria-label="Toggle mention sounds"
				>
					<span class="knob"></span>
				</button>
			</div>
			<div class="sound-row">
				<div class="sound-label">
					<span>DM sounds</span>
					<span class="sound-hint">When you receive a direct message</span>
				</div>
				<button
					class="toggle"
					class:on={$notifPrefs.dmSound}
					onclick={() => {
						notifPrefs.update(p => ({ ...p, dmSound: !p.dmSound }));
						if (!$notifPrefs.dmSound) playDMSound();
					}}
					aria-label="Toggle DM sounds"
				>
					<span class="knob"></span>
				</button>
			</div>
		</div>

		{#if isDesktop && gamesLoaded}
			<div class="section">
				<div class="section-title">Game Detection</div>
				<div class="game-add-row">
					<input class="game-inp" bind:value={newGameName} placeholder="Game name" />
					<input class="game-inp wide" bind:value={newGameProcs} placeholder="process.exe, other.exe" />
					<button class="game-add-btn" onclick={addGame} disabled={!newGameName.trim() || !newGameProcs.trim()}>Add</button>
				</div>
				<div class="game-list">
					{#each games as game, i}
						<div class="game-row">
							<span class="game-name">{game.name}</span>
							<input
								class="proc-input"
								value={game.processes.join(', ')}
								onchange={(e) => { updateGameProcesses(i, (e.target as HTMLInputElement).value); saveGames(); }}
							/>
							<button class="game-remove" onclick={() => removeGame(i)} title="Remove">✕</button>
						</div>
					{/each}
				</div>
				{#if gamesSaving}<p class="game-hint">Saving…</p>{/if}
			</div>
		{/if}

		{#if $instanceConfig.desktop_download_url}
			<div class="desktop-section">
				<a class="desktop-btn" href={$instanceConfig.desktop_download_url} target="_blank" rel="noopener noreferrer">
					Download Desktop App ↓
				</a>
			</div>
		{/if}

		<div class="actions">
			<button onclick={onclose} class="cancel">Cancel</button>
			<button onclick={save} disabled={saving || uploading} class="save">
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
	}
	.modal {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1.5rem;
		width: 480px;
		max-width: 95vw;
		max-height: 90vh;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	h2 { color: var(--text); font-size: 1.1rem; }
	.avatar-section {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.avatar-wrapper {
		position: relative;
		width: 64px;
		height: 64px;
		flex-shrink: 0;
	}
	.avatar {
		width: 64px;
		height: 64px;
		border-radius: 50%;
		object-fit: cover;
	}
	.avatar-uploading {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		background: rgba(0,0,0,0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		color: white;
		font-size: 0.75rem;
	}
	.avatar-actions {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.avatar-actions button {
		background: none;
		border: 1px solid var(--border);
		color: var(--accent);
		padding: 0.4rem 0.75rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.85rem;
	}
	.avatar-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
	.avatar-actions button.remove { color: #e04545; border-color: rgba(224,69,69,0.3); }
	label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		color: var(--text-muted);
		font-size: 0.8rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	label input, label textarea {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem;
		border-radius: 4px;
		font-size: 0.9rem;
		font-family: inherit;
		resize: vertical;
		outline: none;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.cancel {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.5rem 1rem;
	}
	.save {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.5rem 1.25rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.9rem;
	}
	.save:disabled { opacity: 0.6; cursor: not-allowed; }
	.sounds-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		border-top: 1px solid var(--border);
		padding-top: 0.75rem;
	}
	.sounds-title {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-muted);
		margin-bottom: 0.25rem;
	}
	.sound-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.35rem 0;
	}
	.sound-label {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.sound-label span:first-child {
		font-size: 0.88rem;
		color: var(--text);
	}
	.sound-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
	}
	.toggle {
		position: relative;
		width: 38px;
		height: 22px;
		border-radius: 11px;
		background: var(--border);
		border: none;
		cursor: pointer;
		flex-shrink: 0;
		transition: background 0.2s;
		padding: 0;
	}
	.toggle.on { background: var(--accent); }
	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: white;
		transition: transform 0.2s;
		display: block;
	}
	.toggle.on .knob { transform: translateX(16px); }
	.section {
		border-top: 1px solid var(--border);
		padding-top: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.section-title {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-muted);
	}
	.game-add-row { display: flex; gap: 0.4rem; flex-wrap: wrap; }
	.game-inp { background: var(--bg-input); border: 1px solid var(--border); border-radius: 4px; color: var(--text); font-size: 0.8rem; padding: 0.35rem 0.5rem; font-family: inherit; outline: none; min-width: 100px; }
	.game-inp:focus { border-color: var(--accent); }
	.game-inp.wide { flex: 1; }
	.game-add-btn { background: var(--accent); border: none; border-radius: 4px; color: white; font-size: 0.8rem; font-weight: 600; padding: 0.35rem 0.7rem; cursor: pointer; font-family: inherit; white-space: nowrap; }
	.game-add-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.game-list { display: flex; flex-direction: column; gap: 0.25rem; max-height: 180px; overflow-y: auto; }
	.game-row { display: flex; align-items: center; gap: 0.4rem; }
	.game-name { font-size: 0.8rem; font-weight: 600; min-width: 120px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.proc-input { flex: 1; background: var(--bg-input); border: 1px solid var(--border); border-radius: 4px; color: var(--text-muted); font-size: 0.72rem; padding: 0.25rem 0.4rem; font-family: monospace; outline: none; min-width: 0; }
	.proc-input:focus { border-color: var(--accent); color: var(--text); }
	.game-remove { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.75rem; padding: 0.15rem 0.3rem; border-radius: 3px; flex-shrink: 0; }
	.game-remove:hover { color: #e04545; background: rgba(224,69,69,0.1); }
	.game-hint { font-size: 0.72rem; color: var(--text-muted); margin: 0; }
	.desktop-section {
		border-top: 1px solid var(--border);
		padding-top: 0.75rem;
	}
	.desktop-btn {
		display: block;
		width: 100%;
		padding: 0.6rem;
		border-radius: 6px;
		background: var(--accent);
		color: white;
		text-align: center;
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 600;
	}
	.desktop-btn:hover { opacity: 0.85; }
</style>
