<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Thread } from '$lib/api';
	import { activeServer, activeChannel } from '$lib/stores';
	import { socket } from '$lib/socket';
	import EmojiPicker from './EmojiPicker.svelte';

	interface Props {
		onopen: (thread: Thread) => void;
	}
	let { onopen }: Props = $props();

	let threads = $state<Thread[]>([]);
	let showNew = $state(false);
	let newTitle = $state('');
	let newBody = $state('');
	let showBodyEmoji = $state(false);
	let uploading = $state(false);
	let bodyTextarea: HTMLTextAreaElement;
	let fileInput: HTMLInputElement;
	let creating = $state(false);
	let error = $state('');

	const isAdmin = $derived($activeServer?.role === 'owner' || $activeServer?.role === 'admin');

	onMount(() => {
		loadThreads();
	});

	async function loadThreads() {
		if (!$activeServer || !$activeChannel) return;
		threads = await api.listThreads($activeServer.id, $activeChannel.id).catch(() => []);
	}

	// Subscribe to channel room for thread.new and thread.updated events
	$effect(() => {
		const chId = $activeChannel?.id;
		if (!chId) return;
		socket.subscribe('channel:' + chId);
		const unsub = socket.on((event) => {
			if (event.type === 'thread.new') {
				if (event.payload.channel_id !== chId) return;
				threads = [event.payload, ...threads];
			} else if (event.type === 'thread.updated') {
				if (event.payload.channel_id !== chId) return;
				threads = threads.map((t) => t.id === event.payload.id ? event.payload : t);
			}
		});
		return () => { unsub(); socket.unsubscribe('channel:' + chId); };
	});

	async function createThread() {
		if (!newTitle.trim() || !$activeServer || !$activeChannel) return;
		creating = true;
		error = '';
		try {
			const t = await api.createThread($activeServer.id, $activeChannel.id, newTitle.trim(), newBody.trim() || undefined);
			newTitle = '';
			newBody = '';
			showNew = false;
			showBodyEmoji = false;
			onopen(t);
		} catch (e: any) {
			error = e.message ?? 'Failed to create thread';
		} finally {
			creating = false;
		}
	}

	async function uploadBodyImage(file: File) {
		if (!file.type.startsWith('image/')) return;
		uploading = true;
		try {
			const { url } = await api.uploadFile(file);
			insertAtCursor(url);
		} finally {
			uploading = false;
		}
	}

	function insertAtCursor(text: string) {
		const el = bodyTextarea;
		if (!el) { newBody += text; return; }
		const start = el.selectionStart ?? newBody.length;
		const end = el.selectionEnd ?? newBody.length;
		newBody = newBody.slice(0, start) + text + newBody.slice(end);
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = start + text.length;
		}, 0);
	}

	function onBodyPaste(e: ClipboardEvent) {
		const image = Array.from(e.clipboardData?.items ?? []).find((i) => i.type.startsWith('image/'));
		if (!image) return;
		e.preventDefault();
		const file = image.getAsFile();
		if (file) uploadBodyImage(file);
	}

	function timeAgo(iso: string): string {
		const diff = Date.now() - new Date(iso).getTime();
		const m = Math.floor(diff / 60000);
		if (m < 1) return 'just now';
		if (m < 60) return `${m}m ago`;
		const h = Math.floor(m / 60);
		if (h < 24) return `${h}h ago`;
		const d = Math.floor(h / 24);
		return `${d}d ago`;
	}
</script>

<div class="thread-channel">
	<div class="tc-header">
		<div class="tc-meta">
			<h2 class="tc-title">💬 {$activeChannel?.name}</h2>
			<p class="tc-hint">Click a thread to join the conversation.</p>
		</div>
		<button class="new-btn" onclick={() => { showNew = !showNew; newTitle = ''; error = ''; }}>
			+ New Thread
		</button>
	</div>

	{#if showNew}
		<div class="new-thread-form">
			<input
				bind:value={newTitle}
				placeholder="Thread title…"
				maxlength={120}
				autofocus
				onkeydown={(e) => { if (e.key === 'Escape') { showNew = false; } }}
			/>
			<div class="body-area">
				<textarea
					bind:this={bodyTextarea}
					bind:value={newBody}
					placeholder="Add an opening message (optional)…"
					rows="3"
					disabled={uploading}
					onpaste={onBodyPaste}
					onkeydown={(e) => { if (e.key === 'Escape') showNew = false; }}
				></textarea>
				<div class="body-actions">
					<button
						class="action-icon"
						title="Upload image"
						disabled={uploading}
						onclick={() => fileInput.click()}
					>
						{#if uploading}
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
						{:else}
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>
						{/if}
					</button>
					<button
						class="action-icon"
						class:active={showBodyEmoji}
						title="Emoji"
						onclick={() => (showBodyEmoji = !showBodyEmoji)}
					>
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M8 13s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
					</button>
					{#if showBodyEmoji}
						<EmojiPicker
							onSelect={(emoji) => { insertAtCursor(emoji); showBodyEmoji = false; }}
							onClose={() => (showBodyEmoji = false)}
						/>
					{/if}
				</div>
			</div>
			<input bind:this={fileInput} type="file" accept="image/*" style="display:none"
				onchange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) uploadBodyImage(f); (e.target as HTMLInputElement).value = ''; }} />
			{#if error}<span class="form-error">{error}</span>{/if}
			<div class="form-actions">
				<button class="cancel-btn" onclick={() => { showNew = false; newBody = ''; showBodyEmoji = false; }}>Cancel</button>
				<button class="create-btn" onclick={createThread} disabled={creating || uploading || !newTitle.trim()}>
					{creating ? 'Creating…' : 'Create Thread'}
				</button>
			</div>
		</div>
	{/if}

	{#if threads.length === 0}
		<div class="empty">
			<p>No threads yet.</p>
			{#if !showNew}
				<button class="new-btn" onclick={() => (showNew = true)}>Start the first thread</button>
			{/if}
		</div>
	{:else}
		<div class="thread-grid">
			{#each threads as thread}
				<button class="thread-card" onclick={() => onopen(thread)}>
					<div class="card-title">{thread.title}</div>
					<div class="card-meta">
						<img
							src={thread.created_by.avatar_url || '/default-avatar.png'}
							alt=""
							class="card-avatar"
						/>
						<span class="card-author">{thread.created_by.display_name}</span>
						<span class="card-dot">·</span>
						<span class="card-time">{timeAgo(thread.last_message_at)}</span>
					</div>
					<div class="card-count">
						<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
						{thread.message_count}
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.thread-channel {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow-y: auto;
		padding: 1.5rem;
		gap: 1.25rem;
	}
	.tc-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	.tc-title {
		font-size: 1.15rem;
		font-weight: 700;
		color: var(--text);
	}
	.tc-hint {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-top: 2px;
	}
	.new-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.45rem 0.9rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.85rem;
		font-weight: 600;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.new-btn:hover { filter: brightness(1.1); }
	.new-thread-form {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.new-thread-form input {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.6rem 0.75rem;
		border-radius: 6px;
		font-size: 0.95rem;
		outline: none;
		font-family: inherit;
	}
	.new-thread-form input:focus { border-color: var(--accent); }
	.body-area {
		display: flex;
		flex-direction: column;
		gap: 0;
		border: 1px solid var(--border);
		border-radius: 6px;
		overflow: hidden;
	}
	.body-area textarea {
		background: var(--bg-input);
		border: none;
		color: var(--text);
		padding: 0.6rem 0.75rem;
		font-size: 0.9rem;
		outline: none;
		font-family: inherit;
		resize: none;
		line-height: 1.45;
	}
	.body-area textarea:focus { outline: none; }
	.body-area:focus-within { border-color: var(--accent); }
	.body-actions {
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 4px 6px;
		background: var(--bg-panel);
		border-top: 1px solid var(--border);
		position: relative;
	}
	.action-icon {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 4px 6px;
		border-radius: 4px;
		display: flex;
		align-items: center;
	}
	.action-icon:hover, .action-icon.active { color: var(--accent); background: rgba(255,255,255,0.05); }
	.action-icon:disabled { opacity: 0.4; cursor: not-allowed; }
	.form-error { font-size: 0.8rem; color: #e04545; }
	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.cancel-btn {
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.4rem 0.8rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.85rem;
	}
	.cancel-btn:hover { color: var(--text); }
	.create-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.4rem 0.9rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.85rem;
		font-weight: 600;
	}
	.create-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
		padding: 3rem 1rem;
		color: var(--text-muted);
	}
	.thread-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: 0.875rem;
	}
	.thread-card {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1rem 1.1rem;
		text-align: left;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		transition: border-color 0.15s, background 0.15s;
		position: relative;
	}
	.thread-card:hover {
		border-color: var(--accent);
		background: rgba(255, 255, 255, 0.03);
	}
	.card-title {
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--text);
		line-height: 1.3;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.card-meta {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: 0.78rem;
		color: var(--text-muted);
	}
	.card-avatar {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		object-fit: cover;
	}
	.card-author { font-weight: 500; color: var(--text-muted); }
	.card-dot { opacity: 0.5; }
	.card-time { opacity: 0.7; }
	.card-count {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: auto;
	}
</style>
