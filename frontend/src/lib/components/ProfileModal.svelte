<script lang="ts">
	import { api } from '$lib/api';
	import { currentUser, notifPrefs } from '$lib/stores';
	import { playMessageSound, playMentionSound, playDMSound } from '$lib/sounds';
	import { defaultAvatarUrl } from '$lib/avatar';

	let { onclose }: { onclose: () => void } = $props();

	// Keep form state derived from store so it stays fresh after avatar upload
	let displayName = $state($currentUser?.display_name ?? '');
	let bio = $state($currentUser?.bio ?? '');
	let saving = $state(false);
	let uploading = $state(false);
	let fileInput: HTMLInputElement;

	// Sync form fields if store changes (e.g. after avatar upload re-fetch)
	$effect(() => {
		displayName = $currentUser?.display_name ?? '';
		bio = $currentUser?.bio ?? '';
	});

	async function save() {
		if (saving) return;
		saving = true;
		try {
			const updated = await api.updateMe({ display_name: displayName, bio });
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
		width: 400px;
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
</style>
