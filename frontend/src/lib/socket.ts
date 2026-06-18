import { writable } from 'svelte/store';
import type { Message, DirectMessage } from './api';

export type WSEvent =
	| { type: 'message.new'; payload: Message }
	| { type: 'message.edit'; payload: Message }
	| { type: 'message.delete'; payload: { id: string; channel_id: string } }
	| { type: 'dm.new'; payload: DirectMessage }
	| { type: 'dm.delete'; payload: { id: string; conversation_id: string } }
	| { type: 'member.join'; payload: { server_id: string; user_id: string } }
	| { type: 'member.leave'; payload: { server_id: string; user_id: string } }
	| { type: 'friend.accepted'; payload: { id: string; display_name: string; avatar_url: string } }
	| { type: 'mention.new'; payload: import('./api').Message }
	| { type: 'typing'; payload: { user_id: string; display_name: string; room: string } }
	| { type: 'presence.update'; payload: { user_id: string; status: string } }
	| { type: 'member.kicked'; payload: { server_id: string } }
	| { type: 'member.banned'; payload: { server_id: string } }
	| { type: 'join_request.new'; payload: { request_id: string; server_id: string; user: import('./api').User } }
	| { type: 'join_request.reviewed'; payload: { server_id: string; action: string } }
	| { type: 'voice.state'; payload: { channel_id: string; peers: import('./api').VoicePeer[] } }
	| { type: 'voice.joined'; payload: { channel_id: string; user: import('./api').VoicePeer } }
	| { type: 'voice.left'; payload: { channel_id: string; user_id: string } }
	| { type: 'voice.signal'; payload: { channel_id: string; from: string; signal: RTCSessionDescriptionInit | RTCIceCandidateInit } }
	| { type: 'thread.new'; payload: import('./api').Thread }
	| { type: 'thread.updated'; payload: import('./api').Thread }
	| { type: 'thread.message.new'; payload: import('./api').ThreadMessage }
	| { type: 'reaction.toggle'; payload: { message_id: string; channel_id: string; emoji: string; user_id: string; action: 'add' | 'remove' } }
	| { type: 'reaction.new'; payload: { message_id: string; channel_id: string; emoji: string; reactor_id: string } };

type Handler = (event: WSEvent) => void;

class SocketClient {
	private ws: WebSocket | null = null;
	private handlers = new Set<Handler>();
	private rooms = new Set<string>();
	private roomRefs = new Map<string, number>();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	connected = writable(false);

	connect() {
		if (this.ws?.readyState === WebSocket.OPEN) return;
		const proto = location.protocol === 'https:' ? 'wss' : 'ws';
		this.ws = new WebSocket(`${proto}://${location.host}/ws`);

		this.ws.onopen = () => {
			this.connected.set(true);
			// re-subscribe to all rooms after reconnect
			this.rooms.forEach((room) => this.sendSubscribe(room));
		};

		this.ws.onclose = () => {
			this.connected.set(false);
			this.reconnectTimer = setTimeout(() => this.connect(), 3000);
		};

		this.ws.onmessage = (e) => {
			try {
				const event = JSON.parse(e.data) as WSEvent;
				this.handlers.forEach((h) => h(event));
			} catch {}
		};
	}

	disconnect() {
		if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
		this.ws?.close();
		this.ws = null;
	}

	subscribe(room: string) {
		const count = (this.roomRefs.get(room) ?? 0) + 1;
		this.roomRefs.set(room, count);
		if (count === 1) {
			this.rooms.add(room);
			this.sendSubscribe(room);
		}
	}

	unsubscribe(room: string) {
		const count = (this.roomRefs.get(room) ?? 0) - 1;
		if (count <= 0) {
			this.roomRefs.delete(room);
			this.rooms.delete(room);
			this.ws?.send(JSON.stringify({ type: 'unsubscribe', payload: { room } }));
		} else {
			this.roomRefs.set(room, count);
		}
	}

	on(handler: Handler) {
		this.handlers.add(handler);
		return () => this.handlers.delete(handler);
	}

	send(type: string, payload: unknown) {
		if (this.ws?.readyState === WebSocket.OPEN) {
			this.ws.send(JSON.stringify({ type, payload }));
		}
	}

	private sendSubscribe(room: string) {
		if (this.ws?.readyState === WebSocket.OPEN) {
			this.ws.send(JSON.stringify({ type: 'subscribe', payload: { room } }));
		}
	}
}

export const socket = new SocketClient();
