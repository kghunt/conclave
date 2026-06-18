<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { servers, activeServer, channels, activeChannel } from '$lib/stores';

	let status: 'joining' | 'error' = $state('joining');
	let errorMsg = $state('');

	onMount(async () => {
		const code = $page.params.code ?? '';
		try {
			const { server_id } = await api.joinByInvite(code);
			const updated = await api.listServers();
			servers.set(updated ?? []);
			const joined = updated?.find((s) => s.id === server_id);
			if (joined) {
				activeServer.set(joined);
				activeChannel.set(null);
				channels.set([]);
			}
			goto('/');
		} catch (e: any) {
			status = 'error';
			errorMsg = e.message ?? 'Invalid or expired invite link';
		}
	});
</script>

<div class="wrap">
	<div class="card">
		<div class="logo">
			<svg width="40" height="40" viewBox="0 0 32 32">
				<rect width="32" height="32" rx="7" fill="var(--bg-panel)"/>
				<path d="M7 9a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2h-4l-4 4-4-4H9a2 2 0 0 1-2-2V9z" fill="var(--accent)"/>
			</svg>
			<span>Conclave</span>
		</div>

		{#if status === 'joining'}
			<p class="status">Joining space…</p>
			<div class="spinner"></div>
		{:else}
			<p class="error">{errorMsg}</p>
			<a href="/" class="home-btn">Go to Conclave</a>
		{/if}
	</div>
</div>

<style>
	:global(body) { background: var(--bg); }
	.wrap {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background: var(--bg);
	}
	.card {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 2.5rem;
		text-align: center;
		width: 320px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
	}
	.logo {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--text);
	}
	.status { color: var(--text-muted); font-size: 0.95rem; }
	.error {
		color: #e04545;
		font-size: 0.9rem;
		background: rgba(224,69,69,0.1);
		padding: 0.6rem 1rem;
		border-radius: 6px;
		width: 100%;
	}
	.spinner {
		width: 28px;
		height: 28px;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.7s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }
	.home-btn {
		background: var(--accent);
		color: white;
		text-decoration: none;
		padding: 0.6rem 1.25rem;
		border-radius: 6px;
		font-size: 0.9rem;
		font-weight: 500;
	}
	.home-btn:hover { opacity: 0.9; }
</style>
