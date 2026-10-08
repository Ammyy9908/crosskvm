const { execFileSync } = require('child_process');
const path = require('path');
module.exports = async context => {
 if (context.electronPlatformName !== 'darwin') return;
 const app = path.join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
 execFileSync('codesign', ['--verify', '--deep', '--strict', app], { stdio: 'inherit' });
 console.log(`Verified sealed macOS app: ${app}`);
};
