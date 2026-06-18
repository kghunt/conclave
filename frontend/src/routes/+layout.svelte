<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { currentUser } from '$lib/stores';
	import { socket } from '$lib/socket';

	let { children } = $props();

	onMount(async () => {
		if ($page.url.pathname === '/login') return;
		try {
			const user = await api.me();
			currentUser.set(user);
			socket.connect();
		} catch {
			goto('/login');
		}
	});
</script>

{@render children()}
