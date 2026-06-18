<script lang="ts">
	import { voiceState, leaveVoice, toggleMute } from '$lib/voice';
	import { channels, currentUser, voiceParticipants } from '$lib/stores';
	import { get } from 'svelte/store';

	const state = voiceState;

	function channelName(channelId: string | null): string {
		if (!channelId) return '';
		return get(channels).find((c) => c.id === channelId)?.name ?? '…';
	}
</script>

{#if $state.channelId}
	{@const chId = $state.channelId}
	{@const allPeers = $voiceParticipants[chId] ?? []}
	<div class="voice-panel">
		<div class="voice-header">
			<div class="voice-title">
				<span class="voice-dot" class:connecting={$state.connecting}></span>
				<span class="voice-name">{channelName(chId)}</span>
				{#if $state.connecting}<span class="voice-status">Connecting…</span>{/if}
			</div>
			<div class="voice-actions">
				<button
					class="vbtn"
					class:muted={$state.muted}
					onclick={toggleMute}
					title={$state.muted ? 'Unmute' : 'Mute'}
				>
					{#if $state.muted}
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
		{#if allPeers.length > 0}
			<div class="voice-peers">
				{#each allPeers as peer}
					<div class="voice-peer">
						<img src={peer.avatar_url || '/default-avatar.png'} alt="" class="peer-avatar" />
						<span class="peer-name">
							{peer.display_name}{peer.user_id === $currentUser?.id ? ' (you)' : ''}
						</span>
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
		padding: 8px 10px 6px;
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
	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.4; }
	}
	.voice-name {
		font-size: 0.8rem;
		font-weight: 600;
		color: #43b581;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.voice-status {
		font-size: 0.7rem;
		color: #faa61a;
		flex-shrink: 0;
	}
	.voice-actions {
		display: flex;
		gap: 3px;
		flex-shrink: 0;
	}
	.vbtn {
		background: rgba(255, 255, 255, 0.06);
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
	.vbtn:hover {
		background: rgba(255, 255, 255, 0.12);
		color: var(--text);
	}
	.leave-btn:hover {
		background: rgba(240, 71, 71, 0.25);
		color: #f04747;
	}
	.vbtn.muted {
		background: rgba(240, 71, 71, 0.15);
		color: #f04747;
	}
	.voice-peers {
		margin-top: 5px;
		padding-left: 14px;
		display: flex;
		flex-direction: column;
		gap: 3px;
	}
	.voice-peer {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.peer-avatar {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
	}
	.peer-name {
		font-size: 0.78rem;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
</style>
