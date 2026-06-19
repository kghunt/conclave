<script lang="ts">
	import { callState, acceptCall, declineCall, cancelCall } from '$lib/voice';
	import Avatar from './Avatar.svelte';
</script>

{#if $callState.status !== 'idle'}
	<div class="call-overlay">
		<div class="call-card">
			{#if $callState.peer}
				<Avatar
					url={$callState.peer.avatarUrl}
					name={$callState.peer.displayName}
					userId={$callState.peer.userId}
					size={48}
				/>
			{/if}
			<div class="call-info">
				<span class="call-name">{$callState.peer?.displayName ?? ''}</span>
				<span class="call-status">
					{$callState.status === 'ringing_in' ? 'Incoming call…' : 'Calling…'}
				</span>
			</div>
			<div class="call-actions">
				{#if $callState.status === 'ringing_in'}
					<button class="call-btn accept" onclick={acceptCall} title="Accept">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
							<path d="M6.62 10.79c1.44 2.83 3.76 5.14 6.59 6.59l2.2-2.2c.27-.27.67-.36 1.02-.24 1.12.37 2.33.57 3.57.57.55 0 1 .45 1 1V20c0 .55-.45 1-1 1-9.39 0-17-7.61-17-17 0-.55.45-1 1-1h3.5c.55 0 1 .45 1 1 0 1.25.2 2.45.57 3.57.11.35.03.74-.25 1.02l-2.2 2.2z"/>
						</svg>
					</button>
					<button class="call-btn decline" onclick={declineCall} title="Decline">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
							<path d="M20 5.41L18.59 4 12 10.59 5.41 4 4 5.41 10.59 12 4 18.59 5.41 20 12 13.41 18.59 20 20 18.59 13.41 12z"/>
						</svg>
					</button>
				{:else}
					<button class="call-btn decline" onclick={cancelCall} title="Cancel">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
							<path d="M20 5.41L18.59 4 12 10.59 5.41 4 4 5.41 10.59 12 4 18.59 5.41 20 12 13.41 18.59 20 20 18.59 13.41 12z"/>
						</svg>
					</button>
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.call-overlay {
		position: fixed;
		bottom: 80px;
		right: 20px;
		z-index: 1000;
		animation: slide-in 0.2s ease;
	}
	@keyframes slide-in {
		from { transform: translateY(20px); opacity: 0; }
		to   { transform: translateY(0);   opacity: 1; }
	}
	.call-card {
		display: flex;
		align-items: center;
		gap: 12px;
		background: var(--bg-panel, #1e2130);
		border: 1px solid rgba(255,255,255,0.1);
		border-radius: 12px;
		padding: 12px 16px;
		box-shadow: 0 8px 32px rgba(0,0,0,0.5);
		min-width: 240px;
	}
	.call-info {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.call-name {
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--text);
	}
	.call-status {
		font-size: 0.75rem;
		color: var(--text-muted);
	}
	.call-actions {
		display: flex;
		gap: 8px;
	}
	.call-btn {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		border: none;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: filter 0.15s;
	}
	.call-btn:hover { filter: brightness(1.2); }
	.call-btn.accept { background: #43b581; color: #fff; }
	.call-btn.decline { background: #f04747; color: #fff; }
</style>
