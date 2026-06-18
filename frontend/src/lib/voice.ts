import { get, writable } from 'svelte/store';
import { socket } from './socket';
import type { VoicePeer } from './api';
import { voiceParticipants } from './stores';

export interface VoiceState {
	channelId: string | null;
	serverId: string | null;
	muted: boolean;
	connecting: boolean;
	peers: VoicePeer[];
}

export const voiceState = writable<VoiceState>({
	channelId: null,
	serverId: null,
	muted: false,
	connecting: false,
	peers: []
});

// Module-level WebRTC state (not reactive — kept in sync with voiceState store)
let channelId: string | null = null;
let localStream: MediaStream | null = null;
const peerConnections = new Map<string, RTCPeerConnection>();
const pendingCandidates = new Map<string, RTCIceCandidateInit[]>();
let wsUnsubscribe: (() => void) | null = null;

const ICE_SERVERS: RTCConfiguration = {
	iceServers: [{ urls: 'stun:stun.l.google.com:19302' }, { urls: 'stun:stun1.l.google.com:19302' }]
};

function createPC(peerId: string): RTCPeerConnection {
	const pc = new RTCPeerConnection(ICE_SERVERS);

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
		// Attach incoming audio stream to an audio element
		let el = document.getElementById(`voice-peer-${peerId}`) as HTMLAudioElement | null;
		if (!el) {
			el = document.createElement('audio');
			el.id = `voice-peer-${peerId}`;
			el.autoplay = true;
			el.style.display = 'none';
			document.body.appendChild(el);
		}
		el.srcObject = e.streams[0];
	};

	pc.onconnectionstatechange = () => {
		if (pc.connectionState === 'failed' || pc.connectionState === 'closed') {
			cleanupPeer(peerId);
		}
	};

	peerConnections.set(peerId, pc);

	// Add local tracks
	if (localStream) {
		for (const track of localStream.getTracks()) {
			pc.addTrack(track, localStream);
		}
	}

	return pc;
}

async function addPendingCandidates(peerId: string, pc: RTCPeerConnection) {
	const candidates = pendingCandidates.get(peerId) ?? [];
	for (const c of candidates) {
		try {
			await pc.addIceCandidate(new RTCIceCandidate(c));
		} catch {}
	}
	pendingCandidates.delete(peerId);
}

function cleanupPeer(peerId: string) {
	const pc = peerConnections.get(peerId);
	if (pc) {
		pc.close();
		peerConnections.delete(peerId);
	}
	pendingCandidates.delete(peerId);
	const el = document.getElementById(`voice-peer-${peerId}`);
	if (el) el.remove();
}

function cleanupAllPeers() {
	for (const peerId of [...peerConnections.keys()]) {
		cleanupPeer(peerId);
	}
}

export async function joinVoice(chId: string, srvId: string): Promise<void> {
	if (channelId) {
		// Already in a call — leave it first
		leaveVoice();
	}

	voiceState.update((s) => ({ ...s, connecting: true }));

	try {
		localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
	} catch (err) {
		voiceState.update((s) => ({ ...s, connecting: false }));
		throw new Error('Microphone access denied');
	}

	channelId = chId;
	voiceState.set({ channelId: chId, serverId: srvId, muted: false, connecting: true, peers: [] });

	// Listen for voice WS events
	wsUnsubscribe = socket.on((event) => {
		if (event.type === 'voice.state') {
			if (event.payload.channel_id !== channelId) return;
			// We just joined — initiate offers to all existing peers
			handleVoiceState(event.payload.peers);
			voiceState.update((s) => ({
				...s,
				connecting: false,
				peers: event.payload.peers
			}));
		} else if (event.type === 'voice.joined') {
			if (event.payload.channel_id !== channelId) return;
			// New peer joined — they'll send us an offer; just update participants display
			voiceState.update((s) => ({
				...s,
				peers: [...s.peers.filter((p) => p.user_id !== event.payload.user.user_id), event.payload.user]
			}));
		} else if (event.type === 'voice.left') {
			if (event.payload.channel_id !== channelId) return;
			cleanupPeer(event.payload.user_id);
			voiceState.update((s) => ({
				...s,
				peers: s.peers.filter((p) => p.user_id !== event.payload.user_id)
			}));
		} else if (event.type === 'voice.signal') {
			if (event.payload.channel_id !== channelId) return;
			handleSignal(event.payload.from, event.payload.signal as unknown as IncomingSignal);
		}
	});

	socket.send('voice.join', { channel_id: chId });
}

async function handleVoiceState(peers: VoicePeer[]) {
	// Initiating peer sends offers to all existing peers
	for (const peer of peers) {
		const pc = createPC(peer.user_id);
		try {
			const offer = await pc.createOffer();
			await pc.setLocalDescription(offer);
			socket.send('voice.signal', {
				channel_id: channelId,
				to: peer.user_id,
				signal: offer
			});
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
		let pc = peerConnections.get(fromId);
		if (!pc) {
			pc = createPC(fromId);
		}
		try {
			await pc.setRemoteDescription(new RTCSessionDescription({ type: 'offer', sdp: signal.sdp }));
			await addPendingCandidates(fromId, pc);
			const answer = await pc.createAnswer();
			await pc.setLocalDescription(answer);
			socket.send('voice.signal', { channel_id: channelId, to: fromId, signal: answer });
		} catch {}
	} else if (signal.type === 'answer') {
		const pc = peerConnections.get(fromId);
		if (pc && pc.signalingState !== 'stable') {
			try {
				await pc.setRemoteDescription(new RTCSessionDescription({ type: 'answer', sdp: signal.sdp }));
				await addPendingCandidates(fromId, pc);
			} catch {}
		}
	} else if (signal.type === 'candidate') {
		const pc = peerConnections.get(fromId);
		if (pc && pc.remoteDescription) {
			try {
				await pc.addIceCandidate(new RTCIceCandidate(signal.candidate));
			} catch {}
		} else {
			const buf = pendingCandidates.get(fromId) ?? [];
			buf.push(signal.candidate);
			pendingCandidates.set(fromId, buf);
		}
	}
}

export function leaveVoice() {
	if (!channelId) return;
	socket.send('voice.leave', { channel_id: channelId });
	wsUnsubscribe?.();
	wsUnsubscribe = null;
	cleanupAllPeers();
	localStream?.getTracks().forEach((t) => t.stop());
	localStream = null;
	channelId = null;
	voiceState.set({ channelId: null, serverId: null, muted: false, connecting: false, peers: [] });
}

export function toggleMute() {
	if (!localStream) return;
	const audioTrack = localStream.getAudioTracks()[0];
	if (!audioTrack) return;
	audioTrack.enabled = !audioTrack.enabled;
	voiceState.update((s) => ({ ...s, muted: !audioTrack.enabled }));
}
