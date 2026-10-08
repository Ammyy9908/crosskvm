const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const root = path.resolve(__dirname, '../..');
for (const arch of ['amd64', 'arm64']) {
  const output = path.join(root, 'desktop/resources/bin', `crosskvm_${arch}.exe`);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  execFileSync('go', ['build', '-o', output, './cmd/crosskvm'], {
    cwd: root, stdio: 'inherit', env: { ...process.env, GOOS: 'windows', GOARCH: arch, CGO_ENABLED: '0' },
  });
  console.log(`Built current Windows daemon: ${output}`);
}
