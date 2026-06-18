<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Message, type DirectMessage } from '$lib/api';
	import { socket } from '$lib/socket';
	import { currentUser, servers, activeServer, channels, activeChannel, dmConversations, activeDM, showProfileModal, friends, friendRequests, friendRequestsSent } from '$lib/stores';
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
	let isMobile = $state(false);

	onMount(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		isMobile = mq.matches;
		const handler = (e: MediaQueryListEvent) => { isMobile = e.matches; };
		mq.addEventListener('change', handler);

		(async () => {
			const [s, convs, fr, reqs, sent] = await Promise.all([
				api.listServers(),
				api.listConversations(),
				api.listFriends(),
				api.listFriendRequests(),
				api.listFriendRequestsSent()
			]);
			servers.set(s ?? []);
			dmConversations.set(convs ?? []);
			friends.set(fr ?? []);
			friendRequests.set(reqs ?? []);
			friendRequestsSent.set(sent ?? []);

			// Restore last active server
			const lastServerId = localStorage.getItem('lastServerId');
			if (lastServerId) {
				const match = (s ?? []).find((sv) => sv.id === lastServerId);
				if (match) activeServer.set(match);
				else if ((s ?? []).length > 0) activeServer.set(s[0]);
			}
		})();

		return () => mq.removeEventListener('change', handler);
	});

	// On mobile: going back from chat clears the active channel/DM
	function mobileBack() {
		activeChannel.set(null);
		activeDM.set(null);
	}

	// Load channels when active server changes; auto-select first channel; persist choice
	$effect(() => {
		const srv = $activeServer;
		if (!srv) return;
		const id = srv.id;
		localStorage.setItem('lastServerId', id);
		api.listChannels(id).then((ch) => {
			channels.set(ch ?? []);
			if (ch?.length > 0) activeChannel.set(ch[0]);
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

		api.listMessages(serverId, channelId).then((m) => (messages = m ?? []));
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

		api.listDMMessages(convId).then((m) => (dmMessages = m ?? []));
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

	// Gboard (Android) sends GIFs via beforeinput, not paste
	function onBeforeInput(e: InputEvent) {
		const file = e.dataTransfer?.files?.[0];
		if (!file?.type.startsWith('image/')) return;
		e.preventDefault();
		uploadAndSend(file);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	// Whether the chat panel should be visible
	let showChat = $derived(!isMobile || !!$activeChannel || !!$activeDM);
	// Whether the sidebar should be visible
	let showSidebar = $derived(!isMobile || (!$activeChannel && !$activeDM));
</script>

<div class="app">
	<ServerList />

	{#if showSidebar}
		<ChannelSidebar />
	{/if}

	{#if showChat}
		<main class="main">
			<header>
				{#if isMobile}
					<button class="back-btn" onclick={mobileBack}>
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M15 18l-6-6 6-6"/></svg>
						{$activeDM ? 'Messages' : ($activeServer?.name ?? 'Channels')}
					</button>
				{/if}
				<span class="channel-name">
					{#if $activeChannel}# {$activeChannel.name}{/if}
					{#if $activeDM}@ {$activeDM.other_user.display_name}{/if}
				</span>
				<div class="header-actions">
					{#if $activeChannel}
						<button onclick={() => (showMembers = !showMembers)} class="icon-btn" class:active={showMembers} title="Members">
							<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
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
					onbeforeinput={onBeforeInput}
					placeholder={$activeChannel ? `Message #${$activeChannel.name}` : $activeDM ? `Message ${$activeDM.other_user.display_name}` : 'Select a channel'}
					rows="1"
					disabled={(!$activeChannel && !$activeDM) || uploading}
				></textarea>
			</div>
		</main>

		{#if showMembers && $activeChannel && $activeServer}
			{#if isMobile}
				<div class="members-overlay">
					<div class="members-overlay-header">
						<span>Members</span>
						<button onclick={() => (showMembers = false)}>✕</button>
					</div>
					<MemberList serverId={$activeServer.id} onDmStarted={() => (showMembers = false)} />
				</div>
			{:else}
				<MemberList serverId={$activeServer.id} onDmStarted={() => (showMembers = false)} />
			{/if}
		{/if}
	{/if}
</div>

{#if $showProfileModal}
	<ProfileModal onclose={() => showProfileModal.set(false)} />
{/if}

<style>
	:global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
	:global(body) { background: var(--bg); color: var(--text); font-family: system-ui, sans-serif; overflow: hidden; }

	.app {
		display: flex;
		height: 100dvh;
		overflow: hidden;
	}
	@media (max-width: 767px) {
		.app { flex-direction: column; }
	}
	.main {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-width: 0;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0 1rem;
		height: 48px;
		border-bottom: 1px solid #0e0e10;
		background: var(--bg-panel);
		flex-shrink: 0;
	}
	.back-btn {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		background: none;
		border: none;
		color: var(--accent);
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
		padding: 0.375rem 0.5rem;
		border-radius: 4px;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.back-btn:hover { background: rgba(232,84,30,0.1); }
	.channel-name {
		flex: 1;
		font-weight: 600;
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.header-actions { display: flex; gap: 0.5rem; flex-shrink: 0; }
	.icon-btn {
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.35rem;
		border-radius: 4px;
		color: #c8c7d0;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.icon-btn:hover { color: var(--text); background: rgba(255,255,255,0.1); }
	.icon-btn.active { color: var(--accent); }
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
		background: var(--border);
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
	.action-icon:hover:not(:disabled) { background: #3a3a45; color: var(--text); }
	.action-icon.active { background: var(--accent); border-color: var(--accent); color: white; }
	.action-icon:disabled { opacity: 0.35; cursor: not-allowed; }
	textarea {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 8px;
		color: var(--text);
		padding: 0.75rem 1rem;
		font-size: 0.95rem;
		resize: none;
		outline: none;
		font-family: inherit;
	}
	textarea:disabled { opacity: 0.5; cursor: not-allowed; }

	.members-overlay {
		display: none;
	}
	@media (max-width: 767px) {
		textarea { font-size: 16px; /* prevents iOS zoom on focus */ }
		.input-area { padding: 0.5rem; }
		.members-overlay {
			display: flex;
			flex-direction: column;
			position: fixed;
			inset: 0;
			z-index: 50;
			background: var(--bg-sidebar);
		}
		.members-overlay-header {
			display: flex;
			align-items: center;
			justify-content: space-between;
			padding: 0 1rem;
			height: 48px;
			border-bottom: 1px solid #0e0e10;
			font-weight: 700;
			flex-shrink: 0;
		}
		.members-overlay-header button {
			background: none;
			border: none;
			color: var(--text-muted);
			cursor: pointer;
			font-size: 1rem;
			padding: 0.25rem;
		}
	}
</style>
