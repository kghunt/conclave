<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Message, type DirectMessage, type MessageReply, type Thread, type Reaction } from '$lib/api';
	import { socket } from '$lib/socket';
	import { currentUser, servers, activeServer, channels, activeChannel, dmConversations, activeDM, showProfileModal, friends, friendRequests, friendRequestsSent, instanceConfig, serverMembers, mentionedChannels, presenceMap, gameStatus, notifPrefs, serverUnread, joinRequestPending, homeMode, pendingJoinRequests } from '$lib/stores';
	import { playMessageSound, playMentionSound, playDMSound } from '$lib/sounds';
	import { handleIncomingCall, handleCallAccepted, handleCallDeclined, handleCallEnded, handleCallCancelled } from '$lib/voice';
	import CallNotification from '$lib/components/CallNotification.svelte';
	import type { ServerMember } from '$lib/api';
	import ServerList from '$lib/components/ServerList.svelte';
	import ChannelSidebar from '$lib/components/ChannelSidebar.svelte';
	import DMSidebar from '$lib/components/DMSidebar.svelte';
	import MessageFeed from '$lib/components/MessageFeed.svelte';
	import MemberList from '$lib/components/MemberList.svelte';
	import ProfileModal from '$lib/components/ProfileModal.svelte';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';
	import ThreadChannel from '$lib/components/ThreadChannel.svelte';
	import ThreadView from '$lib/components/ThreadView.svelte';

	let messages: Message[] = $state([]);
	let dmMessages: DirectMessage[] = $state([]);
	let activeThread = $state<Thread | null>(null);
	let input = $state('');
	let showMembers = $state(false);
	let isMobile = $state(false);
	let editingDesc = $state(false);
	let descInput = $state('');

	onMount(() => {
		// Prevent the browser from suspending this tab. The never-resolving promise
		// holds the Web Lock for the lifetime of the page.
		navigator.locks?.request('conclave-active', { mode: 'shared' }, () => new Promise(() => {}));

		const mq = window.matchMedia('(max-width: 767px)');
		isMobile = mq.matches;
		const handler = (e: MediaQueryListEvent) => { isMobile = e.matches; };
		mq.addEventListener('change', handler);

		// Android PWA back: pop state goes back to channel list instead of exiting
		function handlePopState() {
			if (isMobile) mobileBack();
		}
		window.addEventListener('popstate', handlePopState);

		// Read saved navigation state synchronously before any $effect can overwrite it
		const savedMode = localStorage.getItem('lastMode');
		const savedDMId = localStorage.getItem('lastDMId');
		const savedServerId = localStorage.getItem('lastServerId');

		(async () => {
			const [s, convs, fr, reqs, sent, cfg] = await Promise.all([
				api.listServers(),
				api.listConversations(),
				api.listFriends(),
				api.listFriendRequests(),
				api.listFriendRequestsSent(),
				api.getConfig().catch(() => ({ allow_user_space_creation: true, max_video_size_mb: 50 }))
			]);
			servers.set(s ?? []);
			dmConversations.set(convs ?? []);
			friends.set(fr ?? []);
			friendRequests.set(reqs ?? []);
			friendRequestsSent.set(sent ?? []);
			instanceConfig.set(cfg);

			// Restore last view: home (DMs) or server+channel
			if (savedMode === 'home') {
				homeMode.set(true);
				if (savedDMId) {
					const dm = (convs ?? []).find((c) => c.id === savedDMId);
					if (dm) activeDM.set(dm);
				}
			} else {
				if (savedServerId) {
					const match = (s ?? []).find((sv) => sv.id === savedServerId);
					if (match) activeServer.set(match);
					else if ((s ?? []).length > 0) activeServer.set(s[0]);
				}
			}
		})();

		return () => {
			mq.removeEventListener('change', handler);
			window.removeEventListener('popstate', handlePopState);
		};
	});

	// Keep exactly one "inChat" history entry on mobile so the back button/gesture
	// returns to the channel list. Push only when entering chat from the list;
	// replace when switching between channels/DMs so the stack doesn't grow.
	$effect(() => {
		if (!isMobile) return;
		if ($activeChannel || $activeDM) {
			if (history.state?.inChat) {
				history.replaceState({ inChat: true }, '');
			} else {
				history.pushState({ inChat: true }, '');
			}
		}
	});

	// On mobile: going back from chat clears the active channel/DM
	function mobileBack() {
		activeChannel.set(null);
		activeDM.set(null);
	}

	// Persist navigation state so refresh restores the same view
	$effect(() => { localStorage.setItem('lastMode', $homeMode ? 'home' : 'server'); });
	$effect(() => { if ($activeDM) localStorage.setItem('lastDMId', $activeDM.id); });
	$effect(() => {
		if ($activeChannel && $activeServer)
			localStorage.setItem('lastChannelId:' + $activeServer.id, $activeChannel.id);
	});

	// Load channels + members + presence when active server changes
	$effect(() => {
		const srv = $activeServer;
		if (!srv) return;
		const id = srv.id;
		localStorage.setItem('lastServerId', id);
		api.listChannels(id).then((ch) => {
			channels.set(ch ?? []);
			// On mobile, show the channel list so the user can choose; on desktop restore last or use first
			if (ch?.length > 0 && !isMobile) {
				const lastChannelId = localStorage.getItem('lastChannelId:' + id);
				const lastCh = lastChannelId ? ch.find((c) => c.id === lastChannelId) : null;
				activeChannel.set(lastCh ?? ch[0]);
			}
		});
		api.getMembers(id).then((ms) => serverMembers.set(ms ?? []));
		api.getPresence(id).then((p) => presenceMap.update(m => ({ ...m, ...p })));

		const isAdmin = srv.role === 'owner' || srv.role === 'admin';
		if (isAdmin) {
			api.listJoinRequests(id).then((reqs) => pendingJoinRequests.set(reqs ?? [])).catch(() => {});
		} else {
			pendingJoinRequests.set([]);
		}

		// Subscribe to server room for presence.update events
		const room = 'server:' + id;
		socket.subscribe(room);
		const unsub = socket.on((event) => {
			if (event.type === 'presence.update') {
				presenceMap.update(m => ({ ...m, [event.payload.user_id]: event.payload.status }));
			} else if (event.type === 'presence.game') {
				gameStatus.update(m => {
					const next = { ...m };
					if (event.payload.game) next[event.payload.user_id] = event.payload.game;
					else delete next[event.payload.user_id];
					return next;
				});
			}
		});
		return () => { unsub(); socket.unsubscribe(room); };
	});

	// Send presence events when page visibility changes; reconnect socket if
	// the browser suspended the tab while it was hidden.
	$effect(() => {
		if (!$currentUser) return;
		function onVisibility() {
			socket.send('presence', { status: document.hidden ? 'away' : 'online' });
			if (!document.hidden) socket.connect();
		}
		document.addEventListener('visibilitychange', onVisibility);
		return () => document.removeEventListener('visibilitychange', onVisibility);
	});

	// Load messages and subscribe to WS when active channel changes
	$effect(() => {
		const ch = $activeChannel;
		const srv = $activeServer;
		if (!ch || !srv) return;
		// Thread channels and voice channels manage their own subscriptions
		if (ch.type === 'threads' || ch.type === 'voice') return;

		messages = [];
		const channelId = ch.id;
		const serverId = srv.id;
		const room = 'channel:' + channelId;

		api.listMessages(serverId, channelId).then((m) => (messages = m ?? []));
		api.markRead(serverId, channelId);
		notifyRead();
		channels.update((cs) => cs.map((c) => c.id === channelId ? { ...c, unread_count: 0 } : c));
		socket.subscribe(room);

		const unsub = socket.on((event) => {
			if (event.type === 'message.new' && event.payload.channel_id === channelId) {
				messages = [...messages, event.payload];
				api.markRead(serverId, channelId);
				notifyRead();
				channels.update((cs) => cs.map((c) => c.id === channelId ? { ...c, unread_count: 0 } : c));
				if ($notifPrefs.messageSound && event.payload.author?.id !== $currentUser?.id) {
					playMessageSound();
				}
			}
			if (event.type === 'message.edit' && event.payload.channel_id === channelId) {
				messages = messages.map((m) => m.id === event.payload.id ? event.payload : m);
			}
			if (event.type === 'message.delete' && event.payload.channel_id === channelId) {
				messages = messages.filter((m) => m.id !== event.payload.id);
			}
			if (event.type === 'reaction.toggle' && event.payload.channel_id === channelId) {
				const { message_id, emoji, user_id, action } = event.payload;
				const isMine = user_id === $currentUser?.id;
				messages = messages.map((m) => {
					if (m.id !== message_id) return m;
					const msg = m as Message;
					const reactions = [...(msg.reactions ?? [])];
					const idx = reactions.findIndex((rx: Reaction) => rx.emoji === emoji);
					if (action === 'add') {
						if (idx >= 0) {
							reactions[idx] = { ...reactions[idx], count: reactions[idx].count + 1, mine: reactions[idx].mine || isMine };
						} else {
							reactions.push({ emoji, count: 1, mine: isMine });
						}
					} else {
						if (idx >= 0) {
							const newCount = reactions[idx].count - 1;
							if (newCount <= 0) reactions.splice(idx, 1);
							else reactions[idx] = { ...reactions[idx], count: newCount, mine: isMine ? false : reactions[idx].mine };
						}
					}
					return { ...msg, reactions };
				});
			}
		});

		return () => {
			unsub();
			socket.unsubscribe(room);
		};
	});

	// Load DM messages and subscribe to WS when active DM changes
	$effect(() => {
		const dm = $activeDM;
		if (!dm) return;

		dmMessages = [];
		const convId = dm.id;
		const room = 'dm:' + convId;

		api.listDMMessages(convId).then((m) => (dmMessages = m ?? []));
		// Mark read and clear unread count in the store
		api.markDMRead(convId);
		notifyRead();
		dmConversations.update((cs) => cs.map((c) => c.id === convId ? { ...c, unread_count: 0 } : c));
		socket.subscribe(room);

		const unsub = socket.on((event) => {
			if (event.type === 'dm.new' && event.payload.conversation_id === convId) {
				dmMessages = [...dmMessages, event.payload];
				if ($notifPrefs.dmSound && event.payload.sender?.id !== $currentUser?.id) {
					playDMSound();
				}
				dmConversations.update((cs) => {
					const updated = cs.map((c) => c.id === convId ? { ...c, last_message_at: event.payload.created_at } : c);
					return [...updated].sort((a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime());
				});
			}
			if (event.type === 'dm.delete' && event.payload.conversation_id === convId) {
				dmMessages = dmMessages.filter((m) => m.id !== event.payload.id);
			}
			if (event.type === 'dm.edit' && event.payload.conversation_id === convId) {
				dmMessages = dmMessages.map((m) => m.id === event.payload.id ? { ...event.payload, reactions: m.reactions } : m);
			}
			if (event.type === 'dm.reaction.toggle' && event.payload.conversation_id === convId) {
				const { message_id, emoji, user_id, action } = event.payload;
				const isMine = user_id === $currentUser?.id;
				dmMessages = dmMessages.map((m) => {
					if (m.id !== message_id) return m;
					const reactions = [...(m.reactions ?? [])];
					const idx = reactions.findIndex((r) => r.emoji === emoji);
					if (action === 'add') {
						if (idx >= 0) reactions[idx] = { ...reactions[idx], count: reactions[idx].count + 1, mine: reactions[idx].mine || isMine };
						else reactions.push({ emoji, count: 1, mine: isMine });
					} else {
						if (idx >= 0) {
							const next = { ...reactions[idx], count: reactions[idx].count - 1, mine: isMine ? false : reactions[idx].mine };
							if (next.count <= 0) reactions.splice(idx, 1);
							else reactions[idx] = next;
						}
					}
					return { ...m, reactions };
				});
			}
		});

		return () => {
			unsub();
			socket.unsubscribe(room);
		};
	});

	let uploading = $state(false);
	let showEmoji = $state(false);
	let fileInput: HTMLInputElement;
	let textarea: HTMLTextAreaElement;
	let replyingTo = $state<Message | null>(null);

	// Typing indicator
	let typers = $state<Record<string, string>>({}); // userId → displayName
	const typerTimeouts: Record<string, ReturnType<typeof setTimeout>> = {};
	let lastTypingSent = 0;

	$effect(() => {
		const unsub = socket.on((event) => {
			if (event.type !== 'typing') return;
			const { user_id, display_name, room } = event.payload;
			const currentRoom = $activeChannel ? 'channel:' + $activeChannel.id
			                  : $activeDM      ? 'dm:'      + $activeDM.id
			                  : null;
			if (room !== currentRoom) return;
			typers = { ...typers, [user_id]: display_name };
			clearTimeout(typerTimeouts[user_id]);
			typerTimeouts[user_id] = setTimeout(() => {
				const { [user_id]: _, ...rest } = typers;
				typers = rest;
			}, 3000);
		});
		return () => unsub();
	});

	// Clear typers, reply state, active thread, and desc edit when switching channels or DMs
	$effect(() => {
		$activeChannel; $activeDM;
		typers = {};
		replyingTo = null;
		activeThread = null;
		editingDesc = false;
		Object.values(typerTimeouts).forEach(clearTimeout);
	});

	let typingLabel = $derived((() => {
		const names = Object.values(typers);
		if (names.length === 0) return '';
		if (names.length === 1) return `${names[0]} is typing`;
		if (names.length === 2) return `${names[0]} and ${names[1]} are typing`;
		return 'Several people are typing';
	})());

	// @mention autocomplete
	let mentionQuery = $state('');
	let mentionStart = $state(-1);
	let mentionIdx = $state(0);
	let showMentionPopup = $state(false);
	let mentionPopupEl = $state<HTMLElement | null>(null);

	$effect(() => {
		if (!showMentionPopup || !mentionPopupEl) return;
		const items = mentionPopupEl.querySelectorAll<HTMLElement>('.mention-item');
		items[mentionIdx]?.scrollIntoView({ block: 'nearest' });
	});

	let mentionMatches = $derived(
		showMentionPopup
			? $serverMembers
				.filter(m => m.user.id !== $currentUser?.id &&
					m.user.display_name.toLowerCase().includes(mentionQuery.toLowerCase()))
				.slice(0, 8)
			: [] as ServerMember[]
	);

	// Keep serverUnread in sync with the active server's channel unread counts and mentions
	$effect(() => {
		const srv = $activeServer;
		if (!srv) return;
		const hasUnread = $channels.some((c) => c.unread_count > 0 || $mentionedChannels.has(c.id));
		serverUnread.update((m) => ({ ...m, [srv.id]: hasUnread }));
	});

	// Clear OS notification badge when all unreads are gone.
	$effect(() => {
		const totalUnread = $channels.reduce((n, c) => n + (c.unread_count ?? 0), 0)
			+ $dmConversations.reduce((n, c) => n + (c.unread_count ?? 0), 0);
		if (totalUnread === 0) navigator.clearAppBadge?.();
		else navigator.setAppBadge?.(totalUnread);
	});

	function notifyRead() {
		navigator.serviceWorker?.controller?.postMessage({ type: 'mark-read' });
	}

	// Listen for mentions, DMs from background conversations, kicks/bans, friend events, and calls
	$effect(() => {
		const uid = $currentUser?.id;
		if (!uid) return;
		const room = 'user:' + uid;
		socket.subscribe(room);
		const unsub = socket.on((event) => {
			if (event.type === 'friend.request') {
				friendRequests.update((prev) => {
					if (prev.find((r) => r.user.id === event.payload.id)) return prev;
					return [...prev, { user: event.payload, since: new Date().toISOString() }];
				});
			}
			if (event.type === 'friend.accepted') {
				Promise.all([api.listFriends(), api.listFriendRequestsSent()]).then(([fr, sent]) => {
					friends.set(fr ?? []);
					friendRequestsSent.set(sent ?? []);
				});
			}
			if (event.type === 'call.ring') {
				handleIncomingCall(event.payload.conv_id, {
					userId: event.payload.from_user_id,
					displayName: event.payload.from_display_name,
					avatarUrl: event.payload.from_avatar_url,
				});
			}
			if (event.type === 'call.accepted') {
				handleCallAccepted(
					event.payload.conv_id,
					event.payload.from_display_name,
					event.payload.from_user_id,
				);
			}
			if (event.type === 'call.declined') handleCallDeclined();
			if (event.type === 'call.ended')   handleCallEnded();
			if (event.type === 'call.cancelled') handleCallCancelled();
			if (event.type === 'mention.new') {
				const chId = event.payload.channel_id;
				if ($activeChannel?.id !== chId) {
					mentionedChannels.update(s => new Set([...s, chId]));
				}
				if ($notifPrefs.mentionSound) playMentionSound();
			}
			if (event.type === 'reaction.new') {
				const chId = event.payload.channel_id;
				if ($activeChannel?.id !== chId) {
					mentionedChannels.update(s => new Set([...s, chId]));
				}
				if ($notifPrefs.mentionSound) playMentionSound();
			}
			// dm.new delivered via user room = message in a non-active conversation
			if (event.type === 'dm.new' && event.payload.conversation_id !== $activeDM?.id && event.payload.sender?.id !== uid) {
				if ($notifPrefs.dmSound) playDMSound();
				dmConversations.update((cs) => {
					const updated = cs.map((c) =>
						c.id === event.payload.conversation_id
							? { ...c, unread_count: c.unread_count + 1, last_message_at: event.payload.created_at }
							: c
					);
					return [...updated].sort((a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime());
				});
			}
			if (event.type === 'join_request.new') {
				const sid = event.payload.server_id;
				// Always mark the server as having a pending request (drives badge in server list).
				joinRequestPending.update((s) => new Set([...s, sid]));
				serverUnread.update((m) => ({ ...m, [sid]: true }));
				// If the admin is currently viewing that space, also add it to the live list.
				if (sid === $activeServer?.id) {
					pendingJoinRequests.update((prev) => {
						if (prev.find((r) => r.id === event.payload.request_id)) return prev;
						return [...prev, { id: event.payload.request_id, server_id: sid, user: event.payload.user, status: 'pending', created_at: new Date().toISOString() }];
					});
				}
			}
			if (event.type === 'member.kicked' || event.type === 'member.banned') {
				const sid = event.payload.server_id;
				servers.update((prev) => prev.filter((s) => s.id !== sid));
				if ($activeServer?.id === sid) {
					activeServer.set(null);
					activeChannel.set(null);
					channels.set([]);
				}
			}
		});
		return () => { unsub(); socket.unsubscribe(room); };
	});

	function onInput() {
		const el = textarea;
		if (!el) return;

		// Throttled typing indicator
		if (input.trim() && ($activeChannel || $activeDM)) {
			const now = Date.now();
			if (now - lastTypingSent > 2000) {
				lastTypingSent = now;
				const room = $activeChannel ? 'channel:' + $activeChannel.id : 'dm:' + $activeDM!.id;
				socket.send('typing', { room });
			}
		}

		// @mention autocomplete
		if (!$activeServer) return;
		const pos = el.selectionStart ?? 0;
		const before = input.slice(0, pos);
		const match = before.match(/@(\w*)$/);
		if (match) {
			mentionQuery = match[1];
			mentionStart = pos - match[0].length;
			mentionIdx = 0;
			showMentionPopup = true;
		} else {
			showMentionPopup = false;
		}
	}

	function insertMention(member: ServerMember) {
		const handle = member.user.display_name.replace(/\s+/g, '_');
		const el = textarea;
		if (!el) return;
		const curPos = el.selectionStart ?? input.length;
		input = input.slice(0, mentionStart) + '@' + handle + ' ' + input.slice(curPos);
		showMentionPopup = false;
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = mentionStart + handle.length + 2;
		}, 0);
	}

	function insertEmoji(emoji: string) {
		const el = textarea;
		if (!el) { input += emoji; return; }
		const start = el.selectionStart ?? input.length;
		const end = el.selectionEnd ?? input.length;
		input = input.slice(0, start) + emoji + input.slice(end);
		setTimeout(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = start + emoji.length;
		}, 0);
	}

	async function send() {
		const text = input.trim();
		if (!text) return;
		input = '';
		showMentionPopup = false;
		const replyId = replyingTo?.id;
		replyingTo = null;
		if ($activeDM) {
			await api.sendDM($activeDM.id, text);
		} else if ($activeChannel && $activeServer) {
			await api.sendMessage($activeServer.id, $activeChannel.id, text, replyId);
		}
	}

	async function uploadAndSend(file: File) {
		const isImage = file.type.startsWith('image/');
		const isVideo = file.type.startsWith('video/');
		if (!isImage && !isVideo) return;
		if (isVideo) {
			const maxMB = $instanceConfig.max_video_size_mb ?? 50;
			if (maxMB === 0) { alert('Video uploads are disabled on this instance.'); return; }
			if (file.size > maxMB * 1024 * 1024) {
				alert(`Video is too large. Maximum size is ${maxMB}MB.`);
				return;
			}
		}
		uploading = true;
		try {
			const { url } = await api.uploadFile(file);
			if ($activeDM) {
				await api.sendDM($activeDM.id, url);
			} else if ($activeChannel && $activeServer) {
				await api.sendMessage($activeServer.id, $activeChannel.id, url);
			}
		} finally {
			uploading = false;
		}
	}

	async function onPaste(e: ClipboardEvent) {
		const media = Array.from(e.clipboardData?.items ?? []).find(
			(i) => i.type.startsWith('image/') || i.type.startsWith('video/')
		);
		if (!media) return;
		e.preventDefault();
		const file = media.getAsFile();
		if (file) uploadAndSend(file);
	}

	// Gboard (Android) sends GIFs via beforeinput, not paste
	function onBeforeInput(e: InputEvent) {
		const file = e.dataTransfer?.files?.[0];
		if (!file?.type.startsWith('image/') && !file?.type.startsWith('video/')) return;
		e.preventDefault();
		uploadAndSend(file);
	}

	function onKeydown(e: KeyboardEvent) {
		if (showMentionPopup && mentionMatches.length > 0) {
			if (e.key === 'ArrowDown') { e.preventDefault(); mentionIdx = (mentionIdx + 1) % mentionMatches.length; return; }
			if (e.key === 'ArrowUp') { e.preventDefault(); mentionIdx = (mentionIdx - 1 + mentionMatches.length) % mentionMatches.length; return; }
			if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); insertMention(mentionMatches[mentionIdx]); return; }
			if (e.key === 'Escape') { showMentionPopup = false; return; }
		}
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	async function onreact(messageId: string, emoji: string) {
		if ($activeDM) {
			const msg = dmMessages.find((m) => m.id === messageId);
			if (!msg) return;
			const existing = msg.reactions?.find((rx: Reaction) => rx.emoji === emoji);
			if (existing?.mine) {
				await api.removeDMReaction($activeDM.id, messageId, emoji);
			} else {
				await api.addDMReaction($activeDM.id, messageId, emoji);
			}
		} else if ($activeServer && $activeChannel) {
			const msg = messages.find((m) => m.id === messageId) as Message | undefined;
			if (!msg) return;
			const existing = msg.reactions?.find((rx: Reaction) => rx.emoji === emoji);
			if (existing?.mine) {
				await api.removeReaction($activeServer.id, $activeChannel.id, messageId, emoji);
			} else {
				await api.addReaction($activeServer.id, $activeChannel.id, messageId, emoji);
			}
		}
	}

	async function saveDesc() {
		if (!$activeChannel || !$activeServer) return;
		const updated = await api.updateChannel($activeServer.id, $activeChannel.id, { description: descInput });
		channels.update((cs) => cs.map((c) => c.id === updated.id ? { ...c, description: updated.description } : c));
		activeChannel.update((ch) => ch ? { ...ch, description: updated.description } : ch);
		editingDesc = false;
	}

	// Whether the chat panel should be visible
	let showChat = $derived(!isMobile || !!$activeChannel || !!$activeDM);
	// Whether the sidebar should be visible
	let showSidebar = $derived(!isMobile || (!$activeChannel && !$activeDM));
</script>

<div class="app">
	<ServerList />

	{#if showSidebar}
		{#if $homeMode}
			<DMSidebar />
		{:else}
			<ChannelSidebar />
		{/if}
	{/if}

	{#if showChat}
		<main class="main">
			<header>
				{#if isMobile}
					<button class="back-btn" onclick={mobileBack}>
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M15 18l-6-6 6-6"/></svg>
						{$homeMode ? 'Messages' : ($activeServer?.name ?? 'Channels')}
					</button>
				{/if}
				<div class="channel-info">
					<span class="channel-name">
						{#if $activeChannel}
							{#if $activeChannel.type === 'threads'}
								{#if activeThread}
									<button class="breadcrumb-btn" onclick={() => (activeThread = null)}>💬 {$activeChannel.name}</button>
									<span class="breadcrumb-sep">/</span>
									{activeThread.title}
								{:else}
									💬 {$activeChannel.name}
								{/if}
							{:else if $activeChannel.type === 'voice'}
								🔊 {$activeChannel.name}
							{:else}
								# {$activeChannel.name}
							{/if}
						{/if}
						{#if $activeDM}@ {$activeDM.other_user.display_name}{/if}
					</span>
					{#if $activeChannel && !activeThread}
						{#if editingDesc}
							<div class="desc-edit">
								<input
									bind:value={descInput}
									placeholder="Add a channel description…"
									onkeydown={(e) => { if (e.key === 'Enter') saveDesc(); if (e.key === 'Escape') editingDesc = false; }}
									autofocus
								/>
								<button class="desc-save-btn" onclick={saveDesc}>Save</button>
								<button class="desc-cancel-btn" onclick={() => (editingDesc = false)}>Cancel</button>
							</div>
						{:else if $activeChannel.description}
							<span class="channel-desc">
								{$activeChannel.description}
								{#if $activeServer?.role === 'owner' || $activeServer?.role === 'admin'}
									<button class="desc-edit-btn" onclick={() => { descInput = $activeChannel?.description ?? ''; editingDesc = true; }} title="Edit description">
										<svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>
									</button>
								{/if}
							</span>
						{:else if $activeServer?.role === 'owner' || $activeServer?.role === 'admin'}
							<button class="desc-add-btn" onclick={() => { descInput = ''; editingDesc = true; }}>+ Add description</button>
						{/if}
					{/if}
				</div>
				<div class="header-actions">
					{#if $activeChannel}
						<button onclick={() => (showMembers = !showMembers)} class="icon-btn" class:active={showMembers} title="Members" style="position:relative">
							<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>
							{#if $pendingJoinRequests.length > 0}
								<span style="position:absolute;top:2px;right:2px;width:8px;height:8px;border-radius:50%;background:#e04545;border:2px solid var(--bg-panel)"></span>
							{/if}
						</button>
					{/if}
				</div>
			</header>

			{#if $activeChannel?.type === 'threads'}
				{#if activeThread}
					<ThreadView thread={activeThread} onback={() => (activeThread = null)} />
				{:else}
					<ThreadChannel onopen={(t) => (activeThread = t)} />
				{/if}
			{:else}

			<MessageFeed
				messages={$activeChannel ? messages : dmMessages}
				isDM={!!$activeDM}
				onreply={(msg) => { replyingTo = msg; setTimeout(() => textarea?.focus(), 0); }}
				{onreact}
			/>

			{#if replyingTo}
				<div class="reply-bar">
					<div class="reply-bar-preview">
						<span class="reply-bar-name">Replying to {replyingTo.author?.display_name}</span>
						<span class="reply-bar-text">{replyingTo.content.startsWith('http') ? '[image]' : replyingTo.content.slice(0, 80)}{replyingTo.content.length > 80 ? '…' : ''}</span>
					</div>
					<button class="reply-bar-cancel" onclick={() => replyingTo = null}>✕</button>
				</div>
			{/if}

			<div class="typing-bar" class:visible={!!typingLabel}>
				<span class="typing-dots">
					<span></span><span></span><span></span>
				</span>
				<span class="typing-text">{typingLabel}</span>
			</div>

			<div class="input-area">
				<div class="input-actions">
					<button
						class="action-icon"
						disabled={(!$activeChannel && !$activeDM) || uploading}
						onclick={() => fileInput.click()}
						title="Upload image or video"
					>
						{#if uploading}
							<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
						{:else}
							<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>
						{/if}
					</button>
					<button
						class="action-icon"
						disabled={!$activeChannel && !$activeDM}
						onclick={() => (showEmoji = !showEmoji)}
						title="Emoji"
						class:active={showEmoji}
					>
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M8 13s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
					</button>
					{#if showEmoji}
						<EmojiPicker onSelect={insertEmoji} onClose={() => (showEmoji = false)} />
					{/if}
				</div>
				<input bind:this={fileInput} type="file" accept="image/*,video/mp4,video/webm,video/quicktime" style="display:none"
					onchange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) uploadAndSend(f); }} />
				{#if showMentionPopup && mentionMatches.length > 0}
					<div class="mention-popup" bind:this={mentionPopupEl}>
						{#each mentionMatches as member, i}
							<button
								class="mention-item"
								class:selected={i === mentionIdx}
								onmousedown={(e) => { e.preventDefault(); insertMention(member); }}
								onmouseenter={() => (mentionIdx = i)}
							>
								<img
									src={member.user.avatar_url || '/default-avatar.png'}
									alt=""
									class="mention-avatar"
								/>
								<div class="mention-info">
									<span class="mention-name">{member.user.display_name}</span>
									{#if member.space_roles?.length}
										<span class="mention-role" style="color:{member.space_roles[0].color}">{member.space_roles[0].name}</span>
									{/if}
								</div>
								<span class="mention-hint">↵</span>
							</button>
						{/each}
					</div>
				{/if}
				<textarea
					bind:this={textarea}
					bind:value={input}
					onkeydown={onKeydown}
					oninput={onInput}
					onpaste={onPaste}
					onbeforeinput={onBeforeInput}
					placeholder={$activeChannel ? `Message #${$activeChannel.name}` : $activeDM ? `Message ${$activeDM.other_user.display_name}` : 'Select a channel'}
					rows="1"
					disabled={(!$activeChannel && !$activeDM) || uploading}
				></textarea>
			</div>
			{/if}
		</main>

		{#if showMembers && $activeChannel && $activeServer}
			{#if isMobile}
				<div class="members-overlay">
					<div class="members-overlay-header">
						<span>Members</span>
						<button onclick={() => (showMembers = false)}>✕</button>
					</div>
					<MemberList serverId={$activeServer.id} onDmStarted={() => (showMembers = false)} />
				</div>
			{:else}
				<MemberList serverId={$activeServer.id} onDmStarted={() => (showMembers = false)} />
			{/if}
		{/if}
	{/if}
</div>

{#if $showProfileModal}
	<ProfileModal onclose={() => showProfileModal.set(false)} />
{/if}

<CallNotification />

<style>
	:global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
	:global(body) { background: var(--bg); color: var(--text); font-family: system-ui, sans-serif; overflow: hidden; }

	.app {
		display: flex;
		height: 100dvh;
		overflow: hidden;
	}
	@media (max-width: 767px) {
		.app { flex-direction: column; }
	}
	.main {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-width: 0;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0 1rem;
		min-height: 48px;
		border-bottom: 1px solid #0e0e10;
		background: var(--bg-panel);
		flex-shrink: 0;
	}
	.back-btn {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		background: none;
		border: none;
		color: var(--accent);
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
		padding: 0.375rem 0.5rem;
		border-radius: 4px;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.back-btn:hover { background: rgba(232,84,30,0.1); }
	.channel-info {
		flex: 1;
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 1px;
		min-width: 0;
		padding: 6px 0;
	}
	.channel-name {
		font-weight: 600;
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-size: 0.95rem;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.breadcrumb-btn {
		background: none;
		border: none;
		font: inherit;
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0;
		white-space: nowrap;
	}
	.breadcrumb-btn:hover { color: var(--text); text-decoration: underline; }
	.breadcrumb-sep { color: var(--text-muted); font-weight: 400; }
	.channel-desc {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.75rem;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.desc-edit-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		padding: 1px 3px;
		border-radius: 3px;
		display: inline-flex;
		align-items: center;
		opacity: 0.6;
		flex-shrink: 0;
	}
	.desc-edit-btn:hover { opacity: 1; color: var(--accent); }
	.desc-add-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.72rem;
		padding: 0;
		opacity: 0.5;
	}
	.desc-add-btn:hover { opacity: 1; color: var(--accent); }
	.desc-edit {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}
	.desc-edit input {
		flex: 1;
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 2px 6px;
		border-radius: 4px;
		font-size: 0.78rem;
		outline: none;
		font-family: inherit;
		min-width: 0;
	}
	.desc-edit input:focus { border-color: var(--accent); }
	.desc-save-btn {
		background: var(--accent);
		border: none;
		color: white;
		padding: 2px 8px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.75rem;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.desc-cancel-btn {
		background: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
		padding: 2px 6px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.75rem;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.header-actions { display: flex; gap: 0.5rem; flex-shrink: 0; align-items: center; }
	.icon-btn {
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.35rem;
		border-radius: 4px;
		color: #c8c7d0;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.icon-btn:hover { color: var(--text); background: rgba(255,255,255,0.1); }
	.icon-btn.active { color: var(--accent); }
	.reply-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 1rem;
		background: var(--bg-input);
		border-top: 1px solid var(--border);
		flex-shrink: 0;
	}
	.reply-bar-preview {
		flex: 1;
		min-width: 0;
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
	}
	.reply-bar-name {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--accent);
		white-space: nowrap;
	}
	.reply-bar-text {
		font-size: 0.75rem;
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.reply-bar-cancel {
		background: none;
		border: none;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 0.8rem;
		padding: 0.25rem;
		flex-shrink: 0;
	}
	.reply-bar-cancel:hover { color: var(--text); }
	.typing-bar {
		height: 0;
		overflow: hidden;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0 1rem;
		transition: height 0.15s ease;
		flex-shrink: 0;
	}
	.typing-bar.visible {
		height: 1.5rem;
	}
	.typing-dots {
		display: flex;
		align-items: center;
		gap: 3px;
	}
	.typing-dots span {
		width: 5px;
		height: 5px;
		background: var(--text-muted);
		border-radius: 50%;
		animation: typing-bounce 1.2s ease-in-out infinite;
	}
	.typing-dots span:nth-child(2) { animation-delay: 0.2s; }
	.typing-dots span:nth-child(3) { animation-delay: 0.4s; }
	@keyframes typing-bounce {
		0%, 60%, 100% { transform: translateY(0); opacity: 0.5; }
		30% { transform: translateY(-4px); opacity: 1; }
	}
	.typing-text {
		font-size: 0.75rem;
		color: var(--text-muted);
		font-style: italic;
	}
	.input-area {
		padding: 0.75rem 1rem;
		flex-shrink: 0;
		display: flex;
		gap: 0.5rem;
		align-items: flex-end;
		position: relative;
	}
	.mention-popup {
		position: absolute;
		bottom: calc(100% - 0.5rem);
		left: 3.5rem;
		right: 1rem;
		background: var(--bg-panel);
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
		box-shadow: 0 -4px 12px rgba(0,0,0,0.4);
		z-index: 50;
		max-height: 240px;
		overflow-y: auto;
	}
	.mention-item {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: 100%;
		padding: 0.4rem 0.75rem;
		background: none;
		border: none;
		color: var(--text);
		cursor: pointer;
		text-align: left;
		font-size: 0.9rem;
		font-family: inherit;
	}
	.mention-item:hover, .mention-item.selected { background: var(--bg-input); }
	.mention-item.selected .mention-hint { opacity: 1; }
	.mention-avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		object-fit: cover;
		flex-shrink: 0;
	}
	.mention-info {
		flex: 1;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		min-width: 0;
	}
	.mention-name { font-weight: 500; }
	.mention-role { font-size: 0.75rem; }
	.mention-hint {
		font-size: 0.7rem;
		color: var(--text-muted);
		opacity: 0;
		flex-shrink: 0;
	}
	.input-actions {
		display: flex;
		gap: 0.25rem;
		flex-shrink: 0;
		position: relative;
	}
	.action-icon {
		background: var(--border);
		border: 1px solid #3a3a45;
		color: #c8c7d0;
		width: 36px;
		height: 36px;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: background 0.15s, color 0.15s;
	}
	.action-icon:hover:not(:disabled) { background: #3a3a45; color: var(--text); }
	.action-icon.active { background: var(--accent); border-color: var(--accent); color: white; }
	.action-icon:disabled { opacity: 0.35; cursor: not-allowed; }
	textarea {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: 8px;
		color: var(--text);
		padding: 0.75rem 1rem;
		font-size: 0.95rem;
		resize: none;
		outline: none;
		font-family: inherit;
	}
	textarea:disabled { opacity: 0.5; cursor: not-allowed; }

	.members-overlay {
		display: none;
	}
	@media (max-width: 767px) {
		textarea { font-size: 16px; /* prevents iOS zoom on focus */ }
		.input-area { padding: 0.5rem; }
		.members-overlay {
			display: flex;
			flex-direction: column;
			position: fixed;
			inset: 0;
			z-index: 50;
			background: var(--bg-sidebar);
		}
		.members-overlay-header {
			display: flex;
			align-items: center;
			justify-content: space-between;
			padding: 0 1rem;
			height: 48px;
			border-bottom: 1px solid #0e0e10;
			font-weight: 700;
			flex-shrink: 0;
		}
		.members-overlay-header button {
			background: none;
			border: none;
			color: var(--text-muted);
			cursor: pointer;
			font-size: 1rem;
			padding: 0.25rem;
		}
	}
</style>
