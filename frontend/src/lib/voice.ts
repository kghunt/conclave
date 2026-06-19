import { get, writable } from 'svelte/store';
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
	vadThreshold: number;
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
	vadThreshold: 0.015,
};

export const voiceState = writable<VoiceState>({ ...DEFAULT_STATE });

// Per-peer output volume (userId → 0–2, default 1)
export const peerVolumesStore = writable<Record<string, number>>({});

// ── Module-level WebRTC/WebAudio state ────────────────────────────────────────

let channelId: string | null = null;
let localStream: MediaStream | null = null;
let processedStream: MediaStream | null = null;
let audioCtx: AudioContext | null = null;
let gainNode: GainNode | null = null;
let localAnalyser: AnalyserNode | null = null;

const pendingCandidates = new Map<string, RTCIceCandidateInit[]>();
const peerAnalysers = new Map<string, AnalyserNode>();
const peerVolumeMap = new Map<string, number>();

let wsUnsubscribe: (() => void) | null = null;
let vadInterval: ReturnType<typeof setInterval> | null = null;
let prevSpeaking = new Set<string>();

const FALLBACK_ICE: RTCConfiguration = {
	iceServers: [{ urls: 'stun:stun.l.google.com:19302' }, { urls: 'stun:stun1.l.google.com:19302' }]
};

let iceConfig: RTCConfiguration = FALLBACK_ICE;

async function fetchICEConfig(): Promise<RTCConfiguration> {
	try {
		const res = await fetch('/api/voice/config');
		if (!res.ok) return FALLBACK_ICE;
		const data = await res.json();
		if (Array.isArray(data.ice_servers) && data.ice_servers.length > 0) {
			return { iceServers: data.ice_servers };
		}
	} catch {}
	return FALLBACK_ICE;
}

// ── Sound effects ─────────────────────────────────────────────────────────────

function playTone(freq: number, duration: number, volume = 0.25, delay = 0) {
	try {
		const ctx = new AudioContext();
		const osc = ctx.createOscillator();
		const env = ctx.createGain();
		osc.connect(env);
		env.connect(ctx.destination);
		osc.type = 'sine';
		osc.frequency.value = freq;
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

// ── VAD ───────────────────────────────────────────────────────────────────────

function getRMS(analyser: AnalyserNode): number {
	const buf = new Uint8Array(analyser.fftSize);
	analyser.getByteTimeDomainData(buf);
	let sum = 0;
	for (const b of buf) {
		const n = (b - 128) / 128;
		sum += n * n;
	}
	return Math.sqrt(sum / buf.length);
}

function startVAD() {
	vadInterval = setInterval(() => {
		const speaking = new Set<string>();
		const me = get(currentUser);
		const threshold = get(voiceState).vadThreshold;

		if (localAnalyser && me) {
			if (getRMS(localAnalyser) > threshold) speaking.add(me.id);
		}
		for (const [uid, analyser] of peerAnalysers) {
			if (getRMS(analyser) > threshold) speaking.add(uid);
		}

		const changed =
			speaking.size !== prevSpeaking.size || [...speaking].some((id) => !prevSpeaking.has(id));
		if (changed) {
			prevSpeaking = speaking;
			voiceState.update((s) => ({ ...s, speakingUsers: new Set(speaking) }));
		}
	}, 80);
}

function stopVAD() {
	if (vadInterval !== null) { clearInterval(vadInterval); vadInterval = null; }
	prevSpeaking = new Set();
	voiceState.update((s) => ({ ...s, speakingUsers: new Set() }));
}

async function addPendingCandidates(peerId: string, pc: RTCPeerConnection) {
	for (const c of pendingCandidates.get(peerId) ?? []) {
		try { await pc.addIceCandidate(new RTCIceCandidate(c)); } catch {}
	}
	pendingCandidates.delete(peerId);
}

// ── Public API ────────────────────────────────────────────────────────────────

export async function joinVoice(chId: string, srvId: string): Promise<void> {
	if (channelId) leaveVoice();

	voiceState.update((s) => ({ ...s, connecting: true }));

	// Fetch ICE config (includes TURN if configured server-side) before touching media
	iceConfig = await fetchICEConfig();

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

	audioCtx = new AudioContext();
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

	channelId = chId;
	voiceState.update((s) => ({
		...s,
		channelId: chId,
		serverId: srvId,
		muted: false,
		connecting: true,
		peers: [],
	}));

	startVAD();
	socket.subscribe('channel:' + chId);

	wsUnsubscribe = socket.on((event) => {
		if (event.type === 'voice.state') {
			if (event.payload.channel_id !== channelId) return;
			handleVoiceState(event.payload.peers);
			voiceState.update((s) => ({ ...s, connecting: false, peers: event.payload.peers }));
			playSelfJoinSound();
		} else if (event.type === 'voice.joined') {
			if (event.payload.channel_id !== channelId) return;
			voiceState.update((s) => ({
				...s,
				peers: [...s.peers.filter((p) => p.user_id !== event.payload.user.user_id), event.payload.user]
			}));
			playPeerJoinSound();
		} else if (event.type === 'voice.left') {
			if (event.payload.channel_id !== channelId) return;
			cleanupRealPeer(event.payload.user_id);
			voiceState.update((s) => ({
				...s,
				peers: s.peers.filter((p) => p.user_id !== event.payload.user_id)
			}));
			playPeerLeaveSound();
		} else if (event.type === 'voice.signal') {
			if (event.payload.channel_id !== channelId) return;
			handleSignal(event.payload.from, event.payload.signal as unknown as IncomingSignal);
		}
	});

	socket.send('voice.join', { channel_id: chId });
}

async function handleVoiceState(peers: VoicePeer[]) {
	for (const peer of peers) {
		const pc = createRealPC(peer.user_id);
		try {
			const offer = await pc.createOffer();
			await pc.setLocalDescription(offer);
			socket.send('voice.signal', { channel_id: channelId, to: peer.user_id, signal: offer });
		} catch {}
	}
}

type IncomingSignal =
	| { type: 'offer'; sdp: string }
	| { type: 'answer'; sdp: string }
	| { type: 'candidate'; candidate: RTCIceCandidateInit };

async function handleSignal(fromId: string, signal: IncomingSignal) {
	if (!channelId) return;

	if (signal.type === 'offer') {
		let pc = realPeerMap.get(fromId);
		if (!pc) pc = createRealPC(fromId);
		try {
			await pc.setRemoteDescription(new RTCSessionDescription({ type: 'offer', sdp: signal.sdp }));
			await addPendingCandidates(fromId, pc);
			const answer = await pc.createAnswer();
			await pc.setLocalDescription(answer);
			socket.send('voice.signal', { channel_id: channelId, to: fromId, signal: answer });
		} catch {}
	} else if (signal.type === 'answer') {
		const pc = realPeerMap.get(fromId);
		if (pc && pc.signalingState !== 'stable') {
			try {
				await pc.setRemoteDescription(new RTCSessionDescription({ type: 'answer', sdp: signal.sdp }));
				await addPendingCandidates(fromId, pc);
			} catch {}
		}
	} else if (signal.type === 'candidate') {
		const pc = realPeerMap.get(fromId);
		if (pc && pc.remoteDescription) {
			try { await pc.addIceCandidate(new RTCIceCandidate(signal.candidate)); } catch {}
		} else {
			const buf = pendingCandidates.get(fromId) ?? [];
			buf.push(signal.candidate);
			pendingCandidates.set(fromId, buf);
		}
	}
}

export function leaveVoice() {
	if (!channelId) return;
	playSelfLeaveSound();
	const ch = channelId;
	socket.send('voice.leave', { channel_id: ch });
	socket.unsubscribe('channel:' + ch);
	wsUnsubscribe?.();
	wsUnsubscribe = null;
	stopVAD();
	cleanupAllRealPeers();
	localStream?.getTracks().forEach((t) => t.stop());
	processedStream?.getTracks().forEach((t) => t.stop());
	audioCtx?.close();
	localStream = null;
	processedStream = null;
	audioCtx = null;
	gainNode = null;
	localAnalyser = null;
	peerAnalysers.clear();
	peerVolumeMap.clear();
	peerVolumesStore.set({});
	channelId = null;
	voiceState.update((s) => ({
		...DEFAULT_STATE,
		echoCancellation: s.echoCancellation,
		noiseSuppression: s.noiseSuppression,
		autoGainControl: s.autoGainControl,
		vadThreshold: s.vadThreshold,
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
	peerVolumeMap.set(userId, value);
	peerVolumesStore.update((m) => ({ ...m, [userId]: value }));
	const el = document.getElementById(`voice-peer-${userId}`) as HTMLAudioElement | null;
	if (el) el.volume = value;
}

export function setVADThreshold(value: number) {
	voiceState.update((s) => ({ ...s, vadThreshold: value }));
}

async function applyAudioConstraints(ec: boolean, ns: boolean, agc: boolean) {
	if (!localStream) return;
	const track = localStream.getAudioTracks()[0];
	if (!track) return;
	try {
		await track.applyConstraints({ echoCancellation: ec, noiseSuppression: ns, autoGainControl: agc });
	} catch { /* browser may not support all constraints */ }
}

export function setEchoCancellation(value: boolean) {
	voiceState.update((s) => ({ ...s, echoCancellation: value }));
	const s = get(voiceState);
	applyAudioConstraints(value, s.noiseSuppression, s.autoGainControl);
}

export function setNoiseSuppression(value: boolean) {
	voiceState.update((s) => ({ ...s, noiseSuppression: value }));
	const s = get(voiceState);
	applyAudioConstraints(s.echoCancellation, value, s.autoGainControl);
}

export function setAutoGainControl(value: boolean) {
	voiceState.update((s) => ({ ...s, autoGainControl: value }));
	const s = get(voiceState);
	applyAudioConstraints(s.echoCancellation, s.noiseSuppression, value);
}

// ── Peer connections ──────────────────────────────────────────────────────────

const realPeerMap = new Map<string, RTCPeerConnection>();

function createRealPC(peerId: string): RTCPeerConnection {
	const pc = new RTCPeerConnection(iceConfig);

	pc.onicecandidate = (e) => {
		if (e.candidate && channelId) {
			socket.send('voice.signal', {
				channel_id: channelId,
				to: peerId,
				signal: { type: 'candidate', candidate: e.candidate }
			});
		}
	};

	pc.ontrack = (e) => {
		const stream = e.streams[0];
		let el = document.getElementById(`voice-peer-${peerId}`) as HTMLAudioElement | null;
		if (!el) {
			el = document.createElement('audio');
			el.id = `voice-peer-${peerId}`;
			el.autoplay = true;
			el.style.display = 'none';
			document.body.appendChild(el);
		}
		el.srcObject = stream;
		el.volume = peerVolumeMap.get(peerId) ?? 1;

		if (audioCtx) {
			try {
				const src = audioCtx.createMediaStreamSource(stream);
				const analyser = audioCtx.createAnalyser();
				analyser.fftSize = 256;
				src.connect(analyser);
				peerAnalysers.set(peerId, analyser);
			} catch {}
		}
	};

	pc.onconnectionstatechange = () => {
		if (pc.connectionState === 'failed' || pc.connectionState === 'closed') {
			cleanupRealPeer(peerId);
			// Remove the stuck peer from the participant display
			const ch = channelId;
			if (ch) {
				voiceParticipants.update((vp) => ({
					...vp,
					[ch]: (vp[ch] ?? []).filter((p) => p.user_id !== peerId)
				}));
				voiceState.update((s) => ({
					...s,
					peers: s.peers.filter((p) => p.user_id !== peerId)
				}));
			}
		}
	};

	realPeerMap.set(peerId, pc);

	if (processedStream) {
		for (const track of processedStream.getTracks()) {
			pc.addTrack(track, processedStream);
		}
	}

	return pc;
}

function cleanupRealPeer(peerId: string) {
	realPeerMap.get(peerId)?.close();
	realPeerMap.delete(peerId);
	pendingCandidates.delete(peerId);
	peerAnalysers.delete(peerId);
	document.getElementById(`voice-peer-${peerId}`)?.remove();
}

function cleanupAllRealPeers() {
	for (const id of [...realPeerMap.keys()]) cleanupRealPeer(id);
}
