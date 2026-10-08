const { createHash } = require('crypto');
const MAX_BYTES = 512 * 1024;
function validPNG(bytes) {
  return bytes.length >= 33 && bytes.length <= MAX_BYTES &&
    bytes.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex')) &&
    bytes.toString('ascii', 12, 16) === 'IHDR' &&
    bytes.readUInt32BE(16) > 0 && bytes.readUInt32BE(20) > 0 &&
    bytes.readUInt32BE(16) * bytes.readUInt32BE(20) <= 16000000;
}
function imageClipboard(clipboard, ClipboardItem, nativeImage, nativeClipboard = null) {
  function snapshot(bytes) {
    const key = 'image:' + createHash('sha256').update(bytes).digest('hex');
    if (!validPNG(bytes)) return { key, error: 'Image not shared: converted PNG must fit 512 KiB and 16 megapixels.' };
    return { key, png: bytes.toString('base64') };
  }
  return {
    async readImageSnapshot() {
      if (nativeClipboard) {
        try {
          const png = nativeClipboard.readPNG();
          if (png) return snapshot(png);
        } catch (err) {
          return { key: 'image-error:' + err.message, error: err.message };
        }
      }
      const items = await clipboard.read();
      for (const item of items) {
        const type = item.types.find(t => t === 'image/png') || item.types.find(t => t === 'image/jpeg');
        if (!type) continue;
        const blob = await item.getType(type);
        if (blob.size > MAX_BYTES) return { key: 'oversized-image', error: 'Image not shared: maximum size is 512 KiB.' };
        let bytes = Buffer.from(await blob.arrayBuffer());
        if (type === 'image/jpeg') {
          const image = nativeImage.createFromBuffer(bytes);
          const size = image.getSize();
          if (image.isEmpty() || size.width * size.height > 16000000) {
            return { key: 'invalid-jpeg', error: 'Image not shared: JPEG is invalid or exceeds 16 megapixels.' };
          }
          bytes = image.toPNG();
        }
        return snapshot(bytes);
      }
      const types = items.flatMap(item => item.types);
      if (types.some(type => /file-url|file-list|filenames|CF_HDROP/i.test(type))) {
        return { key: 'file-copy', error: 'A file was copied. Open it in Preview, select the image and copy; file transfer is not supported yet.' };
      }
      if (types.some(type => /image|tiff|bitmap/i.test(type))) {
        return { key: 'unsupported-image', error: 'Image not shared: this clipboard image format is unsupported.' };
      }
      return null;
    },
    async writePNG(png) {
      if (typeof png !== 'string' || png.length > Math.ceil(MAX_BYTES / 3) * 4) throw new Error('Invalid image');
      const bytes = Buffer.from(png, 'base64');
      if (!validPNG(bytes)) throw new Error('Invalid image');
      await clipboard.write([new ClipboardItem({ 'image/png': new Blob([bytes], { type: 'image/png' }) })]);
    }
  };
}
module.exports = { imageClipboard, validPNG };
