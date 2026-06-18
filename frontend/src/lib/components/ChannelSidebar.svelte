<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { activeServer, servers, channels, activeChannel, dmConversations, activeDM, currentUser, showProfileModal } from '$lib/stores';
	import type { Channel } from '$lib/api';
	import Avatar from './Avatar.svelte';
	import AdminPanel from './AdminPanel.svelte';

	let showAdmin = $state(false);

	// Push notification state
	let pushSupported = $state(false);
	let pushSubscribed = $state(false);
	let pushToggling = $state(false);

	onMount(() => {
		pushSupported = 'PushManager' in window && 'serviceWorker' in navigator;
		if (!pushSupported) return;
		navigator.serviceWorker.ready.then((reg) => {
			reg.pushManager.getSubscription().then((sub) => {
				pushSubscribed = !!sub;
			});
		});
	});

	function urlBase64ToUint8Array(base64: string): Uint8Array {
		const padding = '='.repeat((4 - (base64.length % 4)) % 4);
		const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
		const raw = atob(b64);
		const out = new Uint8Array(raw.length);
		for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
		return out;
	}

	async function toggleNotifications() {
		if (pushToggling) return;
		pushToggling = true;
		try {
			const reg = await navigator.serviceWorker.ready;
			if (pushSubscribed) {
				const sub = await reg.pushManager.getSubscription();
				if (sub) {
					await api.pushUnsubscribe(sub.endpoint);
					await sub.unsubscribe();
				}
				pushSubscribed = false;
			} else {
				const permission = await Notification.requestPermission();
				if (permission !== 'granted') return;
				const { public_key } = await api.getPushKey();
				if (!public_key) return;
				const sub = await reg.pushManager.subscribe({
					userVisibleOnly: true,
					applicationServerKey: urlBase64ToUint8Array(public_key).buffer as ArrayBuffer
				});
				const json = sub.toJSON();
				await api.pushSubscribe({
					endpoint: sub.endpoint,
					p256dh: json.keys?.['p256dh'] ?? '',
					auth: json.keys?.['auth'] ?? ''
				});
				pushSubscribed = true;
			}
		} catch (e) {
			console.error('push toggle failed', e);
		} finally {
			pushToggling = false;
		}
	}

	let showNewChannel = $state(false);
	let newChannelName = $state('');

	function selectChannel(ch: Channel) {
		activeDM.set(null);
		activeChannel.set(ch);
	}

	async function createChannel() {
		if (!newChannelName.trim() || !$activeServer) return;
		const ch = await api.createChannel($activeServer.id, { name: newChannelName, description: '' });
		channels.update((prev) => [...prev, ch]);
		selectChannel(ch);
		showNewChannel = false;
		newChannelName = '';
	}

	async function openDM(userId: string) {
		const conv = await api.getOrCreateDM(userId);
		dmConversations.update((prev) => {
			if (prev.find((c) => c.id === conv.id)) return prev;
			return [conv, ...prev];
		});
		activeDM.set(conv);
	}

	async function logout() {
		await api.logout();
		location.href = '/login';
	}
</script>

<aside class="sidebar">
	{#if $activeServer}
		<div class="server-header">
			<span>{$activeServer.name}</span>
		</div>

		<div class="section-label">
			<span>Channels</span>
			{#if $activeServer.role === 'owner' || $activeServer.role === 'admin'}
				<button class="add-btn" onclick={() => (showNewChannel = !showNewChannel)}>+</button>
			{/if}
		</div>

		{#if showNewChannel}
			<div class="new-channel">
				<input
					bind:value={newChannelName}
					placeholder="channel-name"
					onkeydown={(e) => e.key === 'Enter' && createChannel()}
				/>
				<button onclick={createChannel}>Add</button>
			</div>
		{/if}

		{#each $channels as ch}
			<button
				class="channel-item"
				class:active={$activeChannel?.id === ch.id}
				onclick={() => selectChannel(ch)}
			>
				<span># {ch.name}</span>
				{#if ch.unread_count > 0}
					<span class="badge">{ch.unread_count}</span>
				{/if}
			</button>
		{/each}
	{:else}
		<div class="server-header"><span>Direct Messages</span></div>
	{/if}

	<div class="section-label" style="margin-top: auto">Direct Messages</div>
	{#each $dmConversations as conv}
		<button
			class="channel-item"
			class:active={$activeDM?.id === conv.id}
			onclick={() => activeDM.set(conv)}
		>
			<Avatar url={conv.other_user.avatar_url} name={conv.other_user.display_name} userId={conv.other_user.id} size={20} />
			{conv.other_user.display_name}
		</button>
	{/each}

	<div class="user-bar">
		{#if $currentUser}
			<button class="user-info" onclick={() => showProfileModal.set(true)} title="Edit profile">
				<Avatar url={$currentUser.avatar_url} name={$currentUser.display_name} userId={$currentUser.id} size={32} />
				<span class="username">{$currentUser.display_name}</span>
			</button>
			{#if pushSupported}
				<button
					class="icon-bar-btn"
					class:active={pushSubscribed}
					onclick={toggleNotifications}
					disabled={pushToggling}
					title={pushSubscribed ? 'Disable notifications' : 'Enable notifications'}
				>
					{#if pushSubscribed}
						<svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><path d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6V11c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"/></svg>
					{:else}
						<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/><line x1="1" y1="1" x2="23" y2="23"/></svg>
					{/if}
				</button>
			{/if}
			{#if $currentUser?.is_instance_admin}
				<button class="admin-btn" onclick={() => (showAdmin = true)} title="Instance admin">⚙</button>
			{/if}
			<button class="logout-btn" onclick={logout} title="Logout">⏻</button>
		{/if}
	</div>
</aside>

{#if showAdmin}
	<AdminPanel onclose={() => (showAdmin = false)} />
{/if}

<style>
	.sidebar {
		width: 240px;
		background: #19191d;
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		overflow-y: auto;
	}
	@media (max-width: 767px) {
		.sidebar { width: 100%; flex: 1; }
	}
	.server-header {
		padding: 0.875rem 1rem;
		font-weight: 700;
		border-bottom: 1px solid #0e0e10;
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 0.95rem;
	}
	.section-label {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 0.75rem 0.25rem;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: #8b8b99;
		letter-spacing: 0.05em;
	}
	.add-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 1rem;
		padding: 0 0.25rem;
	}
	.add-btn:hover { color: #f0eff4; }
	.new-channel {
		display: flex;
		gap: 0.25rem;
		padding: 0.25rem 0.75rem;
	}
	.new-channel input {
		flex: 1;
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.new-channel button {
		background: #e8541e;
		border: none;
		color: white;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.85rem;
	}
	.channel-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: none;
		border: none;
		color: #8b8b99;
		padding: 0.375rem 0.75rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
		margin: 0 0.25rem;
		width: calc(100% - 0.5rem);
		font-size: 0.9rem;
	}
	@media (max-width: 767px) {
		.channel-item { padding: 0.65rem 0.75rem; font-size: 1rem; }
		.server-header { height: 52px; font-size: 1rem; }
	}
	.channel-item:hover, .channel-item.active {
		background: rgba(255,255,255,0.07);
		color: #f0eff4;
	}
	.badge {
		margin-left: auto;
		background: #e04545;
		color: white;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.4rem;
	}
	.user-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 0.75rem;
		background: #0e0e10;
		margin-top: auto;
		flex-shrink: 0;
	}
	.username {
		font-size: 0.85rem;
		font-weight: 600;
		color: #f0eff4;
		flex: 1;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.icon-bar-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.2rem;
		border-radius: 3px;
	}
	.icon-bar-btn:hover { color: #f0eff4; background: rgba(255,255,255,0.07); }
	.icon-bar-btn.active { color: #e8541e; }
	.icon-bar-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.logout-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 1rem;
	}
	.logout-btn:hover { color: #e04545; }
	.admin-btn {
		background: none;
		border: none;
		color: #e8541e;
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.15rem 0.25rem;
		border-radius: 3px;
		opacity: 0.8;
	}
	.admin-btn:hover { opacity: 1; background: rgba(232,84,30,0.15); }
	.user-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: 1;
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.25rem;
		border-radius: 4px;
		min-width: 0;
	}
	.user-info:hover { background: rgba(255,255,255,0.07); }
</style>
