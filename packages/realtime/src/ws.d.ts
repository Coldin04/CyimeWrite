declare module 'ws' {
	import type { EventEmitter } from 'node:events';
	import type { IncomingMessage } from 'node:http';

	export class WebSocket extends (EventEmitter as { new (): EventEmitter }) {
		static readonly OPEN: number;
		readyState: number;
		send(data: string): void;
		close(code?: number, reason?: string): void;
		on(event: 'message', listener: (data: Buffer) => void): this;
		on(event: 'error', listener: (error: Error) => void): this;
		on(event: 'close', listener: () => void): this;
	}

	export class WebSocketServer extends (EventEmitter as { new (): EventEmitter }) {
		constructor(options?: { noServer?: boolean; maxPayload?: number });
		handleUpgrade(
			request: IncomingMessage,
			socket: unknown,
			head: Buffer,
			callback: (ws: WebSocket) => void
		): void;
	}
}
