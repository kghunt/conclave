import { get, writable } from 'svelte/store';
import {
	Room,
	RoomEvent,
	ParticipantEvent,
	Track,
	type RemoteParticipant,
	LocalAudioTrack,
} from 'livekit-client';
import { socket } from './socket';
import type { VoicePeer } from './api';
import { voiceParticipants, currentUser } from './stores';

export interface VoiceState {
	channelId: string | null;
	serverId: string | null;
	muted: boolean;
	connecting: boolean;
	peers: VoicePeer[];
	micGain: number;
	speakingUsers: Set<string>;
	echoCancellation: boolean;
	noiseSuppression: boolean;
	autoGainControl: boolean;
}

const DEFAULT_STATE: VoiceState = {
	channelId: null,
	serverId: null,
	muted: false,
	connecting: false,
	peers: [],
	micGain: 1,
	speakingUsers: new Set(),
	echoCancellation: true,
	noiseSuppression: true,
	autoGainControl: true,
};

export const voiceState = writable<VoiceState>({ ...DEFAULT_STATE });
export const peerVolumesStore = writable<Record<string, number>>({});

// ── Module-level state ────────────────────────────────────────────────────────

let livekitRoom: Room | null = null;
let localStream: MediaStream | null = null;   // raw getUserMedia stream
let processedStream: MediaStream | null = null; // after WebAudio gain node
let audioCtx: AudioContext | null = null;
let gainNode: GainNode | null = null;
let localAnalyser: AnalyserNode | null = null;
let localAudioTrack: LocalAudioTrack | null = null;
let channelId: string | null = null;
let wsUnsubscribe: (() => void) | null = null;
let vadInterval: ReturnType<typeof setInterval> | null = null;
let prevSpeaking = new Set<string>();

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

// ── Local VAD (for self speaking indicator) ───────────────────────────────────

function getRMS(analyser: AnalyserNode): number {
	const buf = new Uint8Array(analyser.fftSize);
	analyser.getByteTimeDomainData(buf);
	let sum = 0;
	for (const b of buf) { const n = (b - 128) / 128; sum += n * n; }
	return Math.sqrt(sum / buf.length);
}

function startLocalVAD() {
	vadInterval = setInterval(() => {
		if (!localAnalyser) return;
		const me = get(currentUser);
		if (!me) return;
		const speaking = getRMS(localAnalyser) > 0.015;
		voiceState.update((s) => {
			const next = new Set(s.speakingUsers);
			if (speaking) next.add(me.id); else next.delete(me.id);
			return { ...s, speakingUsers: next };
		});
	}, 80);
}

function stopLocalVAD() {
	if (vadInterval !== null) { clearInterval(vadInterval); vadInterval = null; }
	prevSpeaking = new Set();
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

// ── Public API ────────────────────────────────────────────────────────────────

export async function joinVoice(chId: string, srvId: string): Promise<void> {
	if (channelId) leaveVoice();

	voiceState.update((s) => ({ ...s, connecting: true }));

	// Fetch LiveKit token from our backend
	let livekitToken: string, livekitURL: string;
	try {
		const res = await fetch(`/api/voice/token?channel=${chId}`);
		if (!res.ok) throw new Error(await res.text());
		({ token: livekitToken, url: livekitURL } = await res.json());
	} catch (e: any) {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error(e.message ?? 'Failed to get voice token');
	}

	// Capture mic and build WebAudio gain pipeline so the gain slider still works
	const cur = get(voiceState);
	try {
		localStream = await navigator.mediaDevices.getUserMedia({
			audio: {
				echoCancellation: cur.echoCancellation,
				noiseSuppression: cur.noiseSuppression,
				autoGainControl: cur.autoGainControl,
			},
			video: false,
		});
	} catch {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error('Microphone access denied');
	}

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

	// Wrap the processed track as a LiveKit LocalAudioTrack
	localAudioTrack = new LocalAudioTrack(processedStream.getAudioTracks()[0], undefined, false);

	// Create and configure the LiveKit room
	livekitRoom = new Room({ adaptiveStream: true, dynacast: true });

	function attachSpeakingListener(p: RemoteParticipant) {
		p.on(ParticipantEvent.IsSpeakingChanged, (speaking: boolean) => {
			voiceState.update((s) => {
				const next = new Set(s.speakingUsers);
				if (speaking) next.add(p.identity); else next.delete(p.identity);
				return { ...s, speakingUsers: next };
			});
		});
	}

	livekitRoom.on(RoomEvent.ParticipantConnected, (p: RemoteParticipant) => {
		const peer = participantToPeer(p);
		voiceState.update((s) => ({
			...s,
			peers: [...s.peers.filter((x) => x.user_id !== peer.user_id), peer],
		}));
		voiceParticipants.update((vp) => ({
			...vp,
			[chId]: [...(vp[chId] ?? []).filter((x) => x.user_id !== peer.user_id), peer],
		}));
		attachSpeakingListener(p);
		playPeerJoinSound();
	});

	livekitRoom.on(RoomEvent.ParticipantDisconnected, (p: RemoteParticipant) => {
		voiceState.update((s) => ({
			...s,
			peers: s.peers.filter((x) => x.user_id !== p.identity),
			speakingUsers: new Set([...s.speakingUsers].filter((id) => id !== p.identity)),
		}));
		voiceParticipants.update((vp) => ({
			...vp,
			[chId]: (vp[chId] ?? []).filter((x) => x.user_id !== p.identity),
		}));
		playPeerLeaveSound();
	});

	livekitRoom.on(RoomEvent.TrackSubscribed, (track, _pub, participant) => {
		if (track.kind !== Track.Kind.Audio) return;
		const el = track.attach();
		el.id = `voice-peer-${participant.identity}`;
		el.style.display = 'none';
		document.body.appendChild(el);
		const vol = get(peerVolumesStore)[participant.identity];
		if (vol !== undefined) el.volume = Math.min(vol, 1);
	});

	livekitRoom.on(RoomEvent.TrackUnsubscribed, (track) => {
		track.detach().forEach((el) => el.remove());
	});

	livekitRoom.on(RoomEvent.Disconnected, () => {
		if (channelId) leaveVoice();
	});

	channelId = chId;
	voiceState.update((s) => ({ ...s, channelId: chId, serverId: srvId }));

	startLocalVAD();
	socket.subscribe('channel:' + chId);

	// Keep WS voice.join so observers (not in the call) see participant display update
	wsUnsubscribe = socket.on(() => {});
	socket.send('voice.join', { channel_id: chId });

	await livekitRoom.connect(livekitURL, livekitToken);
	await livekitRoom.localParticipant.publishTrack(localAudioTrack);

	// Attach speaking listeners to participants already in the room
	for (const p of livekitRoom.remoteParticipants.values()) {
		attachSpeakingListener(p);
	}

	// Populate initial peer list from room state
	const initialPeers = peersFromRoom();
	voiceState.update((s) => ({ ...s, connecting: false, peers: initialPeers }));
	voiceParticipants.update((vp) => ({ ...vp, [chId]: initialPeers }));

	// Add self to voiceParticipants for the sidebar
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

export function leaveVoice() {
	if (!channelId) return;
	playSelfLeaveSound();
	const ch = channelId;
	channelId = null;

	socket.send('voice.leave', { channel_id: ch });
	socket.unsubscribe('channel:' + ch);
	wsUnsubscribe?.();
	wsUnsubscribe = null;

	stopLocalVAD();

	localAudioTrack?.stop();
	localAudioTrack = null;
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
		...DEFAULT_STATE,
		echoCancellation: s.echoCancellation,
		noiseSuppression: s.noiseSuppression,
		autoGainControl: s.autoGainControl,
		micGain: s.micGain,
	}));
}

export function toggleMute() {
	if (!localStream) return;
	const track = localStream.getAudioTracks()[0];
	if (!track) return;
	track.enabled = !track.enabled;
	voiceState.update((s) => ({ ...s, muted: !track.enabled }));
}

export function setMicGain(value: number) {
	if (gainNode) gainNode.gain.value = value;
	voiceState.update((s) => ({ ...s, micGain: value }));
}

export function setPeerVolume(userId: string, value: number) {
	peerVolumesStore.update((m) => ({ ...m, [userId]: value }));
	const el = document.getElementById(`voice-peer-${userId}`) as HTMLAudioElement | null;
	// Clamp to 0–1 for the HTML audio element; values >1 can't be boosted here
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

		// Reconnect WebAudio pipeline to new stream
		const newSource = audioCtx.createMediaStreamSource(newStream);
		newSource.connect(gainNode);
		if (localAnalyser) newSource.connect(localAnalyser);

		// Replace the published track
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

// Unused exports kept for type compatibility with VoicePanel
export function setVADThreshold(_value: number) {}
