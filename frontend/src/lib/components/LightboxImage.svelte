<script lang="ts">
	interface Props { src: string; onclose: () => void; }
	let { src, onclose }: Props = $props();

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
<div class="overlay" onclick={onclose}>
	<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
	<img {src} alt="" class="lightbox-img" onclick={(e) => e.stopPropagation()} />
	<button class="close-btn" onclick={onclose} aria-label="Close">✕</button>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 9999;
		background: rgba(0, 0, 0, 0.92);
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: zoom-out;
	}
	.lightbox-img {
		max-width: 95vw;
		max-height: 95vh;
		object-fit: contain;
		border-radius: 4px;
		cursor: default;
		user-select: none;
	}
	.close-btn {
		position: absolute;
		top: 1rem;
		right: 1rem;
		background: rgba(0, 0, 0, 0.5);
		border: 1px solid rgba(255, 255, 255, 0.2);
		color: white;
		width: 36px;
		height: 36px;
		border-radius: 50%;
		font-size: 1rem;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		line-height: 1;
	}
	.close-btn:hover { background: rgba(255, 255, 255, 0.15); }
</style>
