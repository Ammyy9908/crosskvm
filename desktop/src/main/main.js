const { app, BrowserWindow, ipcMain, globalShortcut } = require('electron');
const path = require('path');
const { spawn } = require('child_process');
const fs = require('fs');
const DaemonIPCClient = require('./ipc-client');

let cursorControl = null;
if (process.platform === 'darwin') {
  try { cursorControl = require('../../native/cursor.node'); }
  catch (err) { console.error('[Cursor] Native cursor module unavailable:', err.message); }
}
function setRemoteCursor(remote) {
  if (!cursorControl) return;
  try { cursorControl.setRemote(remote); }
  catch (err) { console.error('[Cursor]', err.message); }
}

let mainWindow = null;
let daemonProcess = null;
let ipcClient = null;

function findDaemonBinary() {
  const isWin = process.platform === 'win32';
  const binName = isWin ? 'crosskvm_amd64.exe' : 'crosskvm';

  // Candidate paths covering packaged app, resources, and dev tree
  const candidates = [
    path.join(process.resourcesPath || '', 'bin', binName),
    path.join(process.resourcesPath || '', binName),
    path.join(__dirname, '../../../bin', binName),
    path.join(__dirname, '../../bin', binName),
    path.join(__dirname, '../bin', binName),
    path.join(process.cwd(), 'bin', binName),
    path.join(process.cwd(), binName),
  ];

  for (const p of candidates) {
    if (fs.existsSync(p)) {
      return p;
    }
  }
  return null;
}

function ensureDaemonRunning() {
  const binPath = findDaemonBinary();
  if (!binPath) {
    console.warn('[Main] Daemon binary not found, assuming external crosskvm daemon is already running.');
    return;
  }

  // Ensure execution permission on Unix systems
  if (process.platform !== 'win32') {
    try {
      fs.chmodSync(binPath, 0o755);
    } catch (_) {}
  }

  const logDir = path.join(app.getPath('userData'), 'logs');
  try {
    fs.mkdirSync(logDir, { recursive: true });
  } catch (_) {}
  const logFile = path.join(logDir, 'daemon.log');

  console.log(`[Main] Launching Go Daemon from: ${binPath} (logging to ${logFile})`);

  try {
    const outLog = fs.openSync(logFile, 'a');
    daemonProcess = spawn(binPath, ['daemon'], {
      stdio: ['ignore', outLog, outLog],
      detached: false,
      windowsHide: true,
      env: { ...process.env, ...(cursorControl ? { CROSSKVM_DESKTOP_CURSOR: '1' } : {}) },
    });

    daemonProcess.on('error', (err) => {
      console.error('[Main] Daemon process spawn error:', err);
    });

    daemonProcess.on('exit', (code, signal) => {
      setRemoteCursor(false);
      console.log(`[Main] Daemon exited with code ${code}, signal ${signal}`);
      daemonProcess = null;
    });
  } catch (err) {
    console.error('[Main] Failed to spawn daemon process:', err);
  }
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 900,
    height: 720,
    minWidth: 800,
    minHeight: 620,
    title: 'CrossKVM',
    backgroundColor: '#0d1117',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    webPreferences: {
      preload: path.join(__dirname, '../preload/preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  mainWindow.loadFile(path.join(__dirname, '../renderer/index.html'));

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

function setupIPC() {
  ipcClient = new DaemonIPCClient();

  ipcClient.on('connected', () => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send('crosskvm:daemon_status', { connected: true });
    }
  });

  ipcClient.on('disconnected', () => {
    setRemoteCursor(false);
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send('crosskvm:daemon_status', { connected: false });
    }
  });

  ipcClient.on('daemon_event', (evt) => {
    if (evt.event === 'control_switched') setRemoteCursor(evt.data.state === 'remote');
    if (evt.event === 'disconnected') setRemoteCursor(false);
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send('crosskvm:event', evt);
    }
  });

  // RPC method handlers from Renderer
  ipcMain.handle('crosskvm:getDaemonStatus', () => {
    return { connected: ipcClient.connected };
  });

  ipcMain.handle('crosskvm:getLocalInfo', async () => {
    return await ipcClient.request('get_local_info');
  });

  ipcMain.handle('crosskvm:getPeers', async () => {
    return await ipcClient.request('get_peers');
  });

  ipcMain.handle('crosskvm:rescan', async () => {
    return await ipcClient.request('rescan');
  });

  ipcMain.handle('crosskvm:getStatus', async () => {
    return await ipcClient.request('get_status');
  });

  ipcMain.handle('crosskvm:getMetrics', async () => {
    return await ipcClient.request('get_metrics');
  });

  ipcMain.handle('crosskvm:connect', async (event, params) => {
    return await ipcClient.request('connect', params);
  });

  ipcMain.handle('crosskvm:disconnect', async () => {
    return await ipcClient.request('disconnect');
  });

  ipcMain.handle('crosskvm:startKVM', async (event, params) => {
    return await ipcClient.request('start_kvm', params);
  });

  ipcMain.handle('crosskvm:stopKVM', async () => {
    return await ipcClient.request('stop_kvm');
  });

  ipcMain.handle('crosskvm:setPeerSide', async (event, params) => {
    return await ipcClient.request('set_peer_side', params);
  });
}

const singleInstance = app.requestSingleInstanceLock();
if (!singleInstance) app.quit();
app.on('second-instance', () => {
 if (mainWindow) { if (mainWindow.isMinimized()) mainWindow.restore(); mainWindow.show(); mainWindow.focus(); }
});

app.whenReady().then(() => {
  if (!singleInstance) return;
  if (process.platform === 'win32') {
    globalShortcut.register('Control+Alt+Shift+Escape', () => {
      // Closing the peer prevents further injection; killing our child is a
      // bounded fallback if the daemon cannot process the recovery request.
      const child = daemonProcess;
      const timer = setTimeout(() => { if (child && child === daemonProcess) child.kill(); }, 1500);
      if (ipcClient && ipcClient.connected) {
        ipcClient.request('disconnect').then(() => clearTimeout(timer)).catch(() => {});
      }
    });
  }
  setupIPC();

  ensureDaemonRunning();
  setTimeout(() => {
    ipcClient.connect();
  }, 500);

  createWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('before-quit', async () => {
  globalShortcut.unregisterAll();
  setRemoteCursor(false);
  // Electron does not await async before-quit handlers. Terminate our daemon
  // before the first await so an unresponsive daemon cannot outlive the UI.
  if (daemonProcess) {
    try { daemonProcess.kill('SIGTERM'); } catch (_) {}
  }
  if (ipcClient && ipcClient.connected) {
    try { await ipcClient.request('stop_kvm'); } catch (_) {}
    ipcClient.close();
  }
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});
