<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { api, type Thread, type ThreadMessage } from '$lib/api';
	import { currentUser } from '$lib/stores';
	import { socket } from '$lib/socket';

	interface Props {
		thread: Thread;
		onback: () => void;
	}
	let { thread, onback }: Props = $props();

	let messages = $state<ThreadMessage[]>([]);
	let input = $state('');
	let sending = $state(false);
	let scrollEl: HTMLElement;

	onMount(() => {
		load();
		socket.subscribe('thread:' + thread.id);
		const unsub = socket.on((event) => {
			if (event.type === 'thread.message.new' && event.payload.thread_id === thread.id) {
				messages = [...messages, event.payload];
				scrollBottom();
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

	async function send() {
		const text = input.trim();
		if (!text || sending) return;
		sending = true;
		input = '';
		try {
			await api.sendThreadMessage(thread.id, text);
		} finally {
			sending = false;
		}
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
				<div class="tv-msg" class:own={msg.author.id === $currentUser?.id}>
					<img
						src={msg.author.avatar_url || '/default-avatar.png'}
						alt=""
						class="tv-avatar"
					/>
					<div class="tv-content">
						<div class="tv-meta">
							<span class="tv-author">{msg.author.display_name}</span>
							<span class="tv-time">{fmt(msg.created_at)}</span>
						</div>
						<div class="tv-text">{msg.content}</div>
					</div>
				</div>
			{/each}
		{/if}
	</div>

	<div class="tv-input-row">
		<input
			bind:value={input}
			placeholder="Reply in thread…"
			disabled={sending}
			onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
		/>
		<button class="send-btn" onclick={send} disabled={sending || !input.trim()}>
			<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/></svg>
		</button>
	</div>
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
	}
	.tv-msg:hover { background: rgba(255,255,255,0.03); }
	.tv-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
		margin-top: 2px;
	}
	.tv-content { flex: 1; min-width: 0; }
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
	.tv-text {
		font-size: 0.9rem;
		color: var(--text);
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.tv-input-row {
		display: flex;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-top: 1px solid var(--border);
		background: var(--bg-panel);
		flex-shrink: 0;
	}
	.tv-input-row input {
		flex: 1;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.6rem 0.75rem;
		border-radius: 8px;
		font-size: 0.9rem;
		outline: none;
		font-family: inherit;
	}
	.tv-input-row input:focus { border-color: var(--accent); }
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
