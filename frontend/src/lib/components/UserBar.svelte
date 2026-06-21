<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { currentUser, showProfileModal } from '$lib/stores';
	import Avatar from './Avatar.svelte';
	import AdminPanel from './AdminPanel.svelte';

	let showAdmin = $state(false);
	let pushSupported = $state(false);
	let pushSubscribed = $state(false);
	let pushToggling = $state(false);

	onMount(async () => {
		pushSupported = 'PushManager' in window && 'serviceWorker' in navigator;
		if (!pushSupported) return;
		navigator.serviceWorker.ready.then((reg) => {
			reg.pushManager.getSubscription().then((sub) => { pushSubscribed = !!sub; });
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
				if (sub) { await api.pushUnsubscribe(sub.endpoint); await sub.unsubscribe(); }
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
				await api.pushSubscribe({ endpoint: sub.endpoint, p256dh: json.keys?.['p256dh'] ?? '', auth: json.keys?.['auth'] ?? '' });
				pushSubscribed = true;
			}
		} catch (e) { console.error('push toggle failed', e); }
		finally { pushToggling = false; }
	}

	async function logout() {
		await api.logout();
		location.href = '/login';
	}
</script>

<div class="user-bar">
	{#if $currentUser}
		<button class="user-info" onclick={() => showProfileModal.set(true)} title="Edit profile">
			<Avatar url={$currentUser.avatar_url} name={$currentUser.display_name} userId={$currentUser.id} size={32} showPresence />
			<span class="username">{$currentUser.display_name}</span>
		</button>
		{#if pushSupported}
			<button class="icon-bar-btn" class:active={pushSubscribed} onclick={toggleNotifications} disabled={pushToggling}
				title={pushSubscribed ? 'Disable notifications' : 'Enable notifications'}>
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

{#if showAdmin}
	<AdminPanel onclose={() => (showAdmin = false)} />
{/if}

<style>
	.user-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 0.75rem;
		background: #0e0e10;
		flex-shrink: 0;
		position: relative;
	}
	.user-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.25rem;
		border-radius: 4px;
		flex: 1;
		min-width: 0;
		text-align: left;
	}
	.user-info:hover { background: rgba(255,255,255,0.06); }
	.username {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--text);
		flex: 1;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.icon-bar-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.2rem;
		border-radius: 3px;
	}
	.icon-bar-btn:hover { color: var(--text); background: rgba(255,255,255,0.07); }
	.icon-bar-btn.active { color: var(--accent); }
	.icon-bar-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.logout-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 1rem;
	}
	.logout-btn:hover { color: #e04545; }
	.admin-btn {
		background: none;
		border: none;
		color: var(--accent);
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.15rem 0.25rem;
		border-radius: 3px;
		opacity: 0.8;
	}
	.admin-btn:hover { opacity: 1; background: rgba(232,84,30,0.15); }
</style>
