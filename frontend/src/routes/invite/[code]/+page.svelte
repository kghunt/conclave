<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api';

	type Phase = 'loading' | 'rules' | 'joining' | 'error';
	let phase = $state<Phase>('loading');
	let serverName = $state('');
	let rules = $state('');
	let rulesAccepted = $state(false);
	let errorMsg = $state('');

	const code = $derived($page.params.code ?? '');

	onMount(async () => {
		try {
			const info = await api.getInviteInfo(code);
			serverName = info.server_name;
			rules = info.rules;
			if (rules) {
				phase = 'rules';
			} else {
				await doJoin();
			}
		} catch (e: any) {
			phase = 'error';
			errorMsg = e.message ?? 'Invalid or expired invite link';
		}
	});

	async function doJoin() {
		phase = 'joining';
		try {
			const { server_id } = await api.joinByInvite(code);
			// Save to localStorage so the root page's onMount selects this server on load
			localStorage.setItem('lastServerId', server_id);
			// Full navigation (not client-side goto) ensures a clean app init
			window.location.replace('/');
		} catch (e: any) {
			phase = 'error';
			errorMsg = e.message ?? 'Failed to join space';
		}
	}
</script>

<div class="wrap">
	<div class="card" class:wide={phase === 'rules'}>
		<div class="logo">
			<svg width="40" height="40" viewBox="0 0 32 32">
				<rect width="32" height="32" rx="7" fill="var(--bg-panel)"/>
				<path d="M7 9a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2h-4l-4 4-4-4H9a2 2 0 0 1-2-2V9z" fill="var(--accent)"/>
			</svg>
			<span>Conclave</span>
		</div>

		{#if phase === 'loading'}
			<p class="status">Loading…</p>
			<div class="spinner"></div>

		{:else if phase === 'rules'}
			<p class="status">You've been invited to <strong>{serverName}</strong>.<br>Please read and accept the rules before joining.</p>
			<div class="rules-box">
				<pre class="rules-text">{rules}</pre>
			</div>
			<label class="accept-label">
				<input type="checkbox" bind:checked={rulesAccepted} />
				I have read and agree to these rules
			</label>
			<button class="join-btn" disabled={!rulesAccepted} onclick={doJoin}>Accept & Join</button>

		{:else if phase === 'joining'}
			<p class="status">Joining {serverName}…</p>
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
		width: 340px;
		max-width: calc(100vw - 2rem);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
	}
	.card.wide { width: 560px; }
	.logo {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--text);
	}
	.status { color: var(--text-muted); font-size: 0.95rem; line-height: 1.5; }
	.status strong { color: var(--text); }
	.rules-box {
		width: 100%;
		max-height: 280px;
		overflow-y: auto;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1rem;
		text-align: left;
	}
	.rules-text {
		white-space: pre-wrap;
		word-break: break-word;
		font-family: inherit;
		font-size: 0.875rem;
		color: var(--text);
		line-height: 1.6;
		margin: 0;
	}
	.accept-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: var(--text);
		cursor: pointer;
		align-self: flex-start;
	}
	.join-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.6rem 1.5rem;
		border-radius: 6px;
		font-size: 0.95rem;
		font-weight: 600;
		cursor: pointer;
		width: 100%;
	}
	.join-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.join-btn:not(:disabled):hover { filter: brightness(1.1); }
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
