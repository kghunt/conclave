<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AdminSettings, type InstanceUser, type RegistrationInvite } from '$lib/api';

	let { onclose }: { onclose: () => void } = $props();

	let settings = $state<AdminSettings>({ message_retention_days: '0', inactive_space_retention_days: '0', google_auth_enabled: 'true', local_auth_enabled: 'true', registration_mode: 'invite' });
	let registrationInvites = $state<RegistrationInvite[]>([]);
	let newInviteMaxUses = $state<number | null>(null);
	let newInviteExpireDays = $state<number | null>(null);
	let creatingInvite = $state(false);
	let deletingInvite = $state<string | null>(null);
	let copiedInvite = $state<string | null>(null);
	let saving = $state(false);
	let running = $state(false);
	let runStatus = $state('');
	let error = $state('');
	let instanceUsers = $state<InstanceUser[]>([]);
	let userSearch = $state('');
	let banningUser = $state<string | null>(null);

	const filteredUsers = $derived(instanceUsers.filter((u) =>
		!userSearch || u.display_name.toLowerCase().includes(userSearch.toLowerCase()) || u.email.toLowerCase().includes(userSearch.toLowerCase())
	));

	type ThemeKey = 'accent' | 'bg' | 'sidebar' | 'panel' | 'input' | 'border' | 'text' | 'text_muted';
	const themeDefaults: Record<ThemeKey, string> = {
		accent: '#e8541e', bg: '#161617', sidebar: '#4b4b4e', panel: '#1c1c21',
		input: '#26262b', border: '#494950', text: '#f0eff4', text_muted: '#d3d3de'
	};
	const themeLabels: Record<ThemeKey, string> = {
		accent: 'Accent', bg: 'App background', sidebar: 'Sidebar', panel: 'Panel / header',
		input: 'Input / secondary', border: 'Borders', text: 'Text', text_muted: 'Muted text'
	};
	const cssVarMap: Record<ThemeKey, string> = {
		accent: '--accent', bg: '--bg', sidebar: '--bg-sidebar', panel: '--bg-panel',
		input: '--bg-input', border: '--border', text: '--text', text_muted: '--text-muted'
	};
	let theme = $state<Record<ThemeKey, string>>({ ...themeDefaults });

	onMount(async () => {
		try {
			const s = await api.getAdminSettings();
			settings = {
				google_auth_enabled: 'true',
				local_auth_enabled: 'true',
				registration_mode: 'invite',
				...s
			};
		} catch (e: any) {
			error = e.message;
		}
		try {
			const t = await fetch('/api/theme').then((r) => r.json()) as Record<string, string>;
			theme = { ...themeDefaults, ...t };
		} catch { /* ignore */ }
		try {
			instanceUsers = await api.listInstanceUsers();
		} catch { /* ignore */ }
		try {
			registrationInvites = await api.listRegistrationInvites();
		} catch { /* ignore */ }
	});

	async function createInvite() {
		if (creatingInvite) return;
		creatingInvite = true;
		try {
			const inv = await api.createRegistrationInvite({
				max_uses: newInviteMaxUses ?? undefined,
				expires_in_days: newInviteExpireDays ?? undefined
			});
			registrationInvites = [inv, ...registrationInvites];
		} catch (e: any) {
			error = e.message;
		} finally {
			creatingInvite = false;
		}
	}

	async function deleteInvite(id: string) {
		deletingInvite = id;
		try {
			await api.deleteRegistrationInvite(id);
			registrationInvites = registrationInvites.filter((i) => i.id !== id);
		} catch { /* ignore */ } finally {
			deletingInvite = null;
		}
	}

	async function copyInviteCode(code: string) {
		await navigator.clipboard.writeText(code);
		copiedInvite = code;
		setTimeout(() => (copiedInvite = null), 2000);
	}

	function onColorInput(key: ThemeKey, value: string) {
		theme = { ...theme, [key]: value };
		document.documentElement.style.setProperty(cssVarMap[key], value);
	}

	function resetColor(key: ThemeKey) {
		theme = { ...theme, [key]: themeDefaults[key] };
		document.documentElement.style.setProperty(cssVarMap[key], themeDefaults[key]);
	}

	async function save() {
		if (saving) return;
		saving = true;
		error = '';
		try {
			const themePayload: Record<string, string> = {};
			for (const k of Object.keys(themeDefaults) as ThemeKey[]) {
				themePayload['theme_' + k] = theme[k] === themeDefaults[k] ? '' : theme[k];
			}
			// number inputs produce JS numbers via bind:value; backend expects map[string]string
			const settingsPayload = Object.fromEntries(
				Object.entries(settings)
					.filter(([, v]) => v !== undefined)
					.map(([k, v]) => [k, String(v)])
			) as AdminSettings;
			await api.updateAdminSettings({ ...settingsPayload, ...themePayload });
			localStorage.setItem('conclave_theme', JSON.stringify(
				Object.fromEntries((Object.keys(themeDefaults) as ThemeKey[])
					.filter((k) => theme[k] !== themeDefaults[k])
					.map((k) => [k, theme[k]]))
			));
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}

	async function runNow() {
		if (running) return;
		running = true;
		runStatus = '';
		try {
			const res = await api.runRetention();
			runStatus = res.status;
		} catch (e: any) {
			error = e.message;
		} finally {
			running = false;
		}
	}

	async function toggleInstanceBan(user: InstanceUser) {
		if (banningUser) return;
		banningUser = user.id;
		try {
			if (user.instance_banned) {
				await api.unbanInstanceUser(user.id);
			} else {
				await api.banInstanceUser(user.id);
			}
			instanceUsers = instanceUsers.map((u) =>
				u.id === user.id ? { ...u, instance_banned: !u.instance_banned } : u
			);
		} catch (e: any) {
			error = e.message;
		} finally {
			banningUser = null;
		}
	}

	function retentionLabel(days: string) {
		const n = parseInt(days);
		if (!n || n === 0) return 'Never (keep forever)';
		if (n === 1) return '1 day';
		if (n < 7) return `${n} days`;
		if (n % 365 === 0) return `${n / 365} year${n / 365 > 1 ? 's' : ''}`;
		if (n % 30 === 0) return `${n / 30} month${n / 30 > 1 ? 's' : ''}`;
		if (n % 7 === 0) return `${n / 7} week${n / 7 > 1 ? 's' : ''}`;
		return `${n} days`;
	}
</script>

<div class="overlay" onclick={onclose}>
	<div class="panel" onclick={(e) => e.stopPropagation()}>
		<div class="panel-header">
			<h2>Instance Admin</h2>
			<button class="close" onclick={onclose}>✕</button>
		</div>

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<section>
			<h3>Authentication</h3>
			<div class="setting">
				<label class="toggle-label">
					<span>Google sign-in</span>
					<input
						type="checkbox"
						checked={settings.google_auth_enabled !== 'false'}
						onchange={(e) => { settings = { ...settings, google_auth_enabled: (e.target as HTMLInputElement).checked ? 'true' : 'false' }; }}
					/>
				</label>
				<p class="hint">Allow users to sign in with their Google account. Requires GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET to be set.</p>
			</div>
			<div class="setting">
				<label class="toggle-label">
					<span>Username &amp; password</span>
					<input
						type="checkbox"
						checked={settings.local_auth_enabled !== 'false'}
						onchange={(e) => { settings = { ...settings, local_auth_enabled: (e.target as HTMLInputElement).checked ? 'true' : 'false' }; }}
					/>
				</label>
				<p class="hint">Allow users to register and log in with a username and password.</p>
			</div>
			{#if settings.local_auth_enabled !== 'false'}
				<div class="setting">
					<label for="reg-mode">Registration</label>
					<select id="reg-mode" bind:value={settings.registration_mode}>
						<option value="invite">Invite only — requires an admin-generated code</option>
						<option value="open">Open — anyone can register (rate limited)</option>
						<option value="closed">Closed — no new registrations</option>
					</select>
					<p class="hint">
						{#if settings.registration_mode === 'invite'}
							New accounts require a registration invite code. Generate codes below.
						{:else if settings.registration_mode === 'open'}
							Anyone can create an account. Rate limited to 5 registrations per IP per hour.
						{:else}
							No new accounts can be created. Existing accounts can still log in.
						{/if}
					</p>
				</div>

				{#if settings.registration_mode === 'invite'}
					<div class="setting">
						<label>Registration invite codes</label>
						<div class="invite-create">
							<div class="invite-create-row">
								<div class="invite-field">
									<span class="invite-field-label">Max uses</span>
									<input type="number" min="1" placeholder="∞" bind:value={newInviteMaxUses} class="invite-num" />
								</div>
								<div class="invite-field">
									<span class="invite-field-label">Expires in</span>
									<div class="invite-expire-row">
										<input type="number" min="1" placeholder="never" bind:value={newInviteExpireDays} class="invite-num" />
										<span class="unit">days</span>
									</div>
								</div>
								<button class="create-invite-btn" onclick={createInvite} disabled={creatingInvite}>
									{creatingInvite ? '…' : 'Generate'}
								</button>
							</div>
						</div>
						{#if registrationInvites.length > 0}
							<div class="invite-list">
								{#each registrationInvites as inv}
									<div class="invite-row">
										<code class="invite-code">{inv.code}</code>
										<span class="invite-meta">
											{inv.use_count}{inv.max_uses != null ? `/${inv.max_uses}` : ''} used
											{#if inv.expires_at}· expires {new Date(inv.expires_at).toLocaleDateString()}{/if}
										</span>
										<button class="copy-invite-btn" onclick={() => copyInviteCode(inv.code)}>
											{copiedInvite === inv.code ? '✓' : 'Copy'}
										</button>
										<button class="del-invite-btn" onclick={() => deleteInvite(inv.id)} disabled={deletingInvite === inv.id}>✕</button>
									</div>
								{/each}
							</div>
						{:else}
							<p class="hint" style="margin-top:0.5rem">No invite codes yet. Generate one above and share it with users you want to invite.</p>
						{/if}
					</div>
				{/if}
			{/if}
		</section>

		<section>
			<h3>General</h3>
			<div class="setting">
				<label class="toggle-label">
					<span>Allow users to create spaces</span>
					<input
						type="checkbox"
						checked={settings.allow_user_space_creation !== 'false'}
						onchange={(e) => {
							settings = { ...settings, allow_user_space_creation: (e.target as HTMLInputElement).checked ? 'true' : 'false' };
						}}
					/>
				</label>
				<p class="hint">When disabled, only the instance admin can create new spaces.</p>
			</div>
		</section>

		<section>
			<h3>Uploads</h3>
			<div class="setting">
				<label for="video-size">Max video upload size</label>
				<div class="setting-row">
					<input
						id="video-size"
						type="number"
						min="0"
						bind:value={settings.max_video_size_mb}
						placeholder="50"
					/>
					<span class="unit">MB</span>
				</div>
				<p class="hint">Maximum size for video file uploads (mp4, webm, mov). Set to 0 to disable video uploads. Default is 50MB.</p>
			</div>
		</section>

		<section>
			<h3>Data Retention</h3>
			<p class="hint">Set to 0 to keep data forever. Cleanup runs daily at startup.</p>

			<div class="setting">
				<label for="msg-retention">
					Message retention
					<span class="current">({retentionLabel(settings.message_retention_days)})</span>
				</label>
				<div class="setting-row">
					<input
						id="msg-retention"
						type="number"
						min="0"
						bind:value={settings.message_retention_days}
						placeholder="0 = forever"
					/>
					<span class="unit">days</span>
				</div>
				<p class="hint">Deletes channel messages and direct messages older than this.</p>
			</div>

			<div class="setting">
				<label for="space-retention">
					Inactive space retention
					<span class="current">({retentionLabel(settings.inactive_space_retention_days)})</span>
				</label>
				<div class="setting-row">
					<input
						id="space-retention"
						type="number"
						min="0"
						bind:value={settings.inactive_space_retention_days}
						placeholder="0 = never"
					/>
					<span class="unit">days</span>
				</div>
				<p class="hint">Deletes spaces with no message activity for this many days.</p>
			</div>

			<div class="retention-run">
				<button class="run-btn" onclick={runNow} disabled={running}>
					{running ? 'Running…' : 'Run cleanup now'}
				</button>
				{#if runStatus}
					<span class="run-status">{runStatus}</span>
				{/if}
			</div>
		</section>

		<section>
			<h3>Users</h3>
			<p class="hint">Manage user access. Banned users cannot log in or use the instance.</p>
			<input
				class="user-search"
				type="search"
				placeholder="Search users…"
				bind:value={userSearch}
			/>
			<div class="user-list">
				{#each filteredUsers as user}
					<div class="user-row" class:banned={user.instance_banned}>
						<div class="user-info">
							<span class="user-name">{user.display_name}</span>
							<span class="user-email">{user.email}</span>
						</div>
						{#if user.instance_banned}
							<span class="ban-badge">Banned</span>
						{/if}
						<button
							class="ban-btn"
							class:unban={user.instance_banned}
							onclick={() => toggleInstanceBan(user)}
							disabled={banningUser === user.id}
						>
							{banningUser === user.id ? '…' : user.instance_banned ? 'Unban' : 'Ban'}
						</button>
					</div>
				{/each}
			</div>
		</section>

		<section>
			<h3>Appearance</h3>
			<p class="hint">Changes apply instantly and are saved instance-wide for all users.</p>
			<div class="color-grid">
				{#each Object.keys(themeDefaults) as k}
					{@const key = k as ThemeKey}
					<div class="color-row">
						<label class="color-label">{themeLabels[key]}</label>
						<div class="color-controls">
							<input
								type="color"
								class="color-swatch"
								value={theme[key]}
								oninput={(e) => onColorInput(key, (e.target as HTMLInputElement).value)}
							/>
							<input
								type="text"
								class="color-hex"
								value={theme[key]}
								maxlength={7}
								onchange={(e) => {
									const v = (e.target as HTMLInputElement).value;
									if (/^#[0-9a-fA-F]{6}$/.test(v)) onColorInput(key, v);
								}}
							/>
							{#if theme[key] !== themeDefaults[key]}
								<button class="reset-btn" onclick={() => resetColor(key)} title="Reset to default">↺</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		</section>

		<div class="actions">
			<button class="cancel-btn" onclick={onclose}>Cancel</button>
			<button class="save-btn" onclick={save} disabled={saving}>
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.75);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}
	.panel {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: calc(100vw - 2rem);
		max-width: 860px;
		height: calc(100vh - 2rem);
		max-height: calc(100vh - 2rem);
		overflow-y: auto;
		display: flex;
		flex-direction: column;
	}
	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
	}
	h2 { color: var(--text); font-size: 1.1rem; }
	.close {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.25rem;
	}
	.close:hover { color: var(--text); }
	section {
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
	}
	h3 {
		font-size: 0.8rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--accent);
		margin-bottom: 0.25rem;
	}
	.setting { margin-top: 1.25rem; }
	label {
		display: block;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--text);
		margin-bottom: 0.375rem;
	}
	.toggle-label {
		display: flex;
		align-items: center;
		justify-content: space-between;
		cursor: pointer;
		margin-bottom: 0;
	}
	.toggle-label input[type="checkbox"] { width: 18px; height: 18px; cursor: pointer; accent-color: var(--accent); }
	.current { color: var(--text-muted); font-weight: 400; font-size: 0.8rem; }
	.setting-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	input[type="number"] {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.9rem;
		width: 120px;
		outline: none;
	}
	input[type="number"]:focus { border-color: var(--accent); }
	.unit { color: var(--text-muted); font-size: 0.875rem; }
	.hint { color: var(--text-muted); font-size: 0.78rem; margin-top: 0.375rem; line-height: 1.4; }
	.user-search {
		display: block;
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.4rem 0.625rem;
		border-radius: 6px;
		font-size: 0.875rem;
		margin-top: 0.75rem;
		box-sizing: border-box;
		outline: none;
	}
	.user-search:focus { border-color: var(--accent); }
	.user-list { max-height: 360px; overflow-y: auto; margin-top: 0.5rem; }
	.user-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0;
		border-bottom: 1px solid rgba(255,255,255,0.04);
	}
	.user-row.banned { opacity: 0.6; }
	.user-info { flex: 1; min-width: 0; }
	.user-name { display: block; font-size: 0.875rem; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.user-email { display: block; font-size: 0.7rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ban-badge {
		font-size: 0.65rem;
		font-weight: 700;
		background: rgba(224,69,69,0.2);
		color: #e04545;
		border-radius: 3px;
		padding: 0.1rem 0.3rem;
		flex-shrink: 0;
	}
	.ban-btn {
		background: rgba(224,69,69,0.15);
		border: 1px solid rgba(224,69,69,0.4);
		color: #e04545;
		padding: 0.25rem 0.6rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.75rem;
		flex-shrink: 0;
	}
	.ban-btn.unban {
		background: rgba(59,165,92,0.15);
		border-color: rgba(59,165,92,0.4);
		color: #3ba55c;
	}
	.ban-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.color-grid { display: flex; flex-direction: column; gap: 0.625rem; margin-top: 0.875rem; }
	.color-row { display: flex; align-items: center; justify-content: space-between; }
	.color-label { font-size: 0.875rem; color: var(--text); }
	.color-controls { display: flex; align-items: center; gap: 0.5rem; }
	.color-swatch {
		width: 32px; height: 32px;
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 2px;
		background: none;
		cursor: pointer;
	}
	.color-swatch::-webkit-color-swatch-wrapper { padding: 0; }
	.color-swatch::-webkit-color-swatch { border: none; border-radius: 2px; }
	.color-hex {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		font-size: 0.8rem;
		font-family: monospace;
		width: 80px;
		outline: none;
	}
	.color-hex:focus { border-color: var(--accent); }
	.reset-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 1rem;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		line-height: 1;
	}
	.reset-btn:hover { color: var(--text); background: rgba(255,255,255,0.07); }
	.error { color: #e04545; font-size: 0.85rem; padding: 0.75rem 1.5rem; background: rgba(224,69,69,0.1); }
	.actions {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		padding: 1rem 1.5rem;
		gap: 0.75rem;
	}
	.retention-run {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}
	.run-btn {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 1rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
	}
	.run-btn:hover:not(:disabled) { background: var(--border); }
	.run-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.run-status { font-size: 0.8rem; color: #44c97d; }
	.cancel-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
	}
	.save-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.5rem 1.25rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
	}
	.save-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	select {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.875rem;
		font-family: inherit;
		outline: none;
		width: 100%;
	}
	select:focus { border-color: var(--accent); }
	.invite-create { margin-top: 0.5rem; }
	.invite-create-row {
		display: flex;
		align-items: flex-end;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.invite-field { display: flex; flex-direction: column; gap: 0.25rem; }
	.invite-field-label { font-size: 0.72rem; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
	.invite-expire-row { display: flex; align-items: center; gap: 0.4rem; }
	.invite-num { width: 80px; }
	.create-invite-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
		white-space: nowrap;
	}
	.create-invite-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.invite-list { margin-top: 0.75rem; display: flex; flex-direction: column; gap: 0.375rem; }
	.invite-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.4rem 0.625rem;
	}
	.invite-code {
		font-family: monospace;
		font-size: 0.85rem;
		color: var(--accent);
		flex-shrink: 0;
	}
	.invite-meta {
		flex: 1;
		font-size: 0.72rem;
		color: var(--text-muted);
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.copy-invite-btn {
		background: rgba(255,255,255,0.08);
		border: none;
		color: var(--text);
		padding: 0.2rem 0.5rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.75rem;
		flex-shrink: 0;
	}
	.copy-invite-btn:hover { background: rgba(255,255,255,0.14); }
	.del-invite-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		font-size: 0.8rem;
		flex-shrink: 0;
	}
	.del-invite-btn:hover { color: #e04545; }
	.del-invite-btn:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
