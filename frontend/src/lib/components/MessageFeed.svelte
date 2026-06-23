<script lang="ts">
	import { currentUser, activeServer, activeChannel, activeDM, serverMembers } from '$lib/stores';
	import { api, type Message, type DirectMessage, type LinkPreview } from '$lib/api';
	import Avatar from './Avatar.svelte';
	import EmojiPicker from './EmojiPicker.svelte';
	import LightboxImage from './LightboxImage.svelte';

	const isAdmin = $derived(
		$activeServer?.role === 'owner' || $activeServer?.role === 'admin'
	);

	type AnyMessage = Message | DirectMessage;

	// Set of lowercased handles (display_name with spaces→_) for highlight matching
	let memberHandles = $derived(new Set(
		$serverMembers.map(m => m.user.display_name.replace(/\s+/g, '_').toLowerCase())
	));

	type ContentPart = { type: 'text' | 'mention'; value: string };

	const EMOTICONS: [RegExp, string][] = [
		[/<3/g,    '❤️'],
		[/:-?\)/g, '😊'],
		[/:-?D/g,  '😄'],
		[/:-?P/gi, '😛'],
		[/:-?\(/g, '😢'],
		[/;-?\)/g, '😉'],
		[/:-?O/gi, '😮'],
	];

	function applyEmoticons(s: string): string {
		for (const [re, emoji] of EMOTICONS) s = s.replace(re, emoji);
		return s;
	}

	function parseContent(text: string): ContentPart[] {
		const parts: ContentPart[] = [];
		let last = 0;
		const re = /@(\w+)/g;
		let m: RegExpExecArray | null;
		while ((m = re.exec(text)) !== null) {
			if (m.index > last) parts.push({ type: 'text', value: applyEmoticons(text.slice(last, m.index)) });
			parts.push(
				memberHandles.has(m[1].toLowerCase())
					? { type: 'mention', value: m[0] }
					: { type: 'text', value: m[0] }
			);
			last = m.index + m[0].length;
		}
		if (last < text.length) parts.push({ type: 'text', value: applyEmoticons(text.slice(last)) });
		return parts;
	}

	let {
		messages,
		isDM = false,
		onreply,
		onreact
	}: { messages: AnyMessage[]; isDM?: boolean; onreply?: (msg: Message) => void; onreact?: (messageId: string, emoji: string) => void } = $props();

	let reactionPickerFor = $state<string | null>(null);
	let reactionPickerRect = $state<DOMRect | null>(null);
	let mobileMenuFor = $state<string | null>(null);
	let mobileMenuRect = $state<DOMRect | null>(null);
	let lightboxSrc = $state<string | null>(null);

	let container: HTMLElement;
	let stickToBottom = true;
	let editingId = $state<string | null>(null);
	let editContent = $state('');

	$effect(() => {
		messages.length;
		if (stickToBottom && container) {
			container.scrollTop = container.scrollHeight;
		}
	});

	function onScroll() {
		if (!container) return;
		stickToBottom = container.scrollTop + container.clientHeight >= container.scrollHeight - 50;
	}

	function isMessage(m: AnyMessage): m is Message {
		return 'author' in m;
	}

	function getReactions(m: AnyMessage) {
		return (isMessage(m) ? m.reactions : (m as DirectMessage).reactions) ?? [];
	}

	function getAuthor(m: AnyMessage) {
		return isMessage(m) ? m.author : (m as DirectMessage).sender;
	}

	function formatTime(ts: string) {
		return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	function formatDate(ts: string) {
		return new Date(ts).toLocaleDateString([], { month: 'long', day: 'numeric', year: 'numeric' });
	}

	function sameDay(a: string, b: string) {
		return formatDate(a) === formatDate(b);
	}

	function replyPreview(content: string): string {
		if (isImageUrl(content.trim())) return '[image]';
		if (isVideoUrl(content.trim())) return '[video]';
		return content.length > 80 ? content.slice(0, 80) + '…' : content;
	}

	const urlRe = /https?:\/\/[^\s<>"']+/gi;
	let previews = $state<Record<string, LinkPreview | null | 'loading'>>({});

	function extractUrl(content: string): string | null {
		if (isImageUrl(content.trim()) || isVideoUrl(content.trim())) return null;
		const m = urlRe.exec(content);
		urlRe.lastIndex = 0;
		return m ? m[0] : null;
	}

	function fetchPreview(url: string) {
		if (url in previews) return;
		previews[url] = 'loading';
		api.unfurl(url).then((p) => {
			previews[url] = p.title ? p : null;
		}).catch(() => {
			previews[url] = null;
		});
	}

	function isImageUrl(text: string): boolean {
		const t = text.trim();
		return /^\/avatars\/[a-f0-9-]+\.(jpg|jpeg|png|gif|webp|svg)$/i.test(t) ||
			/^https?:\/\/[^/]+\/avatars\/[a-f0-9-]+\.(jpg|jpeg|png|gif|webp|svg)(\?.*)?$/i.test(t);
	}

	function isVideoUrl(text: string): boolean {
		const t = text.trim();
		return /^\/avatars\/[a-f0-9-]+\.(mp4|webm|mov)$/i.test(t) ||
			/^https?:\/\/[^/]+\/avatars\/[a-f0-9-]+\.(mp4|webm|mov)(\?.*)?$/i.test(t);
	}

	function startEdit(m: AnyMessage) {
		editingId = m.id;
		editContent = m.content;
	}

	function cancelEdit() {
		editingId = null;
		editContent = '';
	}

	async function saveEdit(m: AnyMessage) {
		if (!editContent.trim()) return;
		if (isDM && $activeDM) {
			await api.editDM($activeDM.id, m.id, editContent.trim());
		} else if (!isDM && $activeServer && $activeChannel) {
			await api.editMessage($activeServer.id, $activeChannel.id, m.id, editContent.trim());
		}
		cancelEdit();
	}

	function onEditKeydown(e: KeyboardEvent, m: AnyMessage) {
		if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); saveEdit(m); }
		if (e.key === 'Escape') cancelEdit();
	}

	async function deleteMsg(m: AnyMessage) {
		if (isDM && $activeDM) {
			await api.deleteDM($activeDM.id, m.id);
		} else if (!isDM && $activeServer && $activeChannel) {
			await api.deleteMessage($activeServer.id, $activeChannel.id, m.id);
		}
	}
</script>

<div class="feed" bind:this={container} onscroll={onScroll}>
	<div class="spacer"></div>

	{#if messages.length === 0}
		<div class="empty">No messages yet. Say hello!</div>
	{/if}

	{#each messages as msg, i}
		{#if i === 0 || !sameDay(messages[i - 1].created_at, msg.created_at)}
			<div class="date-divider">
				<span>{formatDate(msg.created_at)}</span>
			</div>
		{/if}

		{@const author = getAuthor(msg)}
		{@const isOwn = author.id === $currentUser?.id}
		{@const canDelete = isOwn || (!isDM && isAdmin)}
		{@const showHeader = i === 0 || getAuthor(messages[i - 1]).id !== author.id || !sameDay(messages[i-1].created_at, msg.created_at) || !!(isMessage(msg) && msg.reply_to)}
		{@const editing = editingId === msg.id}

		<div class="message" class:editing>
			{#if showHeader}
				<div class="avatar">
					<Avatar url={author.avatar_url} name={author.display_name} userId={author.id} size={40} />
				</div>
				<div class="content">
					{#if isMessage(msg) && msg.reply_to}
						<div class="reply-quote">
							<span class="reply-quote-name">{msg.reply_to.author_name}</span>
							<span class="reply-quote-text">{replyPreview(msg.reply_to.content)}</span>
						</div>
					{/if}
					<div class="header">
						<span class="name" style={author.role_color ? `color:${author.role_color}` : ''}>{author.display_name}</span>
						<span class="time">{formatTime(msg.created_at)}</span>
						{#if (isMessage(msg) && msg.edited_at) || (!isMessage(msg) && (msg as DirectMessage).edited_at)}
							<span class="edited">(edited)</span>
						{/if}
					</div>
					{#if editing}
						<textarea
							bind:value={editContent}
							onkeydown={(e) => onEditKeydown(e, msg)}
							class="edit-input"
							rows="2"
							autofocus
						></textarea>
						<div class="edit-hint">Enter to save · Esc to cancel</div>
					{:else if isImageUrl(msg.content)}
						<img src={msg.content} alt="uploaded" class="msg-image" loading="lazy" onclick={() => lightboxSrc = msg.content} />
					{:else if isVideoUrl(msg.content)}
						<!-- svelte-ignore a11y-media-has-caption -->
						<video src={msg.content} class="msg-video" controls preload="metadata"></video>
					{:else}
						{@const linkUrl = extractUrl(msg.content)}
						{#if linkUrl}{fetchPreview(linkUrl)}{/if}
						<p>{#each parseContent(msg.content) as part}{#if part.type === 'mention'}<span class="mention">{part.value}</span>{:else}{part.value}{/if}{/each}</p>
						{#if linkUrl && previews[linkUrl] && previews[linkUrl] !== 'loading'}
							{@const pv = previews[linkUrl] as LinkPreview}
							<div class="link-preview">
								{#if pv.image}<img class="preview-img" src={pv.image} alt="" loading="lazy" />{/if}
								<div class="preview-body">
									{#if pv.site_name}<span class="preview-site">{pv.site_name}</span>{/if}
									<a class="preview-title" href={pv.url} target="_blank" rel="noopener noreferrer">{pv.title}</a>
									{#if pv.description}<p class="preview-desc">{pv.description}</p>{/if}
								</div>
							</div>
						{/if}
					{/if}
					{#if getReactions(msg).length > 0}
						<div class="reactions">
							{#each getReactions(msg) as rxn}
								<button class="reaction-pill" class:mine={rxn.mine} onclick={() => onreact?.(msg.id, rxn.emoji)} title={rxn.mine ? 'Remove reaction' : 'React'}>{rxn.emoji} <span class="rxn-count">{rxn.count}</span></button>
							{/each}
						</div>
					{/if}
				</div>
			{:else}
				<div class="avatar-spacer"></div>
				<div class="content">
					{#if editing}
						<textarea
							bind:value={editContent}
							onkeydown={(e) => onEditKeydown(e, msg)}
							class="edit-input"
							rows="2"
							autofocus
						></textarea>
						<div class="edit-hint">Enter to save · Esc to cancel</div>
					{:else if isImageUrl(msg.content)}
						<img src={msg.content} alt="uploaded" class="msg-image" loading="lazy" onclick={() => lightboxSrc = msg.content} />
					{:else if isVideoUrl(msg.content)}
						<!-- svelte-ignore a11y-media-has-caption -->
						<video src={msg.content} class="msg-video" controls preload="metadata"></video>
					{:else}
						{@const linkUrl = extractUrl(msg.content)}
						{#if linkUrl}{fetchPreview(linkUrl)}{/if}
						<p>{#each parseContent(msg.content) as part}{#if part.type === 'mention'}<span class="mention">{part.value}</span>{:else}{part.value}{/if}{/each}</p>
						{#if linkUrl && previews[linkUrl] && previews[linkUrl] !== 'loading'}
							{@const pv = previews[linkUrl] as LinkPreview}
							<div class="link-preview">
								{#if pv.image}<img class="preview-img" src={pv.image} alt="" loading="lazy" />{/if}
								<div class="preview-body">
									{#if pv.site_name}<span class="preview-site">{pv.site_name}</span>{/if}
									<a class="preview-title" href={pv.url} target="_blank" rel="noopener noreferrer">{pv.title}</a>
									{#if pv.description}<p class="preview-desc">{pv.description}</p>{/if}
								</div>
							</div>
						{/if}
					{/if}
					{#if getReactions(msg).length > 0}
						<div class="reactions">
							{#each getReactions(msg) as rxn}
								<button class="reaction-pill" class:mine={rxn.mine} onclick={() => onreact?.(msg.id, rxn.emoji)} title={rxn.mine ? 'Remove reaction' : 'React'}>{rxn.emoji} <span class="rxn-count">{rxn.count}</span></button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			{#if !editing}
				<!-- Desktop: hover-reveal action buttons -->
				<div class="msg-actions desktop-actions">
					{#if onreact}
						<button class="action-btn" onclick={(e) => {
							e.stopPropagation();
							if (reactionPickerFor === msg.id) { reactionPickerFor = null; reactionPickerRect = null; }
							else { reactionPickerFor = msg.id; reactionPickerRect = (e.currentTarget as HTMLElement).getBoundingClientRect(); }
						}} title="Add reaction">😊</button>
					{/if}
					{#if isMessage(msg) && onreply}
						<button class="action-btn" onclick={() => onreply(msg as Message)} title="Reply">↩</button>
					{/if}
					{#if isOwn}
						<button class="action-btn edit" onclick={() => startEdit(msg)} title="Edit">✏</button>
					{/if}
					{#if canDelete}
						<button class="action-btn delete" onclick={() => deleteMsg(msg)} title="Delete">✕</button>
					{/if}
				</div>
				<!-- Mobile: single ⋯ button that pops a fixed-position menu -->
				<div class="msg-actions mobile-actions">
					<button class="action-btn more-btn" onclick={(e) => {
						e.stopPropagation();
						if (mobileMenuFor === msg.id) { mobileMenuFor = null; mobileMenuRect = null; }
						else { mobileMenuFor = msg.id; mobileMenuRect = (e.currentTarget as HTMLElement).getBoundingClientRect(); reactionPickerFor = null; }
					}} title="More actions">⋯</button>
				</div>
			{/if}
		</div>
	{/each}
</div>

{#if lightboxSrc}
	<LightboxImage src={lightboxSrc} onclose={() => lightboxSrc = null} />
{/if}

{#if reactionPickerFor && reactionPickerRect && onreact}
	{@const msgId = reactionPickerFor}
	<EmojiPicker
		anchorRect={reactionPickerRect}
		onSelect={(emoji) => { onreact(msgId, emoji); reactionPickerFor = null; reactionPickerRect = null; }}
		onClose={() => { reactionPickerFor = null; reactionPickerRect = null; }}
	/>
{/if}

{#if mobileMenuFor && mobileMenuRect}
	{@const menuMsgId = mobileMenuFor}
	{@const menuMsg = messages.find(m => m.id === menuMsgId)}
	{@const menuAuthor = menuMsg ? getAuthor(menuMsg) : null}
	{@const menuIsOwn = menuAuthor?.id === $currentUser?.id}
	{@const menuCanDelete = menuIsOwn || isAdmin}
	<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
	<div class="mobile-menu-overlay" onclick={() => { mobileMenuFor = null; mobileMenuRect = null; }}></div>
	<div class="mobile-context-menu" style="bottom:{window.innerHeight - mobileMenuRect.top + 8}px;left:{Math.min(mobileMenuRect.right - 160, window.innerWidth - 172)}px">
		{#if menuMsg && onreact}
			<button onclick={() => {
				reactionPickerFor = menuMsgId;
				reactionPickerRect = mobileMenuRect;
				mobileMenuFor = null;
			}}>😊 React</button>
		{/if}
		{#if menuMsg && isMessage(menuMsg) && onreply}
			<button onclick={() => { onreply(menuMsg as Message); mobileMenuFor = null; mobileMenuRect = null; }}>↩ Reply</button>
		{/if}
		{#if menuIsOwn && menuMsg}
			<button onclick={() => { startEdit(menuMsg); mobileMenuFor = null; mobileMenuRect = null; }}>✏ Edit</button>
		{/if}
		{#if menuCanDelete && menuMsg}
			<button class="danger" onclick={() => { deleteMsg(menuMsg); mobileMenuFor = null; mobileMenuRect = null; }}>✕ Delete</button>
		{/if}
	</div>
{/if}

<style>
	.feed {
		flex: 1;
		overflow-y: auto;
		padding-bottom: 0.5rem;
		display: flex;
		flex-direction: column;
	}
	.spacer { flex: 1; }
	.empty {
		padding: 1rem;
		text-align: center;
		color: var(--text-muted);
		font-size: 0.9rem;
	}
	.date-divider {
		display: flex;
		align-items: center;
		padding: 0.5rem 1rem;
		gap: 0.75rem;
		color: var(--text-muted);
		font-size: 0.75rem;
		font-weight: 600;
	}
	.date-divider::before, .date-divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--border);
	}
	.message {
		display: flex;
		gap: 0.75rem;
		padding: 0.125rem 1rem;
		position: relative;
	}
	.message:hover { background: rgba(255,255,255,0.02); }
	.message:hover .msg-actions { opacity: 1; }
	.message.editing { background: rgba(232,84,30,0.05); }
	.avatar { flex-shrink: 0; }
	.avatar-spacer { width: 40px; flex-shrink: 0; }
	.content { flex: 1; min-width: 0; }
	.header {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		margin-bottom: 0.1rem;
	}
	.name { font-weight: 600; color: var(--text); font-size: 0.95rem; }
	.time { font-size: 0.7rem; color: var(--text-muted); }
	.edited { font-size: 0.65rem; color: var(--text-muted); font-style: italic; }
	.reply-quote {
		display: flex;
		gap: 0.4rem;
		align-items: baseline;
		background: var(--bg-input);
		border-left: 3px solid var(--accent);
		border-radius: 0 4px 4px 0;
		padding: 0.25rem 0.5rem;
		margin-bottom: 0.25rem;
		font-size: 0.8rem;
		cursor: default;
		max-width: 100%;
		overflow: hidden;
	}
	.reply-quote-name {
		color: var(--accent);
		font-weight: 600;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.reply-quote-text {
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	p {
		color: var(--text);
		font-size: 0.9rem;
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}
	:global(.mention) {
		color: var(--accent);
		background: color-mix(in srgb, var(--accent) 15%, transparent);
		border-radius: 3px;
		padding: 0 3px;
		font-weight: 600;
	}
	.msg-image {
		max-width: min(480px, 100%);
		max-height: 320px;
		border-radius: 6px;
		display: block;
		margin-top: 0.25rem;
		cursor: pointer;
	}
	.msg-image:hover { opacity: 0.9; }
	.msg-video {
		max-width: min(480px, 100%);
		max-height: 320px;
		border-radius: 6px;
		display: block;
		margin-top: 0.25rem;
		background: #000;
	}
	.edit-input {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--accent);
		border-radius: 6px;
		color: var(--text);
		padding: 0.5rem;
		font-size: 0.9rem;
		font-family: inherit;
		resize: none;
		outline: none;
		line-height: 1.5;
	}
	.edit-hint {
		font-size: 0.7rem;
		color: var(--text-muted);
		margin-top: 0.2rem;
	}
	.msg-actions {
		position: absolute;
		right: 1rem;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		gap: 0.25rem;
		opacity: 0;
		transition: opacity 0.1s;
	}
	.desktop-actions { display: flex; }
	.mobile-actions { display: none; }
	@media (max-width: 767px) {
		.desktop-actions { display: none; }
		.mobile-actions {
			display: flex;
			opacity: 1;
			position: static;
			transform: none;
			align-self: center;
			flex-shrink: 0;
		}
	}
	.more-btn { font-size: 1rem; font-weight: bold; letter-spacing: 1px; }
	.mobile-menu-overlay {
		position: fixed;
		inset: 0;
		z-index: 90;
	}
	.mobile-context-menu {
		position: fixed;
		z-index: 91;
		background: #1e1e24;
		border: 1px solid var(--border);
		border-radius: 8px;
		min-width: 160px;
		overflow: hidden;
		box-shadow: 0 4px 20px rgba(0,0,0,0.5);
	}
	.mobile-context-menu button {
		display: block;
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		color: var(--text);
		padding: 0.65rem 1rem;
		font-size: 0.9rem;
		cursor: pointer;
		font-family: inherit;
	}
	.mobile-context-menu button:hover { background: rgba(255,255,255,0.06); }
	.mobile-context-menu button.danger { color: #e04545; }
	.mobile-context-menu button + button { border-top: 1px solid var(--border); }
	.action-btn {
		background: #222228;
		border: 1px solid var(--border);
		color: var(--text);
		width: 26px;
		height: 26px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.7rem;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.action-btn:hover { background: var(--border); }
	.action-btn.delete:hover { background: #e04545; border-color: #e04545; }
	.link-preview {
		display: flex;
		gap: 0.75rem;
		border: 1px solid var(--border);
		border-left: 3px solid var(--accent);
		border-radius: 4px;
		padding: 0.6rem 0.75rem;
		margin-top: 0.4rem;
		max-width: 480px;
		background: var(--bg-input);
	}
	.preview-img {
		width: 80px;
		height: 60px;
		object-fit: cover;
		border-radius: 3px;
		flex-shrink: 0;
	}
	.preview-body { display: flex; flex-direction: column; gap: 0.2rem; min-width: 0; }
	.preview-site { font-size: 0.72rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }
	.preview-title { font-size: 0.85rem; font-weight: 600; color: var(--text); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.preview-title:hover { text-decoration: underline; }
	.preview-desc { font-size: 0.78rem; color: var(--text-muted); margin: 0; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
	.reactions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		margin-top: 0.3rem;
	}
	.reaction-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		background: rgba(255,255,255,0.05);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 0.1rem 0.5rem;
		font-size: 0.88rem;
		line-height: 1.6;
		cursor: pointer;
		color: var(--text);
		transition: background 0.1s, border-color 0.1s;
	}
	.reaction-pill:hover { background: rgba(255,255,255,0.1); border-color: var(--accent); }
	.reaction-pill.mine {
		background: color-mix(in srgb, var(--accent) 20%, transparent);
		border-color: var(--accent);
	}
	.rxn-count { font-size: 0.75rem; color: var(--text-muted); font-weight: 600; }
</style>
