const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
if (process.platform === 'darwin') {
  const candidates = [process.env.CROSSKVM_NODE_INCLUDE, path.resolve(path.dirname(process.execPath), '../include/node'), '/opt/homebrew/include/node', '/usr/local/include/node'].filter(Boolean);
  const headers = candidates.find(p => fs.existsSync(path.join(p, 'node_api.h')));
  if (!headers) throw new Error('Node headers needed for macOS cursor module; set CROSSKVM_NODE_INCLUDE.');
  execFileSync('clang', ['-bundle', '-undefined', 'dynamic_lookup', '-DNAPI_VERSION=8', '-I', headers, '-framework', 'ApplicationServices', path.resolve(__dirname, '../native/cursor.c'), '-o', path.resolve(__dirname, '../native/cursor.node')], { stdio: 'inherit' });
}
