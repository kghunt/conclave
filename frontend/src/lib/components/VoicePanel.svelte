<script lang="ts">
	import { voiceState, leaveVoice, toggleMute } from '$lib/voice';
	import { channels } from '$lib/stores';
	import { get } from 'svelte/store';

	const state = voiceState;

	function channelName(channelId: string | null): string {
		if (!channelId) return '';
		return get(channels).find((c) => c.id === channelId)?.name ?? '...';
	}
</script>

{#if $state.channelId}
	<div class="voice-panel">
		<div class="voice-info">
			<span class="voice-icon">🔊</span>
			<div class="voice-text">
				<span class="voice-label">{$state.connecting ? 'Connecting…' : 'Voice Connected'}</span>
				<span class="voice-channel">{channelName($state.channelId)}</span>
			</div>
		</div>
		<div class="voice-actions">
			<button
				class="voice-btn mute-btn"
				class:muted={$state.muted}
				onclick={toggleMute}
				title={$state.muted ? 'Unmute' : 'Mute'}
			>
				{$state.muted ? '🔇' : '🎤'}
			</button>
			<button class="voice-btn leave-btn" onclick={leaveVoice} title="Leave voice">✕</button>
		</div>
		{#if $state.peers.length > 0}
			<div class="voice-peers">
				{#each $state.peers as peer}
					<img
						src={peer.avatar_url || '/default-avatar.png'}
						alt={peer.display_name}
						title={peer.display_name}
						class="peer-avatar"
					/>
				{/each}
			</div>
		{/if}
	</div>
{/if}

<style>
	.voice-panel {
		background: #1a1f2e;
		border-top: 1px solid #2a3040;
		padding: 8px 12px;
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.voice-info {
		display: flex;
		align-items: center;
		gap: 6px;
		flex: 1;
		min-width: 0;
	}
	.voice-icon {
		font-size: 14px;
		flex-shrink: 0;
		color: #43b581;
	}
	.voice-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.voice-label {
		font-size: 11px;
		font-weight: 600;
		color: #43b581;
		line-height: 1.2;
	}
	.voice-channel {
		font-size: 11px;
		color: #8899aa;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.voice-actions {
		display: flex;
		gap: 4px;
		flex-shrink: 0;
	}
	.voice-btn {
		background: rgba(255, 255, 255, 0.06);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 14px;
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.15s;
	}
	.voice-btn:hover {
		background: rgba(255, 255, 255, 0.12);
	}
	.leave-btn:hover {
		background: rgba(240, 71, 71, 0.3);
	}
	.mute-btn.muted {
		background: rgba(240, 71, 71, 0.2);
	}
	.voice-peers {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
		width: 100%;
		padding-top: 4px;
	}
	.peer-avatar {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		object-fit: cover;
	}
</style>
