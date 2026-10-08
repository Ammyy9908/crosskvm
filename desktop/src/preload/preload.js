const { contextBridge, ipcRenderer, webUtils } = require('electron');

contextBridge.exposeInMainWorld('crosskvm', {
  getClipboardSettings: () => ipcRenderer.invoke('crosskvm:getClipboardSettings'),
  setClipboardEnabled: (enabled) => ipcRenderer.invoke('crosskvm:setClipboardEnabled', enabled),
  filePath: file => webUtils.getPathForFile(file),
  sendFiles: paths => ipcRenderer.invoke('crosskvm:sendFiles', paths),
  chooseFiles: () => ipcRenderer.invoke('crosskvm:chooseFiles'),
  getDaemonStatus: () => ipcRenderer.invoke('crosskvm:getDaemonStatus'),
  getLocalInfo: () => ipcRenderer.invoke('crosskvm:getLocalInfo'),
  getPeers: () => ipcRenderer.invoke('crosskvm:getPeers'),
  rescan: () => ipcRenderer.invoke('crosskvm:rescan'),
  getStatus: () => ipcRenderer.invoke('crosskvm:getStatus'),
  getMetrics: () => ipcRenderer.invoke('crosskvm:getMetrics'),

  connect: (params) => ipcRenderer.invoke('crosskvm:connect', params),
  disconnect: () => ipcRenderer.invoke('crosskvm:disconnect'),

  startKVM: (params) => ipcRenderer.invoke('crosskvm:startKVM', params),
  stopKVM: () => ipcRenderer.invoke('crosskvm:stopKVM'),

  setPeerSide: (side) => ipcRenderer.invoke('crosskvm:setPeerSide', { side }),

  onDaemonStatus: (callback) => {
    const handler = (e, status) => callback(status);
    ipcRenderer.on('crosskvm:daemon_status', handler);
    return () => ipcRenderer.removeListener('crosskvm:daemon_status', handler);
  },

  onEvent: (callback) => {
    const handler = (e, eventPayload) => callback(eventPayload);
    ipcRenderer.on('crosskvm:event', handler);
    return () => ipcRenderer.removeListener('crosskvm:event', handler);
  },
});
