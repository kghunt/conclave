<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Message, type DirectMessage } from '$lib/api';
	import { socket } from '$lib/socket';
	import { currentUser, servers, activeServer, channels, activeChannel, dmConversations, activeDM, showProfileModal } from '$lib/stores';
	import ServerList from '$lib/components/ServerList.svelte';
	import ChannelSidebar from '$lib/components/ChannelSidebar.svelte';
	import MessageFeed from '$lib/components/MessageFeed.svelte';
	import MemberList from '$lib/components/MemberList.svelte';
	import ProfileModal from '$lib/components/ProfileModal.svelte';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';

	let messages: Message[] = $state([]);
	let dmMessages: DirectMessage[] = $state([]);
	let input = $state('');
	let showMembers = $state(true);

	onMount(async () => {
		const s = await api.listServers();
		servers.set(s);
		const convs = await api.listConversations();
		dmConversations.set(convs);
	});

	// Load channels when active server changes; auto-select first channel
	$effect(() => {
		const srv = $activeServer;
		if (!srv) return;
		// Capture srv.id so the async callback doesn't close over a stale store value
		const id = srv.id;
		api.listChannels(id).then((ch) => {
			channels.set(ch);
			if (ch.length > 0) activeChannel.set(ch[0]);
		});
	});

	// Load messages and subscribe to WS when active channel changes
	$effect(() => {
		const ch = $activeChannel;
		const srv = $activeServer;
		if (!ch || !srv) return;

		messages = [];
		const channelId = ch.id;
		const serverId = srv.id;
		const room = 'channel:' + channelId;

		api.listMessages(serverId, channelId).then((m) => (messages = m));
		api.markRead(serverId, channelId);
		channels.update((cs) => cs.map((c) => c.id === channelId ? { ...c, unread_count: 0 } : c));
		socket.subscribe(room);

		const unsub = socket.on((event) => {
			if (event.type === 'message.new' && event.payload.channel_id === channelId) {
				messages = [...messages, event.payload];
				api.markRead(serverId, channelId);
				channels.update((cs) => cs.map((c) => c.id === channelId ? { ...c, unread_count: 0 } : c));
			}
			if (event.type === 'message.edit' && event.payload.channel_id === channelId) {
				messages = messages.map((m) => m.id === event.payload.id ? event.payload : m);
			}
			if (event.type === 'message.delete' && event.payload.channel_id === channelId) {
				messages = messages.filter((m) => m.id !== event.payload.id);
			}
		});

		return () => {
			unsub();
			socket.unsubscribe(room);
		};
	});

	// Load DM messages and subscribe to WS when active DM changes
	$effect(() => {
		const dm = $activeDM;
		if (!dm) return;

		dmMessages = [];
		const convId = dm.id;
		const room = 'dm:' + convId;

		api.listDMMessages(convId).then((m) => (dmMessages = m));
		socket.subscribe(room);

		const unsub = socket.on((event) => {
			if (event.type === 'dm.new' && event.payload.conversation_id === convId) {
				dmMessages = [...dmMessages, event.payload];
			}
			if (event.type === 'dm.delete' && event.payload.conversation_id === convId) {
				dmMessages = dmMessages.filter((m) => m.id !== event.payload.id);
			}
		});

		return () => {
			unsub();
			socket.unsubscribe(room);
		};
	});

	let uploading = $state(false);
	let showEmoji = $state(false);
	let fileInput: HTMLInputElement;
	let textarea: HTMLTextAreaElement;

	function insertEmoji(emoji: string) {
		const el = textarea;
		if (!el) { input += emoji; return; }
		const start = el.selectionStart ?? input.length;
		const end = el.selectionEnd ?? input.length;
		input = input.slice(0, start) + emoji + input.slice(end);
		// restore cursor after the inserted emoji
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = start + emoji.length;
		}, 0);
	}

	async function send() {
		const text = input.trim();
		if (!text) return;
		input = '';
		if ($activeDM) {
			await api.sendDM($activeDM.id, text);
		} else if ($activeChannel && $activeServer) {
			await api.sendMessage($activeServer.id, $activeChannel.id, text);
		}
	}

	async function uploadAndSend(file: File) {
		if (!file.type.startsWith('image/')) return;
		uploading = true;
		try {
			const { url } = await api.uploadFile(file);
			if ($activeDM) {
				await api.sendDM($activeDM.id, url);
			} else if ($activeChannel && $activeServer) {
				await api.sendMessage($activeServer.id, $activeChannel.id, url);
			}
		} finally {
			uploading = false;
		}
	}

	async function onPaste(e: ClipboardEvent) {
		const image = Array.from(e.clipboardData?.items ?? []).find((i) => i.type.startsWith('image/'));
		if (!image) return;
		e.preventDefault();
		const file = image.getAsFile();
		if (file) uploadAndSend(file);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}
</script>

<div class="app">
	<ServerList />
	<ChannelSidebar />

	<main class="main">
		<header>
			<span class="channel-name">
				{#if $activeChannel}# {$activeChannel.name}{/if}
				{#if $activeDM}@ {$activeDM.other_user.display_name}{/if}
			</span>
			<div class="header-actions">
				{#if $activeChannel}
					<button onclick={() => (showMembers = !showMembers)} class="icon-btn" title="Members">
						&#128101;
					</button>
				{/if}
			</div>
		</header>

		<MessageFeed
			messages={$activeChannel ? messages : dmMessages}
			isDM={!!$activeDM}
		/>

		<div class="input-area">
			<div class="input-actions">
				<button
					class="action-icon"
					disabled={(!$activeChannel && !$activeDM) || uploading}
					onclick={() => fileInput.click()}
					title="Upload image"
				>
					{#if uploading}
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
					{:else}
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>
					{/if}
				</button>
				<button
					class="action-icon"
					disabled={!$activeChannel && !$activeDM}
					onclick={() => (showEmoji = !showEmoji)}
					title="Emoji"
					class:active={showEmoji}
				>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M8 13s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
				</button>
				{#if showEmoji}
					<EmojiPicker onSelect={insertEmoji} onClose={() => (showEmoji = false)} />
				{/if}
			</div>
			<input bind:this={fileInput} type="file" accept="image/*" style="display:none"
				onchange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) uploadAndSend(f); }} />
			<textarea
				bind:this={textarea}
				bind:value={input}
				onkeydown={onKeydown}
				onpaste={onPaste}
				placeholder={$activeChannel ? `Message #${$activeChannel.name}` : $activeDM ? `Message ${$activeDM.other_user.display_name}` : 'Select a channel'}
				rows="1"
				disabled={(!$activeChannel && !$activeDM) || uploading}
			></textarea>
		</div>
	</main>

	{#if showMembers && $activeChannel && $activeServer}
		<MemberList serverId={$activeServer.id} />
	{/if}
</div>

{#if $showProfileModal}
	<ProfileModal onclose={() => showProfileModal.set(false)} />
{/if}

<style>
	:global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
	:global(body) { background: #111113; color: #f0eff4; font-family: system-ui, sans-serif; }

	.app {
		display: flex;
		height: 100vh;
		overflow: hidden;
	}
	.main {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 1rem;
		height: 48px;
		border-bottom: 1px solid #0e0e10;
		background: #1c1c21;
		flex-shrink: 0;
	}
	.channel-name {
		font-weight: 600;
		color: #f0eff4;
	}
	.header-actions { display: flex; gap: 0.5rem; }
	.icon-btn {
		background: none;
		border: none;
		cursor: pointer;
		font-size: 1.1rem;
		padding: 0.25rem;
		border-radius: 4px;
		opacity: 0.7;
	}
	.icon-btn:hover { opacity: 1; background: rgba(255,255,255,0.1); }
	.input-area {
		padding: 0.75rem 1rem;
		flex-shrink: 0;
		display: flex;
		gap: 0.5rem;
		align-items: flex-end;
	}
	.input-actions {
		display: flex;
		gap: 0.25rem;
		flex-shrink: 0;
		position: relative;
	}
	.action-icon {
		background: #2e2e38;
		border: 1px solid #3a3a45;
		color: #c8c7d0;
		width: 36px;
		height: 36px;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.15s, color 0.15s;
	}
	.action-icon:hover:not(:disabled) { background: #3a3a45; color: #f0eff4; }
	.action-icon.active { background: #e8541e; border-color: #e8541e; color: white; }
	.action-icon:disabled { opacity: 0.35; cursor: not-allowed; }
	textarea {
		width: 100%;
		background: #26262b;
		border: 1px solid #2e2e38;
		border-radius: 8px;
		color: #f0eff4;
		padding: 0.75rem 1rem;
		font-size: 0.95rem;
		resize: none;
		outline: none;
		font-family: inherit;
	}
	textarea:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
