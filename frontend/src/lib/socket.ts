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
	| { type: 'typing'; payload: { user_id: string; display_name: string; room: string } };

type Handler = (event: WSEvent) => void;

class SocketClient {
	private ws: WebSocket | null = null;
	private handlers = new Set<Handler>();
	private rooms = new Set<string>();
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
		this.rooms.add(room);
		this.sendSubscribe(room);
	}

	unsubscribe(room: string) {
		this.rooms.delete(room);
		this.ws?.send(JSON.stringify({ type: 'unsubscribe', payload: { room } }));
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
