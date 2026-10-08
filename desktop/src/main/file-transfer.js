const fs = require('fs/promises');
const { constants } = require('fs');
const path = require('path');
const { randomUUID, createHash } = require('crypto');
const CHUNK = 48 * 1024;
const MAX_SIZE = 1024 * 1024 * 1024;

function safeName(value) {
  let name = String(value || '').split(/[\\/]/).pop().replace(/[\x00-\x1f<>:"|?*]/g, '_').replace(/[. ]+$/g, '').slice(0, 180);
  if (!name || /^\.+$/.test(name)) name = 'received-file';
  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(name)) name = '_' + name;
  return name;
}
class FileTransfer {
  constructor({send, downloads, notify, timeout = 30000}) {
    this.send = send; this.downloads = downloads; this.notify = notify;
    this.timeout = timeout; this.pending = new Map(); this.queue = Promise.resolve();
    this.incoming = null; this.busy = false; this.epoch = 0;
  }
  progress(name, direction, bytes, size, state, detail = '') {
    this.notify({name, direction, bytes, size, state, detail});
  }
  async request(packet) {
    const key = packet.id + ':' + packet.step;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(key); reject(new Error('Transfer timed out'));
      }, this.timeout);
      this.pending.set(key, {resolve, reject, timer});
      Promise.resolve().then(() => this.send(packet)).catch(err => {
        const waiter = this.pending.get(key);
        if (waiter) { clearTimeout(timer); this.pending.delete(key); reject(err); }
      });
    });
  }
  receive(packet) {
    if (!packet || typeof packet.id !== 'string') return Promise.resolve();
    if (packet.kind === 'ack' || packet.kind === 'error') {
      const key = packet.id + ':' + packet.step;
      const waiter = this.pending.get(key);
      if (waiter) {
        clearTimeout(waiter.timer); this.pending.delete(key);
        if (packet.kind === 'error') waiter.reject(new Error(String(packet.error || 'Peer rejected transfer')));
        else waiter.resolve(packet);
      }
      return Promise.resolve();
    }
    const epoch = this.epoch;
    this.queue = this.queue.then(async () => {
      if (epoch !== this.epoch) return;
      try { await this.handle(packet, epoch); }
      catch (err) {
        if (this.incoming?.id === packet.id) await this.cleanup();
        this.progress('', 'receive', 0, 0, 'failed', err.message);
        if (epoch === this.epoch) await this.send({kind:'error',id:packet.id,step:packet.step,error:err.message}).catch(()=>{});
      }
    }).catch(()=>{});
    return this.queue;
  }
  armTimeout() {
    const item = this.incoming;
    clearTimeout(item.timer);
    item.timer = setTimeout(() => {
      this.queue = this.queue.then(async () => {
        if (this.incoming === item) {
          await this.cleanup();
          this.progress(item.name,'receive',item.offset,item.size,'failed','Transfer timed out');
        }
      }).catch(()=>{});
    }, this.timeout);
  }
  async cleanup() {
    const item = this.incoming; this.incoming = null;
    if (!item) return;
    clearTimeout(item.timer);
    await item.file.close().catch(()=>{});
    await fs.unlink(item.temp).catch(()=>{});
  }
  async handle(p, epoch) {
    if (p.kind === 'cancel') {
      if (this.incoming?.id === p.id) await this.cleanup();
      return;
    }
    if (p.kind === 'begin') {
      if (this.incoming) throw new Error('Another file is being received');
      if (!Number.isSafeInteger(p.size) || p.size < 0 || p.size > MAX_SIZE || p.step !== 0) throw new Error('Files must be at most 1 GiB');
      if (typeof p.name !== 'string' || p.name.length > 1024) throw new Error('Invalid filename');
      await fs.mkdir(this.downloads, {recursive:true});
      const temp = path.join(this.downloads, '.crosskvm-' + randomUUID() + '.part');
      const file = await fs.open(temp, 'wx', 0o600);
      this.incoming = {id:p.id,name:safeName(p.name),size:p.size,offset:0,step:1,temp,file,hash:createHash('sha256')};
      if (epoch !== this.epoch) { await this.cleanup(); return; }
    } else {
      const item = this.incoming;
      if (!item || item.id !== p.id || p.step !== item.step) throw new Error('Unexpected file transfer packet');
      if (p.kind === 'chunk') {
        if (typeof p.data !== 'string' || p.data.length > CHUNK*4/3 || p.offset !== item.offset) throw new Error('Invalid file chunk');
        const bytes = Buffer.from(p.data,'base64');
        if (bytes.length === 0 || bytes.toString('base64') !== p.data || bytes.length > CHUNK || item.offset + bytes.length > item.size) throw new Error('Invalid file chunk size');
        let written = 0;
        while (written < bytes.length) {
          const result = await item.file.write(bytes,written,bytes.length-written,item.offset+written);
          if (!result.bytesWritten) throw new Error('Unable to write Downloads file');
          written += result.bytesWritten;
        }
        item.hash.update(bytes); item.offset += bytes.length; item.step++;
      } else if (p.kind === 'end') {
        if (item.offset !== item.size || p.sha256 !== item.hash.digest('hex')) throw new Error('File integrity check failed');
        clearTimeout(item.timer);
        await item.file.sync(); await item.file.close();
        if (epoch !== this.epoch) { await this.cleanup(); return; }
        const parsed = path.parse(item.name);
        let destination;
        for (let n=0;n<10000;n++) {
          destination = path.join(this.downloads,n ? parsed.name+' ('+n+')'+parsed.ext : item.name);
          try { await fs.copyFile(item.temp,destination,constants.COPYFILE_EXCL); break; }
          catch(err) { if (err.code !== 'EEXIST' || n === 9999) throw err; }
        }
        await fs.unlink(item.temp);
        this.incoming = null;
        this.progress(path.basename(destination),'receive',item.size,item.size,'complete','Saved to Downloads');
        await this.send({kind:'ack',id:p.id,step:p.step});
        return;
      } else throw new Error('Unknown file transfer operation');
    }
    if (epoch !== this.epoch) { await this.cleanup(); return; }
    const item = this.incoming;
    this.armTimeout();
    this.progress(item.name,'receive',item.offset,item.size,'transferring');
    await this.send({kind:'ack',id:p.id,step:p.step});
  }
  async sendFiles(paths) {
    if (this.busy) throw new Error('A file transfer is already in progress');
    if (!Array.isArray(paths) || !paths.length || paths.length > 100 || paths.some(p=>typeof p !== 'string')) throw new Error('Choose 1–100 files');
    this.busy = true;
    const epoch = this.epoch;
    try {
      for (const filename of paths) {
        const id = randomUUID(), name = path.basename(filename);
        let file, step = 0, offset = 0, size = 0;
        try {
          if (epoch !== this.epoch) throw new Error('Transfer disconnected');
          const initial = await fs.stat(filename);
          if (!initial.isFile()) throw new Error('Folders and special files are not supported');
          file = await fs.open(filename,'r');
          const stat = await file.stat(); size = stat.size;
          if (!stat.isFile() || size > MAX_SIZE) throw new Error('Choose regular files up to 1 GiB; folders are not supported');
          if (epoch !== this.epoch) throw new Error('Transfer disconnected');
          await this.request({kind:'begin',id,step:step++,name,size});
          const hash = createHash('sha256'), buffer = Buffer.alloc(CHUNK);
          while (offset < size) {
            if (epoch !== this.epoch) throw new Error('Transfer disconnected');
            const {bytesRead} = await file.read(buffer,0,Math.min(CHUNK,size-offset),offset);
            if (!bytesRead) throw new Error('Source file changed during transfer');
            const bytes = buffer.subarray(0,bytesRead);
            hash.update(bytes);
            await this.request({kind:'chunk',id,step:step++,offset,data:bytes.toString('base64')});
            offset += bytesRead;
            this.progress(name,'send',offset,size,'transferring');
          }
          if (epoch !== this.epoch) throw new Error('Transfer disconnected');
          await this.request({kind:'end',id,step:step++,sha256:hash.digest('hex')});
          this.progress(name,'send',size,size,'complete','Saved to peer Downloads');
        } catch(err) {
          if (epoch === this.epoch) await this.send({kind:'cancel',id,step}).catch(()=>{});
          this.progress(name,'send',offset,size,'failed',err.message);
          throw err;
        } finally { if (file) await file.close().catch(()=>{}); }
      }
    } finally { this.busy = false; }
  }
  reset() {
    this.epoch++;
    for (const waiter of this.pending.values()) {clearTimeout(waiter.timer);waiter.reject(new Error('Peer disconnected'));}
    this.pending.clear();
    this.queue = this.queue.then(()=>this.cleanup()).catch(()=>{});
    return this.queue;
  }
}
module.exports = {FileTransfer,safeName};
