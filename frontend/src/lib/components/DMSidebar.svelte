<script lang="ts">
	import { api, type User } from '$lib/api';
	import { activeChannel, activeDM, dmConversations, currentUser, friends, friendRequests, friendRequestsSent } from '$lib/stores';
	import { callFriend } from '$lib/voice';
	import Avatar from './Avatar.svelte';
	import UserBar from './UserBar.svelte';

	let showAddFriend = $state(false);
	let friendSearch = $state('');
	let filterQuery = $state('');
	let searchResults = $state<User[]>([]);
	let searchDebounce: ReturnType<typeof setTimeout>;
	let pendingRequests = $state<Record<string, 'sending' | 'sent' | 'error'>>({});

	$effect(() => {
		const q = friendSearch;
		clearTimeout(searchDebounce);
		if (q.length < 2) { searchResults = []; return; }
		searchDebounce = setTimeout(async () => {
			searchResults = await api.searchUsers(q).catch(() => []);
		}, 300);
	});

	async function sendRequest(userId: string) {
		pendingRequests = { ...pendingRequests, [userId]: 'sending' };
		try {
			const result = await api.sendFriendRequest(userId);
			pendingRequests = { ...pendingRequests, [userId]: 'sent' };
			if (result.status === 'accepted') {
				const [fr, reqs] = await Promise.all([api.listFriends(), api.listFriendRequests()]);
				friends.set(fr ?? []);
				friendRequests.set(reqs ?? []);
			} else {
				const user = searchResults.find((u) => u.id === userId);
				if (user) friendRequestsSent.update((s) => [{ user, since: new Date().toISOString() }, ...s]);
			}
		} catch (e: any) {
			pendingRequests = { ...pendingRequests, [userId]: e?.message?.includes('already') ? 'sent' : 'error' };
		}
	}

	async function acceptRequest(userId: string) {
		await api.acceptFriendRequest(userId);
		friendRequests.update((rs) => rs.filter((r) => r.user.id !== userId));
		const fr = await api.listFriends();
		friends.set(fr ?? []);
	}

	async function declineRequest(userId: string) {
		await api.removeFriend(userId);
		friendRequests.update((rs) => rs.filter((r) => r.user.id !== userId));
	}

	async function cancelRequest(userId: string) {
		await api.removeFriend(userId);
		friendRequestsSent.update((s) => s.filter((r) => r.user.id !== userId));
	}

	async function openDM(userId: string) {
		const existing = $dmConversations.find((c) => c.other_user.id === userId);
		if (existing) {
			activeChannel.set(null);
			activeDM.set(existing);
			return;
		}
		const conv = await api.getOrCreateDM(userId);
		dmConversations.update((prev) => {
			if (prev.find((c) => c.id === conv.id)) return prev;
			return [conv, ...prev];
		});
		activeChannel.set(null);
		activeDM.set(conv);
	}

	const sortedFriends = $derived((() => {
		const convByUser = new Map($dmConversations.map((c) => [c.other_user.id, c]));
		const filtered = filterQuery
			? $friends.filter((f) => f.user.display_name.toLowerCase().includes(filterQuery.toLowerCase()))
			: $friends;
		return [...filtered].sort((a, b) => {
			const ca = convByUser.get(a.user.id);
			const cb = convByUser.get(b.user.id);
			if (ca && cb) return new Date(cb.last_message_at).getTime() - new Date(ca.last_message_at).getTime();
			if (ca) return -1;
			if (cb) return 1;
			return a.user.display_name.localeCompare(b.user.display_name);
		});
	})());
</script>

<aside class="sidebar">
<div class="sidebar-scroll">

	<div class="server-header">Messages &amp; Friends</div>

	<div class="section-label">
		<span>
			Friends
			{#if $friendRequests.length > 0}
				<span class="req-badge">{$friendRequests.length}</span>
			{/if}
		</span>
		<button class="add-btn" onclick={() => { showAddFriend = !showAddFriend; friendSearch = ''; searchResults = []; }} title="Add friend">+</button>
	</div>

	{#if showAddFriend}
		<div class="friend-search">
			<input bind:value={friendSearch} placeholder="Search by name…" autofocus />
			{#if searchResults.length > 0}
				<div class="search-results">
					{#each searchResults as u}
						{@const reqState = pendingRequests[u.id]}
						<div class="search-result">
							<Avatar url={u.avatar_url} name={u.display_name} userId={u.id} size={24} />
							<span class="result-name">{u.display_name}</span>
							{#if reqState === 'sent'}
								<span class="req-sent">✓</span>
							{:else}
								<button class="req-btn" onclick={() => sendRequest(u.id)} disabled={reqState === 'sending'}>
									{reqState === 'sending' ? '…' : '+'}
								</button>
							{/if}
						</div>
					{/each}
				</div>
			{:else if friendSearch.length >= 2}
				<p class="no-results">No users found</p>
			{/if}
		</div>
	{/if}

	{#each $friendRequests as req}
		<div class="friend-request">
			<Avatar url={req.user.avatar_url} name={req.user.display_name} userId={req.user.id} size={28} />
			<span class="friend-name">{req.user.display_name}</span>
			<button class="accept-btn" onclick={() => acceptRequest(req.user.id)} title="Accept">✓</button>
			<button class="decline-btn" onclick={() => declineRequest(req.user.id)} title="Decline">✕</button>
		</div>
	{/each}

	{#each $friendRequestsSent as req}
		<div class="friend-item">
			<Avatar url={req.user.avatar_url} name={req.user.display_name} userId={req.user.id} size={28} />
			<span class="friend-name">{req.user.display_name}</span>
			<span class="pending-badge">Pending</span>
			<button class="decline-btn" onclick={() => cancelRequest(req.user.id)} title="Cancel">✕</button>
		</div>
	{/each}

	{#if $friends.length > 0}
		<div class="filter-wrap">
			<input class="filter-input" bind:value={filterQuery} placeholder="Filter friends…" />
		</div>
	{/if}

	{#each sortedFriends as f}
		{@const conv = $dmConversations.find((c) => c.other_user.id === f.user.id)}
		{@const isActive = $activeDM?.other_user.id === f.user.id}
		{@const unread = (conv?.unread_count ?? 0) > 0 && !isActive}
		<div
			class="friend-item clickable"
			class:active={isActive}
			class:has-unread={unread}
			role="button"
			tabindex="0"
			onclick={() => openDM(f.user.id)}
			onkeydown={(e) => e.key === 'Enter' && openDM(f.user.id)}
		>
			<Avatar url={f.user.avatar_url} name={f.user.display_name} userId={f.user.id} size={28} showPresence />
			<span class="friend-name">{f.user.display_name}</span>
			{#if unread}
				<span class="badge">{conv!.unread_count}</span>
			{/if}
			<button
				class="call-btn"
				onclick={(e) => { e.stopPropagation(); callFriend(f.user.id, f.user.display_name, f.user.avatar_url ?? ''); }}
				title="Call"
			>
				<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M6.62 10.79c1.44 2.83 3.76 5.14 6.59 6.59l2.2-2.2c.27-.27.67-.36 1.02-.24 1.12.37 2.33.57 3.57.57.55 0 1 .45 1 1V20c0 .55-.45 1-1 1-9.39 0-17-7.61-17-17 0-.55.45-1 1-1h3.5c.55 0 1 .45 1 1 0 1.25.2 2.45.57 3.57.11.35.03.74-.25 1.02l-2.2 2.2z"/></svg>
			</button>
		</div>
	{/each}

	{#if $friends.length === 0 && $friendRequests.length === 0}
		<p class="empty-hint">Add friends to start chatting.</p>
	{/if}

</div><!-- end sidebar-scroll -->
<UserBar />
</aside>

<style>
	.sidebar {
		width: 240px;
		background: var(--bg-sidebar);
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		overflow: hidden;
	}
	.sidebar-scroll {
		flex: 1;
		overflow-y: auto;
		min-height: 0;
	}
	@media (max-width: 767px) {
		.sidebar { width: 100%; flex: 1; }
	}
	.server-header {
		padding: 0.875rem 1rem;
		font-weight: 700;
		border-bottom: 1px solid #0e0e10;
		font-size: 0.95rem;
	}
	.section-label {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 0.75rem 0.25rem;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--text-muted);
		letter-spacing: 0.04em;
	}
	.add-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 1rem;
		line-height: 1;
		padding: 0 0.1rem;
		border-radius: 3px;
	}
	.add-btn:hover { color: var(--text); }
	.req-badge {
		background: #e04545;
		color: white;
		font-size: 0.65rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.35rem;
		margin-left: 0.25rem;
		vertical-align: middle;
	}
	.filter-wrap {
		padding: 0.25rem 0.75rem 0.125rem;
	}
	.filter-input {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		font-size: 0.82rem;
		width: 100%;
		outline: none;
		font-family: inherit;
	}
	.filter-input::placeholder { color: var(--text-muted); }
	.friend-search {
		padding: 0.25rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.friend-search input {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.35rem 0.5rem;
		border-radius: 4px;
		font-size: 0.85rem;
		width: 100%;
		outline: none;
	}
	.search-results {
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 4px;
		overflow: hidden;
	}
	.search-result {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.5rem;
	}
	.search-result:hover { background: rgba(255,255,255,0.05); }
	.result-name { flex: 1; font-size: 0.85rem; color: var(--text); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.req-btn {
		background: var(--accent);
		border: none;
		color: white;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		cursor: pointer;
		font-size: 0.85rem;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}
	.req-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.req-sent { color: #44c97d; font-size: 0.85rem; flex-shrink: 0; }
	.no-results { font-size: 0.8rem; color: var(--text-muted); padding: 0.25rem 0.5rem; margin: 0; }
	.friend-request {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0.75rem;
		background: rgba(232,84,30,0.06);
		margin: 0 0.25rem;
		border-radius: 4px;
	}
	.friend-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0.75rem;
		margin: 0 0.25rem;
		border-radius: 4px;
	}
	.friend-item.clickable {
		cursor: pointer;
		user-select: none;
	}
	.friend-item.clickable:hover { background: rgba(255,255,255,0.07); }
	.friend-item.active { background: rgba(255,255,255,0.1); }
	.friend-item.active .friend-name { color: var(--text); }
	.friend-item.has-unread .friend-name { color: var(--text); font-weight: 600; }
	.friend-name { flex: 1; font-size: 0.875rem; color: var(--text-muted); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.badge {
		background: #e04545;
		color: white;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.4rem;
		flex-shrink: 0;
	}
	.pending-badge {
		font-size: 0.65rem;
		font-weight: 600;
		color: var(--text-muted);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.1rem 0.35rem;
		flex-shrink: 0;
	}
	.accept-btn {
		background: none;
		border: none;
		color: #44c97d;
		cursor: pointer;
		font-size: 0.9rem;
		padding: 0.15rem 0.3rem;
		border-radius: 3px;
		flex-shrink: 0;
	}
	.accept-btn:hover { background: rgba(68,201,125,0.15); }
	.decline-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.8rem;
		padding: 0.15rem 0.3rem;
		border-radius: 3px;
		flex-shrink: 0;
	}
	.decline-btn:hover { color: #e04545; background: rgba(224,69,69,0.1); }
	.call-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		display: flex;
		align-items: center;
		opacity: 0;
		transition: opacity 0.1s, color 0.1s;
		flex-shrink: 0;
	}
	.friend-item:hover .call-btn { opacity: 1; }
	.call-btn:hover { color: #43b581; background: rgba(67,181,129,0.1); }
	.empty-hint {
		font-size: 0.78rem;
		color: var(--text-muted);
		padding: 0.25rem 0.75rem 0.5rem;
		margin: 0;
		line-height: 1.4;
	}
	@media (max-width: 767px) { .call-btn { opacity: 1; } }
</style>
