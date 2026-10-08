const { execFileSync } = require('child_process');
const path = require('path');
if (process.platform !== 'darwin') throw new Error('Build the macOS native daemon on macOS');
const root = path.resolve(__dirname, '../..');
execFileSync('go', ['build', '-o', path.join(root, 'bin/crosskvm'), './cmd/crosskvm'], {
  cwd: root, stdio: 'inherit', env: { ...process.env, GOOS: 'darwin', GOARCH: process.arch === 'arm64' ? 'arm64' : 'amd64', CGO_ENABLED: '1' },
});
