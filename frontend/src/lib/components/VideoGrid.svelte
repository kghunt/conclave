<script lang="ts">
	import { voiceState, localVideoStore, remoteVideoStore, toggleCamera } from '$lib/voice';

	let minimized = $state(false);

	const hasVideo = $derived(
		$voiceState.cameraOn || Object.keys($remoteVideoStore).length > 0
	);

	function attachStream(node: HTMLVideoElement, stream: MediaStream | null) {
		node.srcObject = stream;
		return {
			update(s: MediaStream | null) { node.srcObject = s; },
			destroy() { node.srcObject = null; },
		};
	}

	function peerName(userId: string): string {
		return $voiceState.peers.find((p) => p.user_id === userId)?.display_name ?? userId;
	}
</script>

{#if hasVideo}
	<div class="video-grid-wrap" class:minimized>
		<div class="vg-header">
			<span class="vg-title">Video</span>
			<div class="vg-header-actions">
				{#if $voiceState.cameraOn}
					<button class="vg-btn cam-off" onclick={toggleCamera} title="Turn off camera">
						<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M21 6.5l-4-4-9.5 9.5-2 4.5 4.5-2L21 6.5zM3.5 18.5c-.3.3-.5.7-.5 1s.2.7.5 1 .7.5 1 .5.7-.2 1-.5l-2-2z"/><path d="M21 6.5l-4-4L3 17l-1 4 4-1L21 6.5z" opacity=".3"/></svg>
					</button>
				{/if}
				<button class="vg-btn" onclick={() => (minimized = !minimized)} title={minimized ? 'Expand' : 'Minimise'}>
					{#if minimized}
						<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z"/></svg>
					{:else}
						<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z"/></svg>
					{/if}
				</button>
			</div>
		</div>

		{#if !minimized}
			<div class="vg-tiles">
				{#if $voiceState.cameraOn && $localVideoStore}
					<div class="vg-tile self">
						<video use:attachStream={$localVideoStore} autoplay playsinline muted></video>
						<span class="vg-label">You</span>
					</div>
				{/if}
				{#each Object.entries($remoteVideoStore) as [userId, stream] (userId)}
					<div class="vg-tile">
						<video use:attachStream={stream} autoplay playsinline></video>
						<span class="vg-label">{peerName(userId)}</span>
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/if}

<style>
	.video-grid-wrap {
		position: fixed;
		bottom: 120px;
		right: 16px;
		z-index: 200;
		background: #111318;
		border: 1px solid rgba(255,255,255,0.12);
		border-radius: 10px;
		box-shadow: 0 8px 32px rgba(0,0,0,0.6);
		min-width: 200px;
		max-width: 640px;
		overflow: hidden;
	}
	.video-grid-wrap.minimized {
		min-width: 0;
	}
	.vg-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 6px 8px;
		background: rgba(255,255,255,0.04);
		border-bottom: 1px solid rgba(255,255,255,0.07);
		gap: 6px;
	}
	.vg-title {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--text-muted);
	}
	.vg-header-actions {
		display: flex;
		gap: 4px;
	}
	.vg-btn {
		background: rgba(255,255,255,0.07);
		border: none;
		border-radius: 4px;
		color: var(--text-muted);
		cursor: pointer;
		width: 22px;
		height: 22px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.15s, color 0.15s;
	}
	.vg-btn:hover { background: rgba(255,255,255,0.14); color: var(--text); }
	.vg-btn.cam-off:hover { background: rgba(240,71,71,0.2); color: #f04747; }

	.vg-tiles {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		padding: 6px;
	}
	.vg-tile {
		position: relative;
		border-radius: 6px;
		overflow: hidden;
		background: #1a1f2e;
		flex: 1 1 180px;
		max-width: 300px;
		aspect-ratio: 16/9;
	}
	.vg-tile video {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}
	.vg-tile.self video {
		transform: scaleX(-1);
	}
	.vg-label {
		position: absolute;
		bottom: 5px;
		left: 7px;
		font-size: 0.7rem;
		font-weight: 600;
		color: #fff;
		text-shadow: 0 1px 4px rgba(0,0,0,0.8);
		pointer-events: none;
	}
</style>
