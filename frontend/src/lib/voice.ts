import { get, writable } from 'svelte/store';
import {
	Room,
	RoomEvent,
	Track,
	type RemoteParticipant,
	LocalAudioTrack,
	LocalVideoTrack,
	createLocalScreenTracks,
	createLocalVideoTrack,
} from 'livekit-client';
import { socket } from './socket';
import type { VoicePeer } from './api';
import { api } from './api';
import { voiceParticipants, currentUser } from './stores';

// ── Types ─────────────────────────────────────────────────────────────────────

export interface VoiceState {
	channelId: string | null;   // set for channel calls
	subChannelId: string | null; // set when inside a breakout sub-channel
	subChannelName: string | null;
	dmConvId: string | null;    // set for DM calls
	dmPeerUserId: string | null;
	label: string;              // display name in VoicePanel
	serverId: string | null;
	muted: boolean;
	cameraOn: boolean;
	screenSharing: boolean;
	connecting: boolean;
	peers: VoicePeer[];
	micGain: number;
	speakingUsers: Set<string>;
	echoCancellation: boolean;
	noiseSuppression: boolean;
	autoGainControl: boolean;
}

export interface CallState {
	status: 'idle' | 'ringing_out' | 'ringing_in';
	convId: string | null;
	peer: { userId: string; displayName: string; avatarUrl: string } | null;
}

const DEFAULT_VOICE: VoiceState = {
	channelId: null,
	subChannelId: null,
	subChannelName: null,
	dmConvId: null,
	dmPeerUserId: null,
	label: '',
	serverId: null,
	muted: false,
	cameraOn: false,
	screenSharing: false,
	connecting: false,
	peers: [],
	micGain: 1,
	speakingUsers: new Set(),
	echoCancellation: true,
	noiseSuppression: true,
	autoGainControl: true,
};

export const voiceState = writable<VoiceState>({ ...DEFAULT_VOICE });
export const peerVolumesStore = writable<Record<string, number>>({});
export const callState = writable<CallState>({ status: 'idle', convId: null, peer: null });
export const localVideoStore = writable<MediaStream | null>(null);
export const remoteVideoStore = writable<Record<string, MediaStream>>({});

// ── Module-level state ────────────────────────────────────────────────────────

let livekitRoom: Room | null = null;
let localVideoTrack: LocalVideoTrack | null = null;
let localStream: MediaStream | null = null;
let processedStream: MediaStream | null = null;
let audioCtx: AudioContext | null = null;
let gainNode: GainNode | null = null;
let localAnalyser: AnalyserNode | null = null;
let localAudioTrack: LocalAudioTrack | null = null;
let channelId: string | null = null;
let subChannelId: string | null = null;
let subChannelName: string | null = null;
let dmConvId: string | null = null;
let dmPeerUserId: string | null = null;
let wsUnsubscribe: (() => void) | null = null;
let vadInterval: ReturnType<typeof setInterval> | null = null;
let ringInterval: ReturnType<typeof setInterval> | null = null;
let ringTimeout: ReturnType<typeof setTimeout> | null = null;

// ── Sound effects ─────────────────────────────────────────────────────────────

function playTone(freq: number, duration: number, volume = 0.25, delay = 0) {
	try {
		const ctx = new AudioContext();
		const osc = ctx.createOscillator();
		const env = ctx.createGain();
		osc.connect(env); env.connect(ctx.destination);
		osc.type = 'sine'; osc.frequency.value = freq;
		env.gain.setValueAtTime(0, ctx.currentTime + delay);
		env.gain.linearRampToValueAtTime(volume, ctx.currentTime + delay + 0.01);
		env.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + delay + duration);
		osc.start(ctx.currentTime + delay);
		osc.stop(ctx.currentTime + delay + duration);
		osc.onended = () => ctx.close();
	} catch {}
}
function playSelfJoinSound() { playTone(880, 0.18, 0.25, 0); playTone(1100, 0.22, 0.2, 0.14); }
function playPeerJoinSound() { playTone(1000, 0.14, 0.18); }
function playPeerLeaveSound() { playTone(600, 0.14, 0.15); }
function playSelfLeaveSound() { playTone(1100, 0.15, 0.2, 0); playTone(880, 0.2, 0.18, 0.13); }
function playRingTone() {
	playTone(880, 0.15, 0.2, 0);
	playTone(880, 0.15, 0.2, 0.2);
}
function playIncomingRing() {
	playTone(1200, 0.12, 0.25, 0);
	playTone(1000, 0.12, 0.2, 0.15);
	playTone(1200, 0.12, 0.25, 0.3);
}

// ── Ring management ───────────────────────────────────────────────────────────

function startOutgoingRing() {
	playRingTone();
	ringInterval = setInterval(playRingTone, 3000);
	ringTimeout = setTimeout(() => cancelCall(), 30000);
}

function startIncomingRing() {
	playIncomingRing();
	ringInterval = setInterval(playIncomingRing, 3000);
}

export function stopRinging() {
	if (ringInterval !== null) { clearInterval(ringInterval); ringInterval = null; }
	if (ringTimeout !== null) { clearTimeout(ringTimeout); ringTimeout = null; }
}

// ── Local VAD + remote speaking poll ─────────────────────────────────────────

function getRMS(analyser: AnalyserNode): number {
	const buf = new Uint8Array(analyser.fftSize);
	analyser.getByteTimeDomainData(buf);
	let sum = 0;
	for (const b of buf) { const n = (b - 128) / 128; sum += n * n; }
	return Math.sqrt(sum / buf.length);
}

function startLocalVAD() {
	vadInterval = setInterval(() => {
		const me = get(currentUser);
		if (!me || !localAnalyser) return;
		voiceState.update((s) => {
			const next = new Set(s.speakingUsers);
			if (getRMS(localAnalyser!) > 0.015) next.add(me.id);
			else next.delete(me.id);
			return { ...s, speakingUsers: next };
		});
	}, 80);
}

function stopLocalVAD() {
	if (vadInterval !== null) { clearInterval(vadInterval); vadInterval = null; }
}

// ── Participant helpers ───────────────────────────────────────────────────────

function participantToPeer(p: RemoteParticipant): VoicePeer {
	let avatarUrl = '';
	try { avatarUrl = JSON.parse(p.metadata ?? '{}').avatar_url ?? ''; } catch {}
	return { user_id: p.identity, display_name: p.name ?? p.identity, avatar_url: avatarUrl };
}

function peersFromRoom(): VoicePeer[] {
	if (!livekitRoom) return [];
	return [...livekitRoom.remoteParticipants.values()].map(participantToPeer);
}

// ── Shared LiveKit room setup ─────────────────────────────────────────────────

async function connectToRoom(livekitURL: string, livekitToken: string, chId: string | null) {
	livekitRoom = new Room({ adaptiveStream: true, dynacast: true });

	livekitRoom.on(RoomEvent.ParticipantConnected, (p: RemoteParticipant) => {
		const peer = participantToPeer(p);
		voiceState.update((s) => ({
			...s,
			peers: [...s.peers.filter((x) => x.user_id !== peer.user_id), peer],
		}));
		if (chId) {
			voiceParticipants.update((vp) => ({
				...vp,
				[chId]: [...(vp[chId] ?? []).filter((x) => x.user_id !== peer.user_id), peer],
			}));
		}
		playPeerJoinSound();
	});

	livekitRoom.on(RoomEvent.ParticipantDisconnected, (p: RemoteParticipant) => {
		voiceState.update((s) => ({
			...s,
			peers: s.peers.filter((x) => x.user_id !== p.identity),
			speakingUsers: new Set([...s.speakingUsers].filter((id) => id !== p.identity)),
		}));
		if (chId) {
			voiceParticipants.update((vp) => ({
				...vp,
				[chId]: (vp[chId] ?? []).filter((x) => x.user_id !== p.identity),
			}));
		}
		playPeerLeaveSound();
	});

	livekitRoom.on(RoomEvent.TrackSubscribed, (track, pub, participant) => {
		if (track.kind === Track.Kind.Audio) {
			const el = track.attach();
			el.id = `voice-peer-${participant.identity}`;
			el.style.display = 'none';
			document.body.appendChild(el);
			const vol = get(peerVolumesStore)[participant.identity];
			if (vol !== undefined) el.volume = Math.min(vol, 1);
		} else if (track.kind === Track.Kind.Video && pub.source !== Track.Source.ScreenShare) {
			const stream = new MediaStream([track.mediaStreamTrack]);
			remoteVideoStore.update((v) => ({ ...v, [participant.identity]: stream }));
		}
	});

	livekitRoom.on(RoomEvent.TrackUnsubscribed, (track, pub, participant) => {
		if (track.kind === Track.Kind.Audio) {
			track.detach().forEach((el) => el.remove());
		} else if (track.kind === Track.Kind.Video && pub.source !== Track.Source.ScreenShare) {
			remoteVideoStore.update((v) => {
				const next = { ...v };
				delete next[participant.identity];
				return next;
			});
		}
	});

	livekitRoom.on(RoomEvent.ActiveSpeakersChanged, (speakers) => {
		const ids = new Set(speakers.map((p) => p.identity));
		voiceState.update((s) => {
			const next = new Set(s.speakingUsers);
			for (const p of livekitRoom!.remoteParticipants.values()) {
				if (ids.has(p.identity)) next.add(p.identity);
				else next.delete(p.identity);
			}
			return { ...s, speakingUsers: next };
		});
	});

	livekitRoom.on(RoomEvent.Disconnected, () => {
		if (channelId || dmConvId) leaveVoice();
	});

	await livekitRoom.connect(livekitURL, livekitToken);
	await livekitRoom.localParticipant.publishTrack(localAudioTrack!);
}

async function acquireMic(): Promise<void> {
	const cur = get(voiceState);
	localStream = await navigator.mediaDevices.getUserMedia({
		audio: {
			echoCancellation: cur.echoCancellation,
			noiseSuppression: cur.noiseSuppression,
			autoGainControl: cur.autoGainControl,
		},
		video: false,
	});

	audioCtx = new AudioContext({ latencyHint: 'interactive', sampleRate: 48000 });
	await audioCtx.resume();
	const source = audioCtx.createMediaStreamSource(localStream);
	gainNode = audioCtx.createGain();
	gainNode.gain.value = cur.micGain;
	localAnalyser = audioCtx.createAnalyser();
	localAnalyser.fftSize = 256;
	source.connect(gainNode);
	source.connect(localAnalyser);
	const dest = audioCtx.createMediaStreamDestination();
	gainNode.connect(dest);
	processedStream = dest.stream;
	localAudioTrack = new LocalAudioTrack(processedStream.getAudioTracks()[0], undefined, false);
}

// ── Public: channel voice ─────────────────────────────────────────────────────

export async function joinVoice(chId: string, srvId: string): Promise<void> {
	if (channelId || dmConvId) leaveVoice();

	voiceState.update((s) => ({ ...s, connecting: true }));

	let livekitToken: string, livekitURL: string;
	try {
		const res = await fetch(`/api/voice/token?channel=${chId}`);
		if (!res.ok) throw new Error(await res.text());
		({ token: livekitToken, url: livekitURL } = await res.json());
	} catch (e: any) {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error(e.message ?? 'Failed to get voice token');
	}

	try {
		await acquireMic();
	} catch {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error('Microphone access denied');
	}

	channelId = chId;
	subChannelId = null;
	subChannelName = null;
	voiceState.update((s) => ({ ...s, channelId: chId, subChannelId: null, subChannelName: null, dmConvId: null, label: '', serverId: srvId }));

	startLocalVAD();
	socket.subscribe('channel:' + chId);
	wsUnsubscribe = socket.on(() => {});
	socket.send('voice.join', { channel_id: chId });

	await connectToRoom(livekitURL, livekitToken, chId);

	const initialPeers = peersFromRoom();
	voiceState.update((s) => ({ ...s, connecting: false, peers: initialPeers }));
	voiceParticipants.update((vp) => ({ ...vp, [chId]: initialPeers }));

	const me = get(currentUser);
	if (me) {
		const self: VoicePeer = { user_id: me.id, display_name: me.display_name, avatar_url: me.avatar_url ?? '' };
		voiceParticipants.update((vp) => ({
			...vp,
			[chId]: [...(vp[chId] ?? []).filter((x) => x.user_id !== me.id), self],
		}));
	}

	playSelfJoinSound();
}

// ── Public: voice sub-channels (breakout rooms) ───────────────────────────────

export function createVoiceSub(chId: string, name: string) {
	socket.send('voice.sub.create', { channel_id: chId, name });
}

export async function joinVoiceSub(chId: string, srvId: string, subId: string, name: string): Promise<void> {
	// Silent leave — no sound when moving between main channel and a sub.
	leaveVoice(true);

	voiceState.update((s) => ({ ...s, connecting: true }));

	let livekitToken: string, livekitURL: string;
	try {
		const res = await fetch(`/api/voice/token?channel=${chId}&sub=${subId}`);
		if (!res.ok) throw new Error(await res.text());
		({ token: livekitToken, url: livekitURL } = await res.json());
	} catch (e: any) {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error(e.message ?? 'Failed to get sub-channel token');
	}

	try {
		await acquireMic();
	} catch {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error('Microphone access denied');
	}

	channelId = chId;
	subChannelId = subId;
	subChannelName = name;
	voiceState.update((s) => ({
		...s, channelId: chId, subChannelId: subId, subChannelName: name,
		dmConvId: null, label: name, serverId: srvId,
	}));

	// Notify server we joined the sub (for participant tracking in voice.sub.state).
	socket.subscribe('channel:' + chId);
	wsUnsubscribe = socket.on(() => {});
	socket.send('voice.sub.join', { channel_id: chId, sub_id: subId });

	startLocalVAD();
	await connectToRoom(livekitURL, livekitToken, chId);

	const initialPeers = peersFromRoom();
	voiceState.update((s) => ({ ...s, connecting: false, peers: initialPeers }));
	playSelfJoinSound();
}

export function leaveVoiceSub(chId: string, subId: string, srvId: string) {
	const sid = subId;
	subChannelId = null;
	subChannelName = null;
	socket.send('voice.sub.leave', { channel_id: chId, sub_id: sid });
	// Disconnect LiveKit sub room, then rejoin the main channel.
	livekitRoom?.disconnect();
	livekitRoom = null;
	localAudioTrack?.stop();
	localAudioTrack = null;
	stopLocalVAD();
	// Rejoin main channel audio.
	joinVoice(chId, srvId);
}

// ── Public: DM call ───────────────────────────────────────────────────────────

export async function joinDMCall(convId: string, peerName: string, peerUserId: string): Promise<void> {
	if (channelId || dmConvId) leaveVoice();

	voiceState.update((s) => ({ ...s, connecting: true }));

	let livekitToken: string, livekitURL: string;
	try {
		const res = await fetch(`/api/voice/dm-token?conv=${convId}`);
		if (!res.ok) throw new Error(await res.text());
		({ token: livekitToken, url: livekitURL } = await res.json());
	} catch (e: any) {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error(e.message ?? 'Failed to get voice token');
	}

	try {
		await acquireMic();
	} catch {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error('Microphone access denied');
	}

	dmConvId = convId;
	dmPeerUserId = peerUserId;
	voiceState.update((s) => ({
		...s,
		channelId: null,
		dmConvId: convId,
		dmPeerUserId: peerUserId,
		label: peerName,
		serverId: null,
	}));

	startLocalVAD();
	await connectToRoom(livekitURL, livekitToken, null);

	const initialPeers = peersFromRoom();
	voiceState.update((s) => ({ ...s, connecting: false, peers: initialPeers }));
	playSelfJoinSound();
}

// ── Public: call signaling ────────────────────────────────────────────────────

export async function callFriend(userId: string, displayName: string, avatarUrl: string): Promise<void> {
	const cur = get(callState);
	if (cur.status !== 'idle') return;

	const conv = await api.getOrCreateDM(userId);
	callState.set({ status: 'ringing_out', convId: conv.id, peer: { userId, displayName, avatarUrl } });
	socket.send('call.ring', { to_user_id: userId, conv_id: conv.id });
	startOutgoingRing();
}

export function cancelCall(): void {
	const cur = get(callState);
	if (cur.status !== 'ringing_out' || !cur.peer) return;
	stopRinging();
	socket.send('call.cancel', { to_user_id: cur.peer.userId, conv_id: cur.convId });
	callState.set({ status: 'idle', convId: null, peer: null });
}

export async function acceptCall(): Promise<void> {
	const cur = get(callState);
	if (cur.status !== 'ringing_in' || !cur.peer || !cur.convId) return;
	stopRinging();
	const { convId, peer } = cur;
	callState.set({ status: 'idle', convId: null, peer: null });
	socket.send('call.accept', { conv_id: convId, caller_id: peer.userId });
	await joinDMCall(convId, peer.displayName, peer.userId);
}

export function declineCall(): void {
	const cur = get(callState);
	if (cur.status !== 'ringing_in' || !cur.peer) return;
	stopRinging();
	socket.send('call.decline', { conv_id: cur.convId, caller_id: cur.peer.userId });
	callState.set({ status: 'idle', convId: null, peer: null });
}

// Called when the OTHER party initiates (incoming call events handled in +page.svelte)
export function handleIncomingCall(convId: string, peer: { userId: string; displayName: string; avatarUrl: string }): void {
	const cur = get(callState);
	if (cur.status !== 'idle') {
		// Busy — auto-decline
		socket.send('call.decline', { conv_id: convId, caller_id: peer.userId });
		return;
	}
	callState.set({ status: 'ringing_in', convId, peer });
	startIncomingRing();
}

export function handleCallAccepted(convId: string, peerName: string, peerUserId: string): void {
	stopRinging();
	callState.set({ status: 'idle', convId: null, peer: null });
	joinDMCall(convId, peerName, peerUserId);
}

export function handleCallDeclined(): void {
	stopRinging();
	callState.set({ status: 'idle', convId: null, peer: null });
}

export function handleCallEnded(): void {
	stopRinging();
	callState.set({ status: 'idle', convId: null, peer: null });
	leaveVoice();
}

export function handleCallCancelled(): void {
	stopRinging();
	callState.set({ status: 'idle', convId: null, peer: null });
}

// ── Public: camera ────────────────────────────────────────────────────────────

export async function toggleCamera(): Promise<void> {
	if (!livekitRoom) return;
	const cur = get(voiceState);
	if (cur.cameraOn) {
		if (localVideoTrack) {
			await livekitRoom.localParticipant.unpublishTrack(localVideoTrack);
			localVideoTrack.stop();
			localVideoTrack = null;
		}
		localVideoStore.set(null);
		voiceState.update((s) => ({ ...s, cameraOn: false }));
	} else {
		try {
			localVideoTrack = await createLocalVideoTrack();
			await livekitRoom.localParticipant.publishTrack(localVideoTrack);
			localVideoStore.set(new MediaStream([localVideoTrack.mediaStreamTrack]));
			voiceState.update((s) => ({ ...s, cameraOn: true }));
		} catch {
			// Camera permission denied or no device available
		}
	}
}

// ── Public: leave ─────────────────────────────────────────────────────────────

export function leaveVoice(silent = false) {
	const wasDM = !!dmConvId;
	const convId = dmConvId;
	const peerUserId = dmPeerUserId;
	const ch = channelId;

	channelId = null;
	subChannelId = null;
	subChannelName = null;
	dmConvId = null;
	dmPeerUserId = null;

	if (!wasDM && ch) {
		if (!silent) playSelfLeaveSound();
		socket.send('voice.leave', { channel_id: ch });
		socket.unsubscribe('channel:' + ch);
		wsUnsubscribe?.();
		wsUnsubscribe = null;
	} else if (wasDM && peerUserId) {
		playSelfLeaveSound();
		socket.send('call.end', { conv_id: convId, other_user_id: peerUserId });
	}

	stopLocalVAD();
	localAudioTrack?.stop();
	localAudioTrack = null;
	if (localVideoTrack) {
		localVideoTrack.stop();
		localVideoTrack = null;
	}
	localVideoStore.set(null);
	remoteVideoStore.set({});
	livekitRoom?.disconnect();
	livekitRoom = null;

	localStream?.getTracks().forEach((t) => t.stop());
	processedStream?.getTracks().forEach((t) => t.stop());
	audioCtx?.close();
	localStream = null;
	processedStream = null;
	audioCtx = null;
	gainNode = null;
	localAnalyser = null;
	peerVolumesStore.set({});

	voiceState.update((s) => ({
		...DEFAULT_VOICE,
		echoCancellation: s.echoCancellation,
		noiseSuppression: s.noiseSuppression,
		autoGainControl: s.autoGainControl,
		micGain: s.micGain,
	}));
}

// ── Controls ──────────────────────────────────────────────────────────────────

export function toggleMute() {
	if (!localStream) return;
	const track = localStream.getAudioTracks()[0];
	if (!track) return;
	track.enabled = !track.enabled;
	voiceState.update((s) => ({ ...s, muted: !track.enabled }));
}

export function setMuted(muted: boolean) {
	if (!localStream) return;
	const track = localStream.getAudioTracks()[0];
	if (!track) return;
	track.enabled = !muted;
	voiceState.update((s) => ({ ...s, muted }));
}

export function setMicGain(value: number) {
	if (gainNode) gainNode.gain.value = value;
	voiceState.update((s) => ({ ...s, micGain: value }));
}

export function setPeerVolume(userId: string, value: number) {
	peerVolumesStore.update((m) => ({ ...m, [userId]: value }));
	const el = document.getElementById(`voice-peer-${userId}`) as HTMLAudioElement | null;
	if (el) el.volume = Math.min(value, 1);
}

export async function setEchoCancellation(value: boolean) {
	voiceState.update((s) => ({ ...s, echoCancellation: value }));
	await restartMic();
}

export async function setNoiseSuppression(value: boolean) {
	voiceState.update((s) => ({ ...s, noiseSuppression: value }));
	await restartMic();
}

export async function setAutoGainControl(value: boolean) {
	voiceState.update((s) => ({ ...s, autoGainControl: value }));
	await restartMic();
}

async function restartMic() {
	if (!livekitRoom || !audioCtx || !gainNode) return;
	const cur = get(voiceState);
	try {
		const newStream = await navigator.mediaDevices.getUserMedia({
			audio: {
				echoCancellation: cur.echoCancellation,
				noiseSuppression: cur.noiseSuppression,
				autoGainControl: cur.autoGainControl,
			},
			video: false,
		});
		const oldStream = localStream;
		localStream = newStream;
		const newSource = audioCtx.createMediaStreamSource(newStream);
		newSource.connect(gainNode);
		if (localAnalyser) newSource.connect(localAnalyser);
		if (localAudioTrack && processedStream) {
			const newTrack = new LocalAudioTrack(processedStream.getAudioTracks()[0], undefined, false);
			await livekitRoom.localParticipant.unpublishTrack(localAudioTrack);
			localAudioTrack.stop();
			localAudioTrack = newTrack;
			await livekitRoom.localParticipant.publishTrack(localAudioTrack);
		}
		oldStream?.getTracks().forEach((t) => t.stop());
	} catch {}
}

export function setVADThreshold(_value: number) {}

export async function toggleScreenShare() {
	if (!livekitRoom) return;
	const cur = get(voiceState);
	if (cur.screenSharing) {
		// Stop all screen tracks.
		for (const pub of livekitRoom.localParticipant.videoTrackPublications.values()) {
			if (pub.source === Track.Source.ScreenShare) {
				await livekitRoom.localParticipant.unpublishTrack(pub.videoTrack!);
				pub.videoTrack?.stop();
			}
		}
		voiceState.update((s) => ({ ...s, screenSharing: false }));
	} else {
		try {
			const tracks = await createLocalScreenTracks({ audio: false });
			for (const track of tracks) {
				await livekitRoom.localParticipant.publishTrack(track);
				track.mediaStreamTrack.addEventListener('ended', () => {
					livekitRoom?.localParticipant.unpublishTrack(track);
					voiceState.update((s) => ({ ...s, screenSharing: false }));
				});
			}
			voiceState.update((s) => ({ ...s, screenSharing: true }));
		} catch {
			// User cancelled screen pick or permission denied — no-op.
		}
	}
}
