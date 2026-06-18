<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AdminSettings } from '$lib/api';

	let { onclose }: { onclose: () => void } = $props();

	let settings = $state<AdminSettings>({ message_retention_days: '0', inactive_space_retention_days: '0' });
	let saving = $state(false);
	let running = $state(false);
	let runStatus = $state('');
	let error = $state('');

	onMount(async () => {
		try {
			settings = await api.getAdminSettings();
		} catch (e: any) {
			error = e.message;
		}
	});

	async function save() {
		if (saving) return;
		saving = true;
		error = '';
		try {
			await api.updateAdminSettings(settings);
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
		background: #1c1c21;
		border: 1px solid #2e2e38;
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
		border-bottom: 1px solid #2e2e38;
	}
	h2 { color: #f0eff4; font-size: 1.1rem; }
	.close {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.25rem;
	}
	.close:hover { color: #f0eff4; }
	section {
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid #2e2e38;
	}
	h3 {
		font-size: 0.8rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: #e8541e;
		margin-bottom: 0.25rem;
	}
	.setting { margin-top: 1.25rem; }
	label {
		display: block;
		font-size: 0.875rem;
		font-weight: 600;
		color: #f0eff4;
		margin-bottom: 0.375rem;
	}
	.current { color: #8b8b99; font-weight: 400; font-size: 0.8rem; }
	.setting-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	input[type="number"] {
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.9rem;
		width: 120px;
		outline: none;
	}
	input[type="number"]:focus { border-color: #e8541e; }
	.unit { color: #8b8b99; font-size: 0.875rem; }
	.hint { color: #8b8b99; font-size: 0.78rem; margin-top: 0.375rem; line-height: 1.4; }
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
		background: #26262b;
		border: 1px solid #2e2e38;
		color: #f0eff4;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		cursor: pointer;
		font-size: 0.875rem;
	}
	.run-btn:hover:not(:disabled) { background: #2e2e38; }
	.run-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.run-status { font-size: 0.8rem; color: #44c97d; }
	.cancel-btn {
		background: none;
		border: none;
		color: #8b8b99;
		cursor: pointer;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
	}
	.save-btn {
		background: #e8541e;
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
