<script lang="ts">
	import { defaultAvatarUrl } from '$lib/avatar';

	let {
		url = '',
		name = '?',
		userId = '',
		size = 32
	}: { url?: string; name?: string; userId?: string; size?: number } = $props();

	const src = $derived(url || defaultAvatarUrl(userId || name, name));
</script>

<img {src} alt={name} width={size} height={size} style="width:{size}px;height:{size}px;border-radius:50%;object-fit:cover;flex-shrink:0" />
