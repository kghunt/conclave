<script lang="ts">
	import { currentUser, activeServer, activeChannel, activeDM } from '$lib/stores';
	import { api, type Message, type DirectMessage } from '$lib/api';
	import Avatar from './Avatar.svelte';

	type AnyMessage = Message | DirectMessage;

	let {
		messages,
		isDM = false
	}: { messages: AnyMessage[]; isDM?: boolean } = $props();

	let container: HTMLElement;
	let stickToBottom = true;
	let editingId = $state<string | null>(null);
	let editContent = $state('');

	$effect(() => {
		messages.length;
		if (stickToBottom && container) {
			container.scrollTop = container.scrollHeight;
		}
	});

	function onScroll() {
		if (!container) return;
		stickToBottom = container.scrollTop + container.clientHeight >= container.scrollHeight - 50;
	}

	function isMessage(m: AnyMessage): m is Message {
		return 'author' in m;
	}

	function getAuthor(m: AnyMessage) {
		return isMessage(m) ? m.author : (m as DirectMessage).sender;
	}

	function formatTime(ts: string) {
		return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	function formatDate(ts: string) {
		return new Date(ts).toLocaleDateString([], { month: 'long', day: 'numeric', year: 'numeric' });
	}

	function sameDay(a: string, b: string) {
		return formatDate(a) === formatDate(b);
	}

	function isImageUrl(text: string): boolean {
		const t = text.trim();
		return /^https?:\/\/\S+\.(jpg|jpeg|png|gif|webp|svg)(\?.*)?$/i.test(t) ||
			/^\/avatars\/[a-f0-9-]+\.(jpg|jpeg|png|gif|webp|svg)$/i.test(t);
	}

	function startEdit(m: AnyMessage) {
		editingId = m.id;
		editContent = m.content;
	}

	function cancelEdit() {
		editingId = null;
		editContent = '';
	}

	async function saveEdit(m: AnyMessage) {
		if (!editContent.trim() || !$activeServer || !$activeChannel) return;
		await api.editMessage($activeServer.id, $activeChannel.id, m.id, editContent.trim());
		cancelEdit();
	}

	function onEditKeydown(e: KeyboardEvent, m: AnyMessage) {
		if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); saveEdit(m); }
		if (e.key === 'Escape') cancelEdit();
	}

	async function deleteMsg(m: AnyMessage) {
		if (isDM && $activeDM) {
			await api.deleteDM($activeDM.id, m.id);
		} else if (!isDM && $activeServer && $activeChannel) {
			await api.deleteMessage($activeServer.id, $activeChannel.id, m.id);
		}
	}
</script>

<div class="feed" bind:this={container} onscroll={onScroll}>
	{#if messages.length === 0}
		<div class="empty">No messages yet. Say hello!</div>
	{/if}

	{#each messages as msg, i}
		{#if i === 0 || !sameDay(messages[i - 1].created_at, msg.created_at)}
			<div class="date-divider">
				<span>{formatDate(msg.created_at)}</span>
			</div>
		{/if}

		{@const author = getAuthor(msg)}
		{@const isOwn = author.id === $currentUser?.id}
		{@const showHeader = i === 0 || getAuthor(messages[i - 1]).id !== author.id || !sameDay(messages[i-1].created_at, msg.created_at)}
		{@const editing = editingId === msg.id}

		<div class="message" class:editing>
			{#if showHeader}
				<div class="avatar">
					<Avatar url={author.avatar_url} name={author.display_name} userId={author.id} size={40} />
				</div>
				<div class="content">
					<div class="header">
						<span class="name">{author.display_name}</span>
						<span class="time">{formatTime(msg.created_at)}</span>
						{#if isMessage(msg) && msg.edited_at}
							<span class="edited">(edited)</span>
						{/if}
					</div>
					{#if editing}
						<textarea
							bind:value={editContent}
							onkeydown={(e) => onEditKeydown(e, msg)}
							class="edit-input"
							rows="2"
							autofocus
						></textarea>
						<div class="edit-hint">Enter to save · Esc to cancel</div>
					{:else if isImageUrl(msg.content)}
						<img src={msg.content} alt="uploaded" class="msg-image" loading="lazy" />
					{:else}
						<p>{msg.content}</p>
					{/if}
				</div>
			{:else}
				<div class="avatar-spacer"></div>
				<div class="content">
					{#if editing}
						<textarea
							bind:value={editContent}
							onkeydown={(e) => onEditKeydown(e, msg)}
							class="edit-input"
							rows="2"
							autofocus
						></textarea>
						<div class="edit-hint">Enter to save · Esc to cancel</div>
					{:else if isImageUrl(msg.content)}
						<img src={msg.content} alt="uploaded" class="msg-image" loading="lazy" />
					{:else}
						<p>{msg.content}</p>
					{/if}
				</div>
			{/if}

			{#if isOwn && !editing}
				<div class="msg-actions">
					{#if isMessage(msg)}
						<button class="action-btn edit" onclick={() => startEdit(msg)} title="Edit">✏</button>
					{/if}
					<button class="action-btn delete" onclick={() => deleteMsg(msg)} title="Delete">✕</button>
				</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	.feed {
		flex: 1;
		overflow-y: auto;
		padding: 1rem 0;
		display: flex;
		flex-direction: column;
	}
	.empty {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: #8b8b99;
		font-size: 0.9rem;
	}
	.date-divider {
		display: flex;
		align-items: center;
		padding: 0.5rem 1rem;
		gap: 0.75rem;
		color: #8b8b99;
		font-size: 0.75rem;
		font-weight: 600;
	}
	.date-divider::before, .date-divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: #2e2e38;
	}
	.message {
		display: flex;
		gap: 0.75rem;
		padding: 0.125rem 1rem;
		position: relative;
	}
	.message:hover { background: rgba(255,255,255,0.02); }
	.message:hover .msg-actions { opacity: 1; }
	.message.editing { background: rgba(232,84,30,0.05); }
	.avatar { flex-shrink: 0; }
	.avatar-spacer { width: 40px; flex-shrink: 0; }
	.content { flex: 1; min-width: 0; }
	.header {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		margin-bottom: 0.1rem;
	}
	.name { font-weight: 600; color: #f0eff4; font-size: 0.95rem; }
	.time { font-size: 0.7rem; color: #8b8b99; }
	.edited { font-size: 0.65rem; color: #8b8b99; font-style: italic; }
	p {
		color: #f0eff4;
		font-size: 0.9rem;
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.msg-image {
		max-width: min(480px, 100%);
		max-height: 320px;
		border-radius: 6px;
		display: block;
		margin-top: 0.25rem;
		cursor: pointer;
	}
	.msg-image:hover { opacity: 0.9; }
	.edit-input {
		width: 100%;
		background: #26262b;
		border: 1px solid #e8541e;
		border-radius: 6px;
		color: #f0eff4;
		padding: 0.5rem;
		font-size: 0.9rem;
		font-family: inherit;
		resize: none;
		outline: none;
		line-height: 1.5;
	}
	.edit-hint {
		font-size: 0.7rem;
		color: #8b8b99;
		margin-top: 0.2rem;
	}
	.msg-actions {
		position: absolute;
		right: 1rem;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		gap: 0.25rem;
		opacity: 0;
		transition: opacity 0.1s;
	}
	@media (max-width: 767px) {
		.msg-actions { opacity: 1; }
		.action-btn { width: 32px; height: 32px; }
		.message { padding: 0.25rem 0.75rem; padding-right: 5rem; }
	}
	.action-btn {
		background: #222228;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		width: 26px;
		height: 26px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.7rem;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.action-btn:hover { background: #2e2e38; }
	.action-btn.delete:hover { background: #e04545; border-color: #e04545; }
</style>
