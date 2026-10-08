const net = require('net');
const EventEmitter = require('events');
const os = require('os');
const path = require('path');

class DaemonIPCClient extends EventEmitter {
  constructor(options = {}) {
    super();
    this.options = options;
    this.tcpPort = options.tcpPort || 4548;
    this.isWindows = process.platform === 'win32';

    // Candidate connection endpoints in priority order
    this.targets = [];
    if (!this.isWindows) {
      try {
        this.targets.push(path.join(os.homedir(), '.crosskvm', 'crosskvm.sock'));
      } catch (_) {}
      this.targets.push('/tmp/crosskvm.sock');
    }
    this.targets.push({ host: '127.0.0.1', port: this.tcpPort });

    this.currentTargetIndex = 0;
    this.socket = null;
    this.connected = false;
    this.connecting = false;
    this.seq = 0;
    this.pending = new Map();
    this.buffer = '';
    this.reconnectTimer = null;
    this.destroyed = false;
  }

  connect() {
    if (this.destroyed || this.connected || this.connecting) return;
    this._clearReconnect();
    this.connecting = true;

    const target = this.targets[this.currentTargetIndex % this.targets.length];
    this.currentTargetIndex++;

    const sock = net.connect(target);
    this.socket = sock;
    sock.setEncoding('utf8');

    sock.on('connect', () => {
      if (this.socket !== sock) return;
      this.connecting = false;
      this.connected = true;
      this.buffer = '';
      this.emit('connected');
    });

    sock.on('data', (chunk) => {
      if (this.socket !== sock) return;
      this.buffer += chunk;
      let boundaryIndex;
      while ((boundaryIndex = this.buffer.indexOf('\n')) !== -1) {
        const line = this.buffer.slice(0, boundaryIndex).trim();
        this.buffer = this.buffer.slice(boundaryIndex + 1);
        if (line) {
          this._handleLine(line);
        }
      }
    });

    sock.on('error', () => {
      // Ignored here; 'close' handles retry
    });

    sock.on('close', () => {
      if (this.socket !== sock) return;
      const wasConnected = this.connected;
      this.connecting = false;
      this.connected = false;
      this.socket = null;
      this._rejectAllPending('Daemon connection closed');

      if (wasConnected) {
        this.emit('disconnected');
      }

      if (!this.destroyed) {
        this._scheduleReconnect();
      }
    });
  }

  _scheduleReconnect() {
    this._clearReconnect();
    this.reconnectTimer = setTimeout(() => {
      if (!this.destroyed && !this.connected) {
        this.connect();
      }
    }, 1000);
  }

  _clearReconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  _handleLine(line) {
    try {
      const msg = JSON.parse(line);
      // RPC Response
      if (msg.id !== undefined && this.pending.has(msg.id)) {
        const { resolve, reject } = this.pending.get(msg.id);
        this.pending.delete(msg.id);
        if (msg.error) {
          reject(new Error(msg.error));
        } else {
          resolve(msg.result);
        }
        return;
      }

      // Asynchronous Daemon Event
      if (msg.event) {
        this.emit('daemon_event', { event: msg.event, data: msg.data });
      }
    } catch (e) {
      console.error('[IPC Client] Invalid daemon response:', e.message);
    }
  }

  request(method, params = {}) {
    return new Promise((resolve, reject) => {
      // If momentarily connecting, wait up to 4 seconds
      if (!this.connected) {
        let timeout;
        const onConn = () => {
          clearTimeout(timeout);
          this.removeListener('connected', onConn);
          this.request(method, params).then(resolve).catch(reject);
        };
        timeout = setTimeout(() => {
          this.removeListener('connected', onConn);
          reject(new Error('Daemon not connected'));
        }, 4000);

        this.once('connected', onConn);
        return;
      }

      if (!this.socket) {
        return reject(new Error('Daemon socket unavailable'));
      }

      const id = ++this.seq;
      this.pending.set(id, { resolve, reject });

      const payload = JSON.stringify({ id, method, params }) + '\n';
      this.socket.write(payload, 'utf8', (err) => {
        if (err) {
          this.pending.delete(id);
          reject(err);
        }
      });
    });
  }

  _rejectAllPending(reason) {
    for (const [id, { reject }] of this.pending) {
      reject(new Error(reason));
    }
    this.pending.clear();
  }

  close() {
    this.destroyed = true;
    this._clearReconnect();
    if (this.socket) {
      this.socket.destroy();
      this.socket = null;
    }
  }
}

module.exports = DaemonIPCClient;
