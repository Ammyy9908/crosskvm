// Publish only a completed Windows x64 desktop ZIP to the download directory.
const fs = require('fs');
const path = require('path');
module.exports = async ({ artifactPaths }) => {
  const zip = artifactPaths.find(file => /-windows-x64\.zip$/.test(file));
  if (!zip) return [];
  const destination = path.resolve(__dirname, '../../bin/CrossKVM-windows-x64.zip');
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.copyFileSync(zip, destination + '.tmp');
  fs.renameSync(destination + '.tmp', destination);
  console.log('Published Windows desktop ZIP: ' + destination);
  return [];
};
