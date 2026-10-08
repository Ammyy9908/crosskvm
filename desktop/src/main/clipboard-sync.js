// Serialize native clipboard operations, supporting both sync and async Electron APIs.
class ClipboardSync {
  constructor(clipboard, send, images = null, sendImage = null, notice = () => {}) {
    this.images = images;
    this.sendImage = sendImage;
    this.notice = notice;
    this.imagesEnabled = false;
    this.clipboard = clipboard;
    this.send = send;
    this.enabled = false;
    this.epoch = 0;
    this.last = '';
    this.queue = Promise.resolve();
    this.polling = false;
  }
  setImagesEnabled(enabled) {
    if (this.imagesEnabled === enabled) return;
    this.imagesEnabled = enabled;
    if (this.enabled) { this.setEnabled(false); this.setEnabled(true); }
  }
  async snapshot() {
    if (this.imagesEnabled && this.images) {
      const image = await this.images.readImageSnapshot();
      if (image) return image;
    }
    const text = await this.clipboard.readText();
    return { key: 'text:' + text, text };
  }
  enqueue(work) {
    this.queue = this.queue.then(work).catch(() => { this.notice('Clipboard operation failed. Copy the image again; if it persists, restart both apps.'); });
    return this.queue;
  }
  setEnabled(enabled) {
    if (enabled === this.enabled) return this.queue;
    this.enabled = enabled;
    const epoch = ++this.epoch;
    if (!enabled) return this.queue;
    // Baseline only: never publish clipboard contents that predate the connection.
    return this.enqueue(async () => {
      const value = await this.snapshot();
      if (this.enabled && epoch === this.epoch) this.last = value.key;
    });
  }
  poll() {
    if (!this.enabled || this.polling) return this.queue;
    this.polling = true;
    const epoch = this.epoch;
    return this.enqueue(async () => {
      const value = await this.snapshot();
      if (!this.enabled || epoch !== this.epoch || value.key === this.last) return;
      this.last = value.key;
      if (value.error) { this.notice(value.error); return; }
      if (value.png) {
        Promise.resolve(this.sendImage(value.png)).catch(() => this.notice('Could not share image. Copy it again to retry.'));
        return;
      }
      const text = value.text;
      // Empty/non-text clipboard changes do not erase the remote clipboard.
      if (!text || text.includes('\0') || Buffer.byteLength(text, 'utf8') > 65536) return;
      // Network completion must not block applying incoming clipboard updates.
      Promise.resolve(this.send(text)).catch(() => {});
    }).finally(() => { this.polling = false; });
  }
  receiveImage(png) {
    const epoch = this.epoch;
    return this.enqueue(async () => {
      if (!this.enabled || !this.imagesEnabled || epoch !== this.epoch || !this.images) return;
      await this.images.writePNG(png);
      this.last = (await this.snapshot()).key;
    });
  }
  receive(text) {
    if (typeof text !== 'string' || !text || text.includes('\0') || Buffer.byteLength(text,'utf8') > 65536) return this.queue;
    const epoch = this.epoch;
    return this.enqueue(async () => {
      if (!this.enabled || epoch !== this.epoch || 'text:' + text === this.last) return;
      await this.clipboard.writeText(text);
      this.last = (await this.snapshot()).key; // Account for native newline normalization.
    });
  }
}
module.exports = ClipboardSync;
