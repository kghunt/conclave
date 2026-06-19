<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { api, type Thread, type ThreadMessage } from '$lib/api';
	import { currentUser, activeServer, serverMembers } from '$lib/stores';
	import type { ServerMember } from '$lib/api';
	import { socket } from '$lib/socket';
	import EmojiPicker from './EmojiPicker.svelte';
	import LightboxImage from './LightboxImage.svelte';

	const isAdmin = $derived(
		$activeServer?.role === 'owner' || $activeServer?.role === 'admin'
	);

	interface Props {
		thread: Thread;
		onback: () => void;
	}
	let { thread, onback }: Props = $props();

	let messages = $state<ThreadMessage[]>([]);
	let input = $state('');
	let sending = $state(false);
	let uploading = $state(false);
	let showEmoji = $state(false);
	let scrollEl: HTMLElement;
	let textarea: HTMLTextAreaElement;
	let fileInput: HTMLInputElement;

	let editingId = $state<string | null>(null);
	let editContent = $state('');
	let replyingTo = $state<ThreadMessage | null>(null);
	let lightboxSrc = $state<string | null>(null);

	// @mention autocomplete
	let mentionQuery = $state('');
	let mentionStart = $state(-1);
	let mentionIdx = $state(0);
	let showMentionPopup = $state(false);
	let mentionPopupEl = $state<HTMLElement | null>(null);

	let mentionMatches = $derived(
		showMentionPopup
			? $serverMembers
				.filter((m) => m.user.id !== $currentUser?.id &&
					m.user.display_name.toLowerCase().includes(mentionQuery.toLowerCase()))
				.slice(0, 8)
			: [] as ServerMember[]
	);

	$effect(() => {
		if (!showMentionPopup || !mentionPopupEl) return;
		const items = mentionPopupEl.querySelectorAll<HTMLElement>('.tm-mention-item');
		items[mentionIdx]?.scrollIntoView({ block: 'nearest' });
	});

	onMount(() => {
		load();
		socket.subscribe('thread:' + thread.id);
		const unsub = socket.on((event) => {
			if (event.type === 'thread.message.new' && event.payload.thread_id === thread.id) {
				messages = [...messages, event.payload];
				scrollBottom();
			}
			if (event.type === 'thread.message.edit' && event.payload.thread_id === thread.id) {
				messages = messages.map((m) => m.id === event.payload.id ? event.payload : m);
			}
			if (event.type === 'thread.message.delete' && event.payload.thread_id === thread.id) {
				messages = messages.filter((m) => m.id !== event.payload.id);
			}
		});
		return () => { unsub(); socket.unsubscribe('thread:' + thread.id); };
	});

	async function load() {
		messages = await api.listThreadMessages(thread.id).catch(() => []);
		await tick();
		scrollBottom();
	}

	function scrollBottom() {
		if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
	}

	function onInput() {
		const el = textarea;
		if (!el) return;
		const pos = el.selectionStart ?? 0;
		const before = input.slice(0, pos);
		const match = before.match(/@(\w*)$/);
		if (match && $serverMembers.length > 0) {
			mentionQuery = match[1];
			mentionStart = pos - match[0].length;
			mentionIdx = 0;
			showMentionPopup = true;
		} else {
			showMentionPopup = false;
		}
	}

	function insertMention(member: ServerMember) {
		const handle = member.user.display_name.replace(/\s+/g, '_');
		const el = textarea;
		if (!el) return;
		const curPos = el.selectionStart ?? input.length;
		input = input.slice(0, mentionStart) + '@' + handle + ' ' + input.slice(curPos);
		showMentionPopup = false;
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = mentionStart + handle.length + 2;
		}, 0);
	}

	function onKeydown(e: KeyboardEvent) {
		if (showMentionPopup && mentionMatches.length > 0) {
			if (e.key === 'ArrowDown') { e.preventDefault(); mentionIdx = (mentionIdx + 1) % mentionMatches.length; return; }
			if (e.key === 'ArrowUp') { e.preventDefault(); mentionIdx = (mentionIdx - 1 + mentionMatches.length) % mentionMatches.length; return; }
			if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); insertMention(mentionMatches[mentionIdx]); return; }
			if (e.key === 'Escape') { showMentionPopup = false; return; }
		}
		if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
		if (e.key === 'Escape' && replyingTo) { replyingTo = null; }
	}

	async function send() {
		const text = input.trim();
		if (!text || sending) return;
		sending = true;
		const replyId = replyingTo?.id;
		input = '';
		replyingTo = null;
		showMentionPopup = false;
		try {
			await api.sendThreadMessage(thread.id, text, replyId);
		} finally {
			sending = false;
		}
	}

	async function uploadAndSend(file: File) {
		const isImage = file.type.startsWith('image/');
		const isVideo = file.type.startsWith('video/');
		if (!isImage && !isVideo) return;
		uploading = true;
		try {
			const { url } = await api.uploadFile(file);
			await api.sendThreadMessage(thread.id, url);
		} finally {
			uploading = false;
		}
	}

	function startEdit(msg: ThreadMessage) {
		editingId = msg.id;
		editContent = msg.content;
		replyingTo = null;
	}

	function cancelEdit() {
		editingId = null;
		editContent = '';
	}

	async function saveEdit(msg: ThreadMessage) {
		if (!editContent.trim()) return;
		await api.editThreadMessage(thread.id, msg.id, editContent.trim());
		cancelEdit();
	}

	async function deleteMsg(msg: ThreadMessage) {
		await api.deleteThreadMessage(thread.id, msg.id);
	}

	function startReply(msg: ThreadMessage) {
		replyingTo = msg;
		editingId = null;
		setTimeout(() => textarea?.focus(), 0);
	}

	function insertEmoji(emoji: string) {
		const el = textarea;
		if (!el) { input += emoji; return; }
		const start = el.selectionStart ?? input.length;
		const end = el.selectionEnd ?? input.length;
		input = input.slice(0, start) + emoji + input.slice(end);
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = start + emoji.length;
		}, 0);
	}

	function onPaste(e: ClipboardEvent) {
		const media = Array.from(e.clipboardData?.items ?? []).find(
			(i) => i.type.startsWith('image/') || i.type.startsWith('video/')
		);
		if (!media) return;
		e.preventDefault();
		const file = media.getAsFile();
		if (file) uploadAndSend(file);
	}

	function onBeforeInput(e: InputEvent) {
		const file = e.dataTransfer?.files?.[0];
		if (!file?.type.startsWith('image/') && !file?.type.startsWith('video/')) return;
		e.preventDefault();
		uploadAndSend(file);
	}

	function fmt(iso: string): string {
		return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleDateString([], { month: 'short', day: 'numeric' });
	}

	function sameDay(a: string, b: string): boolean {
		return new Date(a).toDateString() === new Date(b).toDateString();
	}

	function isImageUrl(text: string): boolean {
		const t = text.trim();
		return /^\/avatars\/[a-f0-9-]+\.(jpg|jpeg|png|gif|webp|svg)$/i.test(t) ||
			/^https?:\/\/[^/]+\/avatars\/[a-f0-9-]+\.(jpg|jpeg|png|gif|webp|svg)(\?.*)?$/i.test(t);
	}

	function isVideoUrl(text: string): boolean {
		const t = text.trim();
		return /^\/avatars\/[a-f0-9-]+\.(mp4|webm|mov)$/i.test(t) ||
			/^https?:\/\/[^/]+\/avatars\/[a-f0-9-]+\.(mp4|webm|mov)(\?.*)?$/i.test(t);
	}

	function replyPreview(content: string): string {
		if (isImageUrl(content)) return '[image]';
		if (isVideoUrl(content)) return '[video]';
		return content.length > 80 ? content.slice(0, 80) + '…' : content;
	}
</script>

<div class="thread-view">
	<div class="tv-header">
		<button class="back-btn" onclick={onback}>
			<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/></svg>
			Back
		</button>
		<div class="tv-title">
			<span class="tv-icon">💬</span>
			<span>{thread.title}</span>
		</div>
	</div>

	<div class="tv-messages" bind:this={scrollEl}>
		{#if messages.length === 0}
			<div class="tv-empty">No replies yet. Be the first!</div>
		{:else}
			{#each messages as msg, i}
				{#if i === 0 || !sameDay(messages[i - 1].created_at, msg.created_at)}
					<div class="date-divider"><span>{fmtDate(msg.created_at)}</span></div>
				{/if}
				{@const isOwn = msg.author.id === $currentUser?.id}
				{@const canDelete = isOwn || isAdmin}
				{@const editing = editingId === msg.id}
				<div class="tv-msg" class:editing>
					<img
						src={msg.author.avatar_url || '/default-avatar.png'}
						alt=""
						class="tv-avatar"
					/>
					<div class="tv-content">
						<div class="tv-meta">
							<span class="tv-author">{msg.author.display_name}</span>
							<span class="tv-time">{fmt(msg.created_at)}</span>
							{#if msg.edited_at}
								<span class="tv-edited">(edited)</span>
							{/if}
						</div>
						{#if msg.reply_to}
							<div class="tv-reply-quote">
								<span class="tv-reply-author">{msg.reply_to.author_name}</span>
								<span class="tv-reply-content">{replyPreview(msg.reply_to.content)}</span>
							</div>
						{/if}
						{#if editing}
							<textarea
								class="edit-input"
								bind:value={editContent}
								rows="2"
								autofocus
								onkeydown={(e) => {
									if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); saveEdit(msg); }
									if (e.key === 'Escape') cancelEdit();
								}}
							></textarea>
							<div class="edit-hint">Enter to save · Esc to cancel</div>
						{:else if isImageUrl(msg.content)}
							<img src={msg.content} alt="uploaded" class="tv-media tv-media-img" loading="lazy" onclick={() => lightboxSrc = msg.content} />
						{:else if isVideoUrl(msg.content)}
							<!-- svelte-ignore a11y-media-has-caption -->
							<video src={msg.content} class="tv-media" controls preload="metadata"></video>
						{:else}
							<div class="tv-text">{msg.content}</div>
						{/if}
					</div>
					{#if !editing}
						<div class="tv-actions">
							<button class="tv-action-btn" onclick={() => startReply(msg)} title="Reply">↩</button>
							{#if isOwn}
								<button class="tv-action-btn" onclick={() => startEdit(msg)} title="Edit">✏</button>
							{/if}
							{#if canDelete}
								<button class="tv-action-btn delete" onclick={() => deleteMsg(msg)} title="Delete">✕</button>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		{/if}
	</div>

	<div class="tv-compose">
		{#if replyingTo}
			<div class="tv-reply-bar">
				<span class="tv-reply-bar-label">Replying to <strong>{replyingTo.author.display_name}</strong></span>
				<span class="tv-reply-bar-preview">{replyPreview(replyingTo.content)}</span>
				<button class="tv-reply-cancel" onclick={() => (replyingTo = null)} title="Cancel reply">✕</button>
			</div>
		{/if}

		{#if showMentionPopup && mentionMatches.length > 0}
			<div class="tm-mention-popup" bind:this={mentionPopupEl}>
				{#each mentionMatches as member, i}
					<button
						class="tm-mention-item"
						class:selected={i === mentionIdx}
						onmousedown={(e) => { e.preventDefault(); insertMention(member); }}
						onmouseenter={() => (mentionIdx = i)}
					>
						<img src={member.user.avatar_url || '/default-avatar.png'} alt="" class="tm-mention-avatar" />
						<span class="tm-mention-name">{member.user.display_name}</span>
						{#if member.space_roles?.length}
							<span class="tm-mention-role" style="color:{member.space_roles[0].color}">{member.space_roles[0].name}</span>
						{/if}
					</button>
				{/each}
			</div>
		{/if}

		<div class="tv-input-row">
			<div class="tv-input-actions">
				<button
					class="action-icon"
					title="Upload image or video"
					disabled={uploading || sending}
					onclick={() => fileInput.click()}
				>
					{#if uploading}
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
					{:else}
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>
					{/if}
				</button>
				<button
					class="action-icon"
					class:active={showEmoji}
					title="Emoji"
					onclick={() => (showEmoji = !showEmoji)}
				>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M8 13s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
				</button>
				{#if showEmoji}
					<EmojiPicker onSelect={(e) => { insertEmoji(e); showEmoji = false; }} onClose={() => (showEmoji = false)} />
				{/if}
			</div>
			<input bind:this={fileInput} type="file" accept="image/*,video/mp4,video/webm,video/quicktime" style="display:none"
				onchange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) uploadAndSend(f); (e.target as HTMLInputElement).value = ''; }} />
			<textarea
				bind:this={textarea}
				bind:value={input}
				placeholder={replyingTo ? `Reply to ${replyingTo.author.display_name}…` : 'Reply in thread…'}
				rows="1"
				disabled={sending || uploading}
				onpaste={onPaste}
				onbeforeinput={onBeforeInput}
				oninput={onInput}
				onkeydown={onKeydown}
			></textarea>
			<button class="send-btn" onclick={send} disabled={sending || uploading || !input.trim()} aria-label="Send">
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/></svg>
			</button>
		</div>
	</div>
{#if lightboxSrc}
	<LightboxImage src={lightboxSrc} onclose={() => lightboxSrc = null} />
{/if}
</div>

<style>
	.thread-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}
	.tv-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--border);
		background: var(--bg-panel);
		flex-shrink: 0;
	}
	.back-btn {
		display: flex;
		align-items: center;
		gap: 5px;
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.35rem 0.7rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.82rem;
		flex-shrink: 0;
	}
	.back-btn:hover { color: var(--text); border-color: var(--accent); }
	.tv-title {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--text);
		min-width: 0;
	}
	.tv-icon { flex-shrink: 0; }
	.tv-title span:last-child {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tv-messages {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.tv-empty {
		color: var(--text-muted);
		font-size: 0.9rem;
		text-align: center;
		padding: 2rem;
	}
	.date-divider {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin: 0.5rem 0;
	}
	.date-divider::before, .date-divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
	}
	.date-divider span {
		font-size: 0.75rem;
		color: var(--text-muted);
		white-space: nowrap;
	}
	.tv-msg {
		display: flex;
		gap: 0.6rem;
		padding: 0.25rem 0.4rem;
		border-radius: 6px;
		position: relative;
	}
	.tv-msg:hover { background: rgba(255,255,255,0.03); }
	.tv-msg:hover .tv-actions { opacity: 1; }
	.tv-msg.editing { background: rgba(232,84,30,0.05); }
	.tv-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
		margin-top: 2px;
	}
	.tv-content { flex: 1; min-width: 0; padding-right: 4.5rem; }
	.tv-meta {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		margin-bottom: 2px;
	}
	.tv-author {
		font-size: 0.88rem;
		font-weight: 600;
		color: var(--text);
	}
	.tv-time {
		font-size: 0.72rem;
		color: var(--text-muted);
	}
	.tv-edited {
		font-size: 0.65rem;
		color: var(--text-muted);
		font-style: italic;
	}
	.tv-reply-quote {
		display: flex;
		align-items: baseline;
		gap: 0.4rem;
		background: var(--bg-input);
		border-left: 3px solid var(--accent);
		border-radius: 0 4px 4px 0;
		padding: 0.2rem 0.5rem;
		margin-bottom: 0.25rem;
		font-size: 0.82rem;
		cursor: default;
		overflow: hidden;
	}
	.tv-reply-author {
		font-weight: 600;
		color: var(--accent);
		white-space: nowrap;
		flex-shrink: 0;
	}
	.tv-reply-content {
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tv-text {
		font-size: 0.9rem;
		color: var(--text);
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.tv-media {
		max-width: min(480px, 100%);
		max-height: 300px;
		border-radius: 6px;
		display: block;
		margin-top: 0.25rem;
	}
	.tv-media-img { cursor: pointer; }
	.tv-media-img:hover { opacity: 0.9; }
	.tv-actions {
		position: absolute;
		right: 0.5rem;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		gap: 0.25rem;
		opacity: 0;
		transition: opacity 0.1s;
	}
	@media (max-width: 767px) { .tv-actions { opacity: 1; } }
	.tv-action-btn {
		background: #222228;
		border: 1px solid var(--border);
		color: var(--text);
		width: 26px;
		height: 26px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.7rem;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.tv-action-btn:hover { background: var(--border); }
	.tv-action-btn.delete:hover { background: #e04545; border-color: #e04545; }
	.edit-input {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--accent);
		border-radius: 6px;
		color: var(--text);
		padding: 0.5rem;
		font-size: 0.9rem;
		font-family: inherit;
		resize: none;
		outline: none;
		line-height: 1.5;
	}
	.edit-hint {
		font-size: 0.7rem;
		color: var(--text-muted);
		margin-top: 0.2rem;
	}
	/* compose area */
	.tv-compose {
		flex-shrink: 0;
		border-top: 1px solid var(--border);
		background: var(--bg-panel);
		position: relative;
	}
	.tv-reply-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 1rem;
		background: rgba(255,255,255,0.03);
		border-bottom: 1px solid var(--border);
		font-size: 0.8rem;
		overflow: hidden;
	}
	.tv-reply-bar-label { color: var(--text-muted); white-space: nowrap; flex-shrink: 0; }
	.tv-reply-bar-label strong { color: var(--accent); }
	.tv-reply-bar-preview {
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
	}
	.tv-reply-cancel {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.9rem;
		padding: 2px 4px;
		flex-shrink: 0;
		line-height: 1;
	}
	.tv-reply-cancel:hover { color: var(--text); }
	/* mention popup */
	.tm-mention-popup {
		position: absolute;
		bottom: calc(100%);
		left: 0;
		right: 0;
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-bottom: none;
		border-radius: 8px 8px 0 0;
		overflow: hidden;
		box-shadow: 0 -4px 12px rgba(0,0,0,0.4);
		max-height: 240px;
		overflow-y: auto;
		z-index: 10;
	}
	.tm-mention-item {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: 100%;
		padding: 0.4rem 0.75rem;
		background: none;
		border: none;
		color: var(--text);
		cursor: pointer;
		text-align: left;
		font-size: 0.9rem;
		font-family: inherit;
	}
	.tm-mention-item:hover, .tm-mention-item.selected { background: var(--bg-input); }
	.tm-mention-avatar {
		width: 26px;
		height: 26px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
	}
	.tm-mention-name { font-weight: 500; flex: 1; }
	.tm-mention-role { font-size: 0.75rem; }
	/* input row */
	.tv-input-row {
		display: flex;
		align-items: flex-end;
		gap: 0.4rem;
		padding: 0.75rem 1rem;
	}
	.tv-input-actions {
		display: flex;
		flex-direction: column;
		gap: 2px;
		flex-shrink: 0;
		position: relative;
	}
	.action-icon {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 5px;
		border-radius: 4px;
		display: flex;
		align-items: center;
	}
	.action-icon:hover, .action-icon.active { color: var(--accent); background: rgba(255,255,255,0.05); }
	.action-icon:disabled { opacity: 0.4; cursor: not-allowed; }
	.tv-input-row textarea {
		flex: 1;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.6rem 0.75rem;
		border-radius: 8px;
		font-size: 0.9rem;
		outline: none;
		font-family: inherit;
		resize: none;
		line-height: 1.45;
		max-height: 150px;
		overflow-y: auto;
	}
	.tv-input-row textarea:focus { border-color: var(--accent); }
	.send-btn {
		background: var(--accent);
		border: none;
		color: white;
		width: 38px;
		height: 38px;
		border-radius: 8px;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		flex-shrink: 0;
	}
	.send-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.send-btn:not(:disabled):hover { filter: brightness(1.1); }
</style>
