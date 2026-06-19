<script lang="ts">
	import { api, type User } from '$lib/api';
	import { activeChannel, activeDM, dmConversations, currentUser, friends, friendRequests, friendRequestsSent } from '$lib/stores';
	import Avatar from './Avatar.svelte';
	import UserBar from './UserBar.svelte';

	let showAddFriend = $state(false);
	let friendSearch = $state('');
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

	async function messageFriend(userId: string) {
		const conv = await api.getOrCreateDM(userId);
		dmConversations.update((prev) => {
			if (prev.find((c) => c.id === conv.id)) return prev;
			return [conv, ...prev];
		});
		activeChannel.set(null);
		activeDM.set(conv);
	}
</script>

<aside class="sidebar">
<div class="sidebar-scroll">

	<div class="server-header">Messages &amp; Friends</div>

	<div class="section-label">Direct Messages</div>
	{#each $dmConversations as conv}
		<button
			class="channel-item"
			class:active={$activeDM?.id === conv.id}
			class:has-unread={conv.unread_count > 0 && $activeDM?.id !== conv.id}
			onclick={() => { activeChannel.set(null); activeDM.set(conv); }}
		>
			<Avatar url={conv.other_user.avatar_url} name={conv.other_user.display_name} userId={conv.other_user.id} size={20} showPresence />
			<span class="dm-name">{conv.other_user.display_name}</span>
			{#if conv.unread_count > 0 && $activeDM?.id !== conv.id}
				<span class="badge">{conv.unread_count}</span>
			{/if}
		</button>
	{/each}
	{#if $dmConversations.length === 0}
		<p class="empty-hint">No conversations yet. Message a friend to get started.</p>
	{/if}

	<div class="section-divider"></div>

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

	{#each $friends as f}
		<div class="friend-item">
			<Avatar url={f.user.avatar_url} name={f.user.display_name} userId={f.user.id} size={28} />
			<span class="friend-name">{f.user.display_name}</span>
			<button class="msg-btn" onclick={() => messageFriend(f.user.id)} title="Message">
				<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
			</button>
		</div>
	{/each}

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
	.section-divider {
		height: 1px;
		background: var(--border);
		margin: 0.75rem 0.75rem 0;
	}
	.channel-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: none;
		border: none;
		color: var(--text-muted);
		padding: 0.375rem 0.75rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
		margin: 0 0.25rem;
		width: calc(100% - 0.5rem);
		font-size: 0.9rem;
	}
	@media (max-width: 767px) {
		.channel-item { padding: 0.65rem 0.75rem; font-size: 1rem; }
	}
	.channel-item:hover, .channel-item.active { background: rgba(255,255,255,0.07); color: var(--text); }
	.channel-item.has-unread { color: var(--text); font-weight: 600; }
	.dm-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.badge {
		margin-left: auto;
		background: #e04545;
		color: white;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.4rem;
		flex-shrink: 0;
	}
	.empty-hint {
		font-size: 0.78rem;
		color: var(--text-muted);
		padding: 0.25rem 0.75rem 0.5rem;
		margin: 0;
		line-height: 1.4;
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
	.friend-item:hover { background: rgba(255,255,255,0.05); }
	.friend-name { flex: 1; font-size: 0.875rem; color: var(--text); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
	.msg-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		display: flex;
		align-items: center;
		opacity: 0;
		transition: opacity 0.1s;
		flex-shrink: 0;
	}
	.friend-item:hover .msg-btn { opacity: 1; }
	.msg-btn:hover { color: var(--text); background: rgba(255,255,255,0.1); }
	@media (max-width: 767px) { .msg-btn { opacity: 1; } }
</style>
