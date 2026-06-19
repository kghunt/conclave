<script lang="ts">
	import {
		voiceState, leaveVoice, toggleMute, setMicGain, setPeerVolume, peerVolumesStore,
		setEchoCancellation, setNoiseSuppression, setAutoGainControl
	} from '$lib/voice';
	import { channels, currentUser, voiceParticipants } from '$lib/stores';
	import { get } from 'svelte/store';

	let showSettings = $state(false);

	function channelName(channelId: string | null): string {
		if (!channelId) return '';
		return get(channels).find((c) => c.id === channelId)?.name ?? '…';
	}

	function onMicGain(e: Event) {
		setMicGain(+(e.target as HTMLInputElement).value);
	}

	function onPeerVolume(userId: string, e: Event) {
		setPeerVolume(userId, +(e.target as HTMLInputElement).value);
	}

</script>

{#if $voiceState.channelId}
	{@const chId = $voiceState.channelId}
	{@const allPeers = $voiceParticipants[chId] ?? []}
	<div class="voice-panel">
		<!-- Header row -->
		<div class="voice-header">
			<div class="voice-title">
				<span class="voice-dot" class:connecting={$voiceState.connecting}></span>
				<span class="voice-name">{channelName(chId)}</span>
				{#if $voiceState.connecting}<span class="voice-status">Connecting…</span>{/if}
			</div>
			<div class="voice-actions">
				<button
					class="vbtn"
					class:settings-active={showSettings}
					onclick={() => (showSettings = !showSettings)}
					title="Audio settings"
				>
					<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M19.14 12.94c.04-.3.06-.61.06-.94s-.02-.64-.07-.94l2.03-1.58a.49.49 0 0 0 .12-.61l-1.92-3.32a.49.49 0 0 0-.59-.22l-2.39.96a7.03 7.03 0 0 0-1.62-.94l-.36-2.54a.484.484 0 0 0-.48-.41h-3.84a.484.484 0 0 0-.47.41l-.36 2.54a7.03 7.03 0 0 0-1.62.94l-2.39-.96a.477.477 0 0 0-.59.22L2.74 8.87a.47.47 0 0 0 .12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 0 0-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.27.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54a7.03 7.03 0 0 0 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32a.47.47 0 0 0-.12-.61l-2.01-1.58zM12 15.6a3.6 3.6 0 1 1 0-7.2 3.6 3.6 0 0 1 0 7.2z"/></svg>
				</button>
				<button
					class="vbtn"
					class:muted={$voiceState.muted}
					onclick={toggleMute}
					title={$voiceState.muted ? 'Unmute' : 'Mute mic'}
				>
					{#if $voiceState.muted}
						<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M19 11h-1.7c0 .74-.16 1.43-.43 2.05l1.23 1.23c.56-.98.9-2.09.9-3.28zm-4.02.17c0-.06.02-.11.02-.17V5c0-1.66-1.34-3-3-3S9 3.34 9 5v.18l5.98 5.99zM4.27 3L3 4.27l6.01 6.01V11c0 1.66 1.33 3 2.99 3 .22 0 .44-.03.65-.08l1.66 1.66c-.71.33-1.5.52-2.31.52-2.76 0-5.3-2.1-5.3-5.1H5c0 3.41 2.72 6.23 6 6.72V21h2v-3.28c.91-.13 1.77-.45 2.54-.9L19.73 21 21 19.73 4.27 3z"/></svg>
					{:else}
						<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm-1-9c0-.55.45-1 1-1s1 .45 1 1v6c0 .55-.45 1-1 1s-1-.45-1-1V5zm6 6c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/></svg>
					{/if}
				</button>
				<button class="vbtn leave-btn" onclick={leaveVoice} title="Leave voice">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M10.9 15.6l-1.42 1.42C10.99 18.3 12.42 19 14 19h1v-2h-1c-.93 0-1.78-.4-2.1-1.4zM20 8h-1c0-2.21-1.79-4-4-4H9v2h6c1.1 0 2 .9 2 2H15v2h2v2h-2v2h2v2h-2v2h5V8zm-10 0H4c-1.1 0-2 .9-2 2v4c0 1.1.9 2 2 2h6v-4H8v-2h2V8z"/></svg>
				</button>
			</div>
		</div>

		<!-- Mic gain -->
		<div class="mic-row">
			<svg class="row-icon" width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm-1-9c0-.55.45-1 1-1s1 .45 1 1v6c0 .55-.45 1-1 1s-1-.45-1-1V5zm6 6c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/></svg>
			<input class="vol-slider" type="range" min="0" max="2" step="0.05"
				value={$voiceState.micGain} oninput={onMicGain} title="Microphone input volume" />
			<span class="vol-pct">{Math.round($voiceState.micGain * 100)}%</span>
		</div>

		<!-- Audio quality settings (collapsible) -->
		{#if showSettings}
			<div class="settings-panel">
				<div class="settings-row">
					<span class="settings-label">Echo cancellation</span>
					<button
						class="toggle-btn"
						class:on={$voiceState.echoCancellation}
						onclick={() => setEchoCancellation(!$voiceState.echoCancellation)}
					>{$voiceState.echoCancellation ? 'On' : 'Off'}</button>
				</div>
				<div class="settings-row">
					<span class="settings-label">Noise suppression</span>
					<button
						class="toggle-btn"
						class:on={$voiceState.noiseSuppression}
						onclick={() => setNoiseSuppression(!$voiceState.noiseSuppression)}
					>{$voiceState.noiseSuppression ? 'On' : 'Off'}</button>
				</div>
				<div class="settings-row">
					<span class="settings-label">Auto gain</span>
					<button
						class="toggle-btn"
						class:on={$voiceState.autoGainControl}
						onclick={() => setAutoGainControl(!$voiceState.autoGainControl)}
					>{$voiceState.autoGainControl ? 'On' : 'Off'}</button>
				</div>
				<p class="settings-note">EC/NS/AGC changes restart the mic briefly. Handled by your browser.</p>
			</div>
		{/if}

		<!-- Participant list -->
		{#if allPeers.length > 0}
			<div class="voice-peers">
				{#each allPeers as peer}
					{@const isSelf = peer.user_id === $currentUser?.id}
					{@const speaking = $voiceState.speakingUsers.has(peer.user_id)}
					<div class="voice-peer" class:speaking>
						<span class="speak-dot" class:active={speaking}></span>
						<img src={peer.avatar_url || '/default-avatar.png'} alt="" class="peer-avatar" class:speaking />
						<span class="peer-name">{peer.display_name}{isSelf ? ' (you)' : ''}</span>
						{#if !isSelf}
							<input
								class="vol-slider peer-vol"
								type="range" min="0" max="2" step="0.05"
								value={$peerVolumesStore[peer.user_id] ?? 1}
								oninput={(e) => onPeerVolume(peer.user_id, e)}
								title="Participant volume"
							/>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/if}

<style>
	.voice-panel {
		background: #1a1f2e;
		border-top: 1px solid #2a3040;
		padding: 8px 10px 8px;
		flex-shrink: 0;
	}
	.voice-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
	}
	.voice-title {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
	}
	.voice-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #43b581;
		flex-shrink: 0;
	}
	.voice-dot.connecting {
		background: #faa61a;
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
	.voice-name {
		font-size: 0.8rem;
		font-weight: 600;
		color: #43b581;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.voice-status { font-size: 0.7rem; color: #faa61a; flex-shrink: 0; }
	.voice-actions { display: flex; gap: 3px; flex-shrink: 0; }
	.vbtn {
		background: rgba(255,255,255,0.06);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		color: var(--text-muted);
		width: 26px;
		height: 26px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.15s, color 0.15s;
	}
	.vbtn:hover { background: rgba(255,255,255,0.12); color: var(--text); }
	.leave-btn:hover { background: rgba(240,71,71,0.25); color: #f04747; }
	.vbtn.muted { background: rgba(240,71,71,0.15); color: #f04747; }
	.vbtn.settings-active { background: rgba(255,255,255,0.12); color: var(--text); }

	.mic-row, .vad-row {
		display: flex;
		align-items: center;
		gap: 5px;
		margin-top: 6px;
	}
	.row-icon { color: var(--text-muted); flex-shrink: 0; }
	.vol-pct { font-size: 0.68rem; color: var(--text-muted); width: 30px; text-align: right; flex-shrink: 0; }
	.vol-slider { flex: 1; height: 3px; accent-color: #43b581; cursor: pointer; min-width: 0; }

	.settings-panel {
		background: rgba(0,0,0,0.2);
		border-radius: 4px;
		padding: 6px 8px;
		margin-top: 6px;
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.settings-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
	}
	.settings-label {
		font-size: 0.72rem;
		color: var(--text-muted);
		flex: 1;
	}
	.toggle-btn {
		background: rgba(255,255,255,0.08);
		border: none;
		color: var(--text-muted);
		font-size: 0.68rem;
		font-weight: 600;
		padding: 2px 8px;
		border-radius: 3px;
		cursor: pointer;
		min-width: 32px;
		text-align: center;
		flex-shrink: 0;
	}
	.toggle-btn:hover { background: rgba(255,255,255,0.14); }
	.toggle-btn.on { background: rgba(67,181,129,0.2); color: #43b581; }
	.settings-note {
		font-size: 0.65rem;
		color: var(--text-muted);
		opacity: 0.6;
		margin: 2px 0 0;
		line-height: 1.4;
	}
	.voice-peers { margin-top: 6px; display: flex; flex-direction: column; gap: 4px; }
	.voice-peer { display: flex; align-items: center; gap: 5px; padding: 2px 0; border-radius: 4px; }
	.speak-dot {
		width: 6px; height: 6px; border-radius: 50%;
		background: #444c5c; flex-shrink: 0; transition: background 0.1s;
	}
	.speak-dot.active { background: #43b581; }
	.peer-avatar { width: 16px; height: 16px; border-radius: 50%; object-fit: cover; flex-shrink: 0; transition: box-shadow 0.1s; }
	.peer-avatar.speaking { box-shadow: 0 0 0 2px #43b581; }
	.peer-name { font-size: 0.78rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; min-width: 0; }
	.voice-peer.speaking .peer-name { color: var(--text); }
	.peer-vol { width: 48px; flex: none; }
</style>
