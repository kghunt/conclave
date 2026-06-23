<script lang="ts">
	import { api } from '$lib/api';
	import { activeServer, channels, activeChannel, activeDM, currentUser, mentionedChannels, voiceParticipants, voiceSubState } from '$lib/stores';
	import { socket } from '$lib/socket';
	import type { Channel } from '$lib/api';
	import { voiceState, joinVoice, leaveVoice, joinVoiceSub, leaveVoiceSub, createVoiceSub, peerVolumesStore, setPeerVolume } from '$lib/voice';
	import VoicePanel from './VoicePanel.svelte';
	import Avatar from './Avatar.svelte';
	import UserBar from './UserBar.svelte';
	import ChannelPermsModal from './ChannelPermsModal.svelte';

	let permsChannel = $state<Channel | null>(null);

	let showNewChannel = $state(false);
	let newChannelName = $state('');
	let newChannelDesc = $state('');
	let newChannelType = $state<'text' | 'voice' | 'threads'>('text');

	// Load initial voice state when switching servers; listen for live updates via server room
	$effect(() => {
		const server = $activeServer;
		if (!server) return;
		api.getVoiceState(server.id)
			.then((state) => voiceParticipants.set(state))
			.catch(() => {});
		const unsub = socket.on((event) => {
			if (event.type === 'voice.joined') {
				voiceParticipants.update((vp) => {
					const peers = vp[event.payload.channel_id] ?? [];
					if (peers.find((p) => p.user_id === event.payload.user.user_id)) return vp;
					return { ...vp, [event.payload.channel_id]: [...peers, event.payload.user] };
				});
			} else if (event.type === 'voice.left') {
				voiceParticipants.update((vp) => ({
					...vp,
					[event.payload.channel_id]: (vp[event.payload.channel_id] ?? []).filter(
						(p) => p.user_id !== event.payload.user_id
					)
				}));
			} else if (event.type === 'voice.sub.state') {
				voiceSubState.update((s) => ({ ...s, [event.payload.channel_id]: event.payload.subs }));
			} else if (event.type === 'voice.sub.closed') {
				const { channel_id, sub_id } = event.payload;
				voiceSubState.update((s) => ({
					...s,
					[channel_id]: (s[channel_id] ?? []).filter((sub) => sub.id !== sub_id),
				}));
				// If the current user was in this sub, automatically rejoin the main channel.
				if ($voiceState.subChannelId === sub_id && $voiceState.channelId === channel_id) {
					const srvId = $voiceState.serverId ?? '';
					socket.send('voice.sub.leave', { channel_id, sub_id });
					joinVoice(channel_id, srvId);
				}
			} else if (event.type === 'voice.sub.created') {
				// Auto-join the sub the current user just created.
				const { channel_id, sub_id, name } = event.payload;
				if ($voiceState.channelId === channel_id) {
					joinVoiceSub(channel_id, $voiceState.serverId ?? '', sub_id, name);
				}
			}
		});
		return unsub;
	});

	// Keep current user in voiceParticipants while they're in voice
	$effect(() => {
		const chId = $voiceState.channelId;
		const user = $currentUser;
		if (!chId || !user) return;
		voiceParticipants.update((vp) => {
			const peers = vp[chId] ?? [];
			if (peers.find((p) => p.user_id === user.id)) return vp;
			return { ...vp, [chId]: [...peers, { user_id: user.id, display_name: user.display_name, avatar_url: user.avatar_url }] };
		});
	});

	function selectChannel(ch: Channel) {
		activeDM.set(null);
		activeChannel.set(ch);
		mentionedChannels.update(s => { s.delete(ch.id); return new Set(s); });
	}

	async function handleVoiceChannelClick(ch: Channel) {
		if ($voiceState.channelId === ch.id) {
			leaveVoice();
		} else if ($activeServer) {
			await joinVoice(ch.id, $activeServer.id).catch((e) => alert(e.message));
		}
	}

	async function deleteChannel(ch: Channel) {
		if (!$activeServer) return;
		if (!confirm(`Delete #${ch.name}? This cannot be undone.`)) return;
		await api.deleteChannel($activeServer.id, ch.id);
		channels.update((prev) => prev.filter((c) => c.id !== ch.id));
		if ($activeChannel?.id === ch.id) activeChannel.set(null);
	}

	async function createChannel() {
		if (!newChannelName.trim() || !$activeServer) return;
		const ch = await api.createChannel($activeServer.id, { name: newChannelName, description: newChannelDesc.trim(), type: newChannelType });
		channels.update((prev) => [...prev, ch]);
		if (ch.type === 'text' || ch.type === 'threads') selectChannel(ch);
		showNewChannel = false;
		newChannelName = '';
		newChannelDesc = '';
		newChannelType = 'text';
	}

</script>

<aside class="sidebar">
<div class="sidebar-scroll">
	{#if $activeServer}
		<div class="server-header">
			<span>{$activeServer.name}</span>
		</div>

		<div class="section-label">
			<span>Channels</span>
			{#if $activeServer.role === 'owner' || $activeServer.role === 'admin'}
				<button class="add-btn" onclick={() => (showNewChannel = !showNewChannel)}>+</button>
			{/if}
		</div>

		{#if showNewChannel}
			<div class="new-channel">
				<div class="new-channel-type">
					<button
						class="type-btn"
						class:active={newChannelType === 'text'}
						onclick={() => (newChannelType = 'text')}
					># Text</button>
					<button
						class="type-btn"
						class:active={newChannelType === 'threads'}
						onclick={() => (newChannelType = 'threads')}
					>💬 Threads</button>
					<button
						class="type-btn"
						class:active={newChannelType === 'voice'}
						onclick={() => (newChannelType = 'voice')}
					>🔊 Voice</button>
				</div>
				<input
					bind:value={newChannelName}
					placeholder="channel-name"
					autofocus
					onkeydown={(e) => e.key === 'Enter' && createChannel()}
				/>
				<input
					bind:value={newChannelDesc}
					placeholder="Description (optional)"
					class="desc-input"
					onkeydown={(e) => e.key === 'Enter' && createChannel()}
				/>
				<div class="new-channel-actions">
					<button class="cancel-channel-btn" onclick={() => { showNewChannel = false; newChannelName = ''; newChannelDesc = ''; newChannelType = 'text'; }}>Cancel</button>
					<button class="add-channel-btn" onclick={createChannel}>Add</button>
				</div>
			</div>
		{/if}

		{#each $channels.filter((c) => c.type === 'text') as ch}
			{@const isAdmin = $activeServer?.role === 'owner' || $activeServer?.role === 'admin'}
			<div class="channel-row">
				<button
					class="channel-item"
					class:active={$activeChannel?.id === ch.id}
					onclick={() => selectChannel(ch)}
				>
					<span># {ch.name}</span>
					{#if !isAdmin && !ch.can_write}
						<span class="ch-readonly" title="Read-only">🔒</span>
					{:else if $mentionedChannels.has(ch.id)}
						<span class="badge mention-badge">@</span>
					{:else if ch.unread_count > 0}
						<span class="badge">{ch.unread_count}</span>
					{/if}
				</button>
				{#if isAdmin}
					<button class="ch-perms-btn" onclick={() => (permsChannel = ch)} title="Channel permissions">⚙</button>
					<button class="ch-delete-btn" onclick={() => deleteChannel(ch)} title="Delete channel">✕</button>
				{/if}
			</div>
		{/each}

		{#if $channels.some((c) => c.type === 'threads')}
			<div class="section-label" style="margin-top: 0.5rem">
				<span>Thread Channels</span>
			</div>
			{#each $channels.filter((c) => c.type === 'threads') as ch}
				{@const isAdmin = $activeServer?.role === 'owner' || $activeServer?.role === 'admin'}
				<div class="channel-row">
					<button
						class="channel-item"
						class:active={$activeChannel?.id === ch.id}
						onclick={() => selectChannel(ch)}
					>
						<span>💬 {ch.name}</span>
						{#if !isAdmin && !ch.can_write}
							<span class="ch-readonly" title="Read-only">🔒</span>
						{:else if ch.unread_count > 0}
							<span class="badge">{ch.unread_count}</span>
						{/if}
					</button>
					{#if isAdmin}
						<button class="ch-perms-btn" onclick={() => (permsChannel = ch)} title="Channel permissions">⚙</button>
						<button class="ch-delete-btn" onclick={() => deleteChannel(ch)} title="Delete channel">✕</button>
					{/if}
				</div>
			{/each}
		{/if}

		{#if $channels.some((c) => c.type === 'voice')}
			<div class="section-label" style="margin-top: 0.5rem">
				<span>Voice Channels</span>
			</div>
			{#each $channels.filter((c) => c.type === 'voice') as ch}
				{@const peers = $voiceParticipants[ch.id] ?? []}
				{@const inThisChannel = $voiceState.channelId === ch.id}
				{@const subs = $voiceSubState[ch.id] ?? []}
				<div class="channel-row">
					<button
						class="channel-item voice-channel-item"
						class:active={inThisChannel && !$voiceState.subChannelId}
						onclick={() => handleVoiceChannelClick(ch)}
					>
						<span class="voice-ch-icon">🔊</span>
						<span class="voice-ch-name">{ch.name}</span>
						{#if peers.length > 0}
							<span class="voice-count">{peers.length}</span>
						{/if}
					</button>
					{#if inThisChannel && !$voiceState.subChannelId}
						<button
							class="sub-create-btn"
							title="Create sub-channel"
							onclick={() => {
								const name = prompt('Sub-channel name (e.g. Team 1):');
								if (name?.trim()) createVoiceSub(ch.id, name.trim());
							}}
						>+</button>
					{/if}
					{#if $activeServer?.role === 'owner' || $activeServer?.role === 'admin'}
						<button class="ch-delete-btn" onclick={() => deleteChannel(ch)} title="Delete channel">✕</button>
					{/if}
				</div>

				{#if peers.length > 0}
					<div class="voice-participant-list">
						{#each peers as peer}
							{@const speaking = $voiceState.speakingUsers.has(peer.user_id)}
							{@const isSelf = peer.user_id === $currentUser?.id}
							<div class="voice-participant" class:speaking>
								<img
									src={peer.avatar_url || '/default-avatar.png'}
									alt={peer.display_name}
									class="vp-avatar"
									class:speaking
								/>
								<span class="vp-name" class:speaking>{peer.display_name}</span>
								{#if inThisChannel && !$voiceState.subChannelId && !isSelf}
									<input
										class="vp-vol"
										type="range" min="0" max="2" step="0.05"
										value={$peerVolumesStore[peer.user_id] ?? 1}
										oninput={(e) => setPeerVolume(peer.user_id, +(e.target as HTMLInputElement).value)}
										title="Volume"
									/>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

				{#each subs as sub}
					{@const inThisSub = $voiceState.subChannelId === sub.id}
					<div class="sub-channel-row">
						<button
							class="sub-channel-item"
							class:active={inThisSub}
							onclick={() => {
								if (inThisSub) {
									leaveVoiceSub(ch.id, sub.id, $voiceState.serverId ?? '');
								} else {
									joinVoiceSub(ch.id, $voiceState.serverId ?? $activeServer?.id ?? '', sub.id, sub.name);
								}
							}}
						>
							<span class="sub-icon">╰</span>
							<span class="sub-name">{sub.name}</span>
							<span class="voice-count">{sub.participants.length}</span>
						</button>
						{#if inThisSub || $activeServer?.role === 'owner' || $activeServer?.role === 'admin' || sub.creator_id === $currentUser?.id}
							<button
								class="ch-delete-btn"
								title="Close sub-channel"
								onclick={() => socket.send('voice.sub.close', { channel_id: ch.id, sub_id: sub.id })}
							>✕</button>
						{/if}
					</div>
					{#if sub.participants.length > 0}
						<div class="voice-participant-list sub-participant-list">
							{#each sub.participants as peer}
								{@const speaking = inThisSub && $voiceState.speakingUsers.has(peer.user_id)}
								<div class="voice-participant" class:speaking>
									<img
										src={peer.avatar_url || '/default-avatar.png'}
										alt={peer.display_name}
										class="vp-avatar"
										class:speaking
									/>
									<span class="vp-name" class:speaking>{peer.display_name}</span>
									{#if inThisSub && peer.user_id !== $currentUser?.id}
										<input
											class="vp-vol"
											type="range" min="0" max="2" step="0.05"
											value={$peerVolumesStore[peer.user_id] ?? 1}
											oninput={(e) => setPeerVolume(peer.user_id, +(e.target as HTMLInputElement).value)}
											title="Volume"
										/>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				{/each}
			{/each}
		{/if}
	{/if}

</div><!-- end sidebar-scroll -->

<VoicePanel />
<UserBar />
</aside>

{#if permsChannel && $activeServer}
	<ChannelPermsModal
		serverId={$activeServer.id}
		channel={permsChannel}
		onclose={() => (permsChannel = null)}
	/>
{/if}

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
		display: flex;
		align-items: center;
		justify-content: space-between;
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
		letter-spacing: 0.05em;
	}
	.add-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 1rem;
		padding: 0 0.25rem;
	}
	.add-btn:hover { color: var(--text); }
	.new-channel {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		padding: 0.4rem 0.75rem;
	}
	.new-channel input {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		font-size: 0.85rem;
		outline: none;
	}
	.new-channel input:focus { border-color: var(--accent); }
	.new-channel .desc-input {
		font-size: 0.78rem;
		opacity: 0.8;
	}
	.new-channel-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.4rem;
	}
	.cancel-channel-btn {
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 0.25rem 0.6rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.82rem;
	}
	.cancel-channel-btn:hover { color: var(--text); }
	.add-channel-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 0.25rem 0.65rem;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.82rem;
		font-weight: 600;
	}
	.add-channel-btn:hover { filter: brightness(1.1); }
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
		.server-header { height: 52px; font-size: 1rem; }
	}
	.channel-item:hover, .channel-item.active {
		background: rgba(255,255,255,0.07);
		color: var(--text);
	}
	.channel-item.has-unread { color: var(--text); font-weight: 600; }
	.badge {
		margin-left: auto;
		background: #e04545;
		color: white;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 8px;
		padding: 0.1rem 0.4rem;
	}
	.mention-badge {
		background: var(--accent);
	}
	.channel-row {
		display: flex;
		align-items: center;
		margin: 0 0.25rem;
	}
	.channel-row .channel-item {
		margin: 0;
		width: auto;
		flex: 1;
		min-width: 0;
	}
	.ch-delete-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.7rem;
		padding: 0.2rem 0.3rem;
		border-radius: 3px;
		opacity: 0;
		flex-shrink: 0;
		transition: opacity 0.1s;
	}
	.channel-row:hover .ch-delete-btn { opacity: 1; }
	.ch-delete-btn:hover { color: #e04545; background: rgba(224,69,69,0.1); }
	@media (max-width: 767px) { .ch-delete-btn { opacity: 1; } }
	.new-channel-type {
		display: flex;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 5px;
		padding: 2px;
		gap: 2px;
	}
	.type-btn {
		flex: 1;
		background: none;
		border: none;
		color: var(--text-muted);
		border-radius: 3px;
		cursor: pointer;
		padding: 0.25rem 0.3rem;
		font-size: 0.75rem;
		line-height: 1.2;
		white-space: nowrap;
		transition: background 0.1s, color 0.1s;
	}
	.type-btn:hover:not(.active) { background: rgba(255,255,255,0.06); color: var(--text); }
	.type-btn.active {
		background: var(--accent);
		color: white;
	}
	.voice-channel-item {
		position: relative;
	}
	.voice-ch-icon {
		font-size: 0.75rem;
		flex-shrink: 0;
	}
	.voice-ch-name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.voice-count {
		font-size: 0.7rem;
		font-weight: 700;
		color: #43b581;
		background: rgba(67,181,129,0.15);
		border-radius: 8px;
		padding: 0.1rem 0.35rem;
		flex-shrink: 0;
	}
	.voice-participant-list {
		padding: 2px 0.75rem 2px 2rem;
	}
	.voice-participant {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 2px 0;
	}
	.vp-avatar {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
		transition: box-shadow 0.1s;
	}
	.vp-avatar.speaking {
		box-shadow: 0 0 0 2px #43b581;
	}
	.vp-name {
		font-size: 0.78rem;
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
		min-width: 0;
		transition: color 0.1s;
	}
	.vp-name.speaking {
		color: var(--text);
	}
	.vp-vol {
		width: 44px;
		flex-shrink: 0;
		height: 3px;
		accent-color: #43b581;
		cursor: pointer;
	}
	.sub-create-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 1rem;
		line-height: 1;
		padding: 0 4px;
		border-radius: 4px;
		flex-shrink: 0;
	}
	.sub-create-btn:hover { color: var(--text); background: rgba(255,255,255,0.06); }
	.sub-channel-row {
		display: flex;
		align-items: center;
		padding: 0 0.5rem 0 0.75rem;
		gap: 2px;
	}
	.sub-channel-item {
		display: flex;
		align-items: center;
		gap: 6px;
		flex: 1;
		min-width: 0;
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 2px 4px;
		border-radius: 4px;
		font-size: 0.82rem;
		text-align: left;
	}
	.sub-channel-item:hover { background: rgba(255,255,255,0.04); color: var(--text); }
	.sub-channel-item.active { color: #43b581; }
	.sub-icon {
		font-size: 0.75rem;
		opacity: 0.5;
		flex-shrink: 0;
	}
	.sub-name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.sub-participant-list {
		padding-left: 3rem;
	}
	.user-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: 1;
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.25rem;
		border-radius: 4px;
		min-width: 0;
	}
	.user-info:hover { background: rgba(255,255,255,0.07); }
	.ch-perms-btn {
		display: none;
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.8rem;
		padding: 2px 4px;
		border-radius: 3px;
		line-height: 1;
	}
	.ch-perms-btn:hover { color: var(--text); background: rgba(255,255,255,0.1); }
	.channel-row:hover .ch-perms-btn { display: block; }
	.ch-readonly {
		font-size: 0.7rem;
		color: var(--text-muted);
		margin-left: auto;
		flex-shrink: 0;
		opacity: 0.7;
	}
</style>
