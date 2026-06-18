<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { currentUser } from '$lib/stores';
	import { socket } from '$lib/socket';

	let { children } = $props();

	async function applyTheme() {
		try {
			const t = await fetch('/api/theme').then((r) => r.json());
			const map: Record<string, string> = {
				accent: '--accent', bg: '--bg', sidebar: '--bg-sidebar',
				panel: '--bg-panel', input: '--bg-input', border: '--border',
				text: '--text', text_muted: '--text-muted'
			};
			const s = document.documentElement.style;
			for (const [k, v] of Object.entries(map)) {
				if ((t as Record<string,string>)[k]) s.setProperty(v, (t as Record<string,string>)[k]);
				else s.removeProperty(v);
			}
			localStorage.setItem('conclave_theme', JSON.stringify(t));
		} catch { /* best-effort */ }
	}

	onMount(async () => {
		applyTheme();
		if ($page.url.pathname === '/login') return;
		try {
			const user = await api.me();
			currentUser.set(user);
			socket.connect();
			const pending = sessionStorage.getItem('pendingRedirect');
			if (pending) {
				sessionStorage.removeItem('pendingRedirect');
				goto(pending);
			}
		} catch {
			if ($page.url.pathname !== '/') {
				sessionStorage.setItem('pendingRedirect', $page.url.pathname + $page.url.search);
			}
			goto('/login');
		}
	});
</script>

{@render children()}
