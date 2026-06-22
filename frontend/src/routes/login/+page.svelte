<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type InstanceConfig } from '$lib/api';

	let config = $state<InstanceConfig | null>(null);
	let view = $state<'login' | 'register'>('login');

	let username = $state('');
	let password = $state('');
	let inviteCode = $state('');
	let error = $state('');
	let submitting = $state(false);

	const urlError = typeof window !== 'undefined'
		? new URLSearchParams(window.location.search).get('error')
		: null;

	const errorMessages: Record<string, string> = {
		google_disabled: 'Google sign-in is disabled on this instance.',
		google_not_configured: 'Google sign-in is not configured on this server.',
	};

	onMount(async () => {
		try {
			config = await api.getConfig();
		} catch {
			config = { allow_user_space_creation: true, max_video_size_mb: 50, google_auth_enabled: true, local_auth_enabled: false, registration_mode: 'closed' };
		}
		if (urlError && errorMessages[urlError]) {
			error = errorMessages[urlError];
		}
	});

	async function submit() {
		if (submitting) return;
		error = '';
		submitting = true;
		try {
			if (view === 'login') {
				await api.localLogin({ username, password });
			} else {
				await api.register({ username, password, invite_code: inviteCode || undefined });
			}
			window.location.href = '/';
		} catch (e: any) {
			error = e.message ?? 'Something went wrong';
		} finally {
			submitting = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') submit();
	}

	const canRegister = $derived(
		config?.local_auth_enabled && config?.registration_mode !== 'closed'
	);
	const needsInvite = $derived(config?.registration_mode === 'invite');
</script>

<div class="login">
	<div class="card">
		<h1>Conclave</h1>
		<p class="subtitle">A private space for your community</p>

		{#if error}
			<p class="error">{error}</p>
		{/if}

		{#if config === null}
			<p class="loading">Loading…</p>
		{:else}
			{#if config.local_auth_enabled}
				{#if canRegister}
					<div class="tabs">
						<button class="tab" class:active={view === 'login'} onclick={() => { view = 'login'; error = ''; }}>Log in</button>
						<button class="tab" class:active={view === 'register'} onclick={() => { view = 'register'; error = ''; }}>Register</button>
					</div>
				{/if}

				<div class="fields">
					<input
						type="text"
						placeholder="Username"
						bind:value={username}
						onkeydown={onKeydown}
						autocomplete={view === 'login' ? 'username' : 'username'}
						autocapitalize="none"
						spellcheck={false}
					/>
					<input
						type="password"
						placeholder="Password"
						bind:value={password}
						onkeydown={onKeydown}
						autocomplete={view === 'login' ? 'current-password' : 'new-password'}
					/>
					{#if view === 'register' && needsInvite}
						<input
							type="text"
							placeholder="Invite code"
							bind:value={inviteCode}
							onkeydown={onKeydown}
							autocapitalize="none"
							spellcheck={false}
						/>
					{/if}
				</div>

				<button class="submit-btn" onclick={submit} disabled={submitting || !username || !password}>
					{submitting ? '…' : view === 'login' ? 'Log in' : 'Create account'}
				</button>
			{/if}

			{#if config.google_auth_enabled && config.local_auth_enabled}
				<div class="divider"><span>or</span></div>
			{/if}

			{#if config.google_auth_enabled}
				<a href="/api/auth/login" class="google-btn">
					<svg viewBox="0 0 24 24" width="18" height="18">
						<path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
						<path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
						<path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
						<path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
					</svg>
					Continue with Google
				</a>
			{/if}

			{#if !config.google_auth_enabled && !config.local_auth_enabled}
				<p class="no-auth">No authentication methods are enabled on this instance. Contact the administrator.</p>
			{/if}
		{/if}
	</div>
</div>

<style>
	:global(body) { margin: 0; font-family: system-ui, sans-serif; }
	.login {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background: var(--bg, #161617);
	}
	.card {
		background: var(--bg-panel, #1c1c21);
		border: 1px solid var(--border, #494950);
		border-radius: 12px;
		padding: 2.5rem 2rem;
		width: 340px;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	h1 {
		font-size: 1.75rem;
		font-weight: 700;
		color: var(--text, #f0eff4);
		margin: 0 0 0.125rem;
		text-align: center;
	}
	.subtitle {
		color: var(--text-muted, #d3d3de);
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
		text-align: center;
	}
	.error {
		background: rgba(224,69,69,0.12);
		color: #e04545;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
		font-size: 0.82rem;
		margin: 0;
	}
	.loading { color: var(--text-muted, #d3d3de); font-size: 0.875rem; text-align: center; margin: 0; }
	.tabs {
		display: flex;
		background: rgba(255,255,255,0.05);
		border-radius: 8px;
		padding: 3px;
		gap: 3px;
	}
	.tab {
		flex: 1;
		background: none;
		border: none;
		color: var(--text-muted, #d3d3de);
		padding: 0.4rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 500;
		transition: background 0.15s, color 0.15s;
	}
	.tab.active {
		background: var(--bg-input, #26262b);
		color: var(--text, #f0eff4);
	}
	.fields {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	input {
		background: var(--bg-input, #26262b);
		border: 1px solid var(--border, #494950);
		color: var(--text, #f0eff4);
		padding: 0.625rem 0.75rem;
		border-radius: 8px;
		font-size: 0.9rem;
		font-family: inherit;
		outline: none;
		width: 100%;
		box-sizing: border-box;
	}
	input:focus { border-color: var(--accent, #e8541e); }
	.submit-btn {
		background: var(--accent, #e8541e);
		border: none;
		color: white;
		padding: 0.65rem;
		border-radius: 8px;
		font-size: 0.95rem;
		font-weight: 600;
		cursor: pointer;
		width: 100%;
		transition: opacity 0.15s;
	}
	.submit-btn:hover:not(:disabled) { opacity: 0.9; }
	.submit-btn:disabled { opacity: 0.45; cursor: not-allowed; }
	.divider {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		color: var(--text-muted, #d3d3de);
		font-size: 0.78rem;
	}
	.divider::before, .divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border, #494950);
	}
	.google-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.625rem;
		background: white;
		color: #333;
		text-decoration: none;
		padding: 0.625rem;
		border-radius: 8px;
		font-weight: 500;
		font-size: 0.9rem;
		transition: opacity 0.15s;
	}
	.google-btn:hover { opacity: 0.9; }
	.no-auth {
		color: var(--text-muted, #d3d3de);
		font-size: 0.82rem;
		text-align: center;
		margin: 0;
	}
</style>
