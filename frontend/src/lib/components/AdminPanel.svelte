<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AdminSettings } from '$lib/api';

	let { onclose }: { onclose: () => void } = $props();

	let settings = $state<AdminSettings>({ message_retention_days: '0', inactive_space_retention_days: '0' });
	let saving = $state(false);
	let running = $state(false);
	let runStatus = $state('');
	let error = $state('');

	type ThemeKey = 'accent' | 'bg' | 'sidebar' | 'panel' | 'input' | 'border' | 'text' | 'text_muted';
	const themeDefaults: Record<ThemeKey, string> = {
		accent: '#e8541e', bg: '#111113', sidebar: '#19191d', panel: '#1c1c21',
		input: '#26262b', border: '#2e2e38', text: '#f0eff4', text_muted: '#8b8b99'
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
			settings = await api.getAdminSettings();
		} catch (e: any) {
			error = e.message;
		}
		try {
			const t = await fetch('/api/theme').then((r) => r.json()) as Record<string, string>;
			theme = { ...themeDefaults, ...t };
		} catch { /* ignore */ }
	});

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
			await api.updateAdminSettings({ ...settings, ...themePayload });
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
			<div class="left">
				<button class="run-btn" onclick={runNow} disabled={running}>
					{running ? 'Running…' : 'Run cleanup now'}
				</button>
				{#if runStatus}
					<span class="run-status">{runStatus}</span>
				{/if}
			</div>
			<div class="right">
				<button class="cancel-btn" onclick={onclose}>Cancel</button>
				<button class="save-btn" onclick={save} disabled={saving}>
					{saving ? 'Saving…' : 'Save'}
				</button>
			</div>
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
		width: 480px;
		max-height: 80vh;
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
		justify-content: space-between;
		padding: 1rem 1.5rem;
		gap: 0.75rem;
	}
	.left, .right { display: flex; align-items: center; gap: 0.75rem; }
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
</style>
