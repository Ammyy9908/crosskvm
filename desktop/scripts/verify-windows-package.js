const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const asar = require('@electron/asar');
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const desktop = path.resolve(__dirname, '..');
const root = path.resolve(desktop, '..');
function verifyWindowsPackage(appOutDir, arch) {
  if (!['x64', 'arm64'].includes(arch)) throw new Error(`Unsupported Windows architecture: ${arch}`);
  const executable = fs.readFileSync(path.join(appOutDir, 'CrossKVM.exe'));
  const machine = executable.readUInt16LE(executable.readUInt32LE(0x3c) + 4);
  if (machine !== (arch === 'x64' ? 0x8664 : 0xaa64)) throw new Error('Windows runtime architecture mismatch');
  const archive = path.join(appOutDir, 'resources', 'app.asar');
  const files = ['src/main/main.js', 'src/main/ipc-client.js', 'src/main/clipboard-sync.js', 'src/main/clipboard-images.js', 'src/main/file-transfer.js', 'src/preload/preload.js', 'src/renderer/app.js', 'src/renderer/index.html', 'src/renderer/styles.css'];
  const hashes = {};
  for (const file of files) {
    const packaged = asar.extractFile(archive, file);
    if (!packaged.equals(fs.readFileSync(path.join(desktop, file)))) throw new Error(`Stale packaged source: ${file}`);
    hashes[file] = hash(packaged);
  }
  const daemon = `crosskvm_${arch === 'arm64' ? 'arm64' : 'amd64'}.exe`;
  const bytes = fs.readFileSync(path.join(appOutDir, 'resources/bin', daemon));
  if (!bytes.equals(fs.readFileSync(path.join(desktop, 'resources/bin', daemon)))) throw new Error(`Stale packaged daemon: ${daemon}`);
  if (bytes.readUInt16LE(bytes.readUInt32LE(0x3c) + 4) !== machine) throw new Error('Daemon architecture mismatch');
  hashes[daemon] = hash(bytes);
  const stop = fs.readFileSync(path.join(desktop, 'resources/bin/stop-crosskvm.bat'));
  if (!stop.equals(fs.readFileSync(path.join(appOutDir, 'resources/bin/stop-crosskvm.bat')))) throw new Error('Stale recovery script');
  fs.writeFileSync(path.join(appOutDir, 'stop-crosskvm.bat'), stop);
  fs.writeFileSync(path.join(appOutDir, 'build-info.json'), JSON.stringify({ platform: 'windows', arch, builtAt: new Date().toISOString(), hashes }, null, 2));
  console.log(`Verified current Windows ${arch} UI, daemon, architecture and recovery script`);
}
module.exports = async context => {
  if (context.electronPlatformName !== 'win32') return;
  const { Arch } = require('builder-util');
  verifyWindowsPackage(context.appOutDir, Arch[context.arch]);
};
module.exports.verifyWindowsPackage = verifyWindowsPackage;
