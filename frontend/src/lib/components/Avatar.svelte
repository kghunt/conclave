<script lang="ts">
	import { defaultAvatarUrl } from '$lib/avatar';
	import { presenceMap } from '$lib/stores';

	let {
		url = '',
		name = '?',
		userId = '',
		size = 32,
		showPresence = false
	}: { url?: string; name?: string; userId?: string; size?: number; showPresence?: boolean } = $props();

	const src = $derived(url || defaultAvatarUrl(userId || name, name));
	const status = $derived(showPresence && userId ? ($presenceMap[userId] ?? 'offline') : null);

	const dotSize = $derived(Math.max(8, Math.round(size * 0.3)));
</script>

<div class="avatar-wrap" style="width:{size}px;height:{size}px;flex-shrink:0">
	<img {src} alt={name} width={size} height={size} />
	{#if status}
		<span class="presence-dot {status}" style="width:{dotSize}px;height:{dotSize}px;border-radius:50%;bottom:{-Math.floor(dotSize*0.15)}px;right:{-Math.floor(dotSize*0.15)}px"></span>
	{/if}
</div>

<style>
	.avatar-wrap {
		position: relative;
		display: inline-block;
	}
	.avatar-wrap img {
		width: 100%;
		height: 100%;
		border-radius: 50%;
		object-fit: cover;
		display: block;
	}
	.presence-dot {
		position: absolute;
		box-sizing: border-box;
		border: 2px solid var(--bg-sidebar, #19191d);
	}
	.presence-dot.online  { background: #3ba55c; }
	.presence-dot.away    { background: #faa81a; }
	.presence-dot.offline { background: transparent; border-color: #72767d; }
</style>
