// CrossKVM Desktop Renderer Application Logic

document.addEventListener('DOMContentLoaded', () => {
  // State
  let localInfo = null;
  let currentStatus = null;
  let knownPeers = [];
  let selectedPeerId = null;
  let layoutSaving = false;
  let layoutDrag = null;
  let layoutSide = 'right';
  const peerFirst = () => ['left', 'top'].includes(layoutSide);

  // DOM Elements
  const daemonBadge = document.getElementById('daemon-badge');
  const daemonStatusText = document.getElementById('daemon-status-text');

  const localDeviceName = document.getElementById('local-device-name');
  const localDeviceOS = document.getElementById('local-device-os');
  const localDeviceID = document.getElementById('local-device-id');
  const peersList = document.getElementById('peers-list');
  const btnRefreshPeers = document.getElementById('btn-refresh-peers');

  const layoutCanvas = document.getElementById('layout-canvas');
  const screenLeft = document.getElementById('screen-left');
  const screenRight = document.getElementById('screen-right');
  const screenLeftTitle = document.getElementById('screen-left-title');
  const screenRightTitle = document.getElementById('screen-right-title');
  const screenLeftSub = document.getElementById('screen-left-sub');
  const screenRightSub = document.getElementById('screen-right-sub');

  const sideLeftRadio = document.getElementById('side-left');
  const sideRightRadio = document.getElementById('side-right');

  const kvmConnState = document.getElementById('kvm-conn-state');
  const kvmFocusState = document.getElementById('kvm-focus-state');
  const kvmLatencyVal = document.getElementById('kvm-latency-val');

  const btnStartKVM = document.getElementById('btn-start-kvm');
  const btnStopKVM = document.getElementById('btn-stop-kvm');
  const btnDisconnect = document.getElementById('btn-disconnect');

  const metricRTT = document.getElementById('metric-rtt');
  const metricP95 = document.getElementById('metric-p95');
  const metricCaptureRate = document.getElementById('metric-capture-rate');
  const metricQueueDepth = document.getElementById('metric-queue-depth');

  // Settings Modal Elements
  const settingsModal = document.getElementById('settings-modal');
  const btnSettingsOpen = document.getElementById('btn-settings-open');
  const btnSettingsClose = document.getElementById('btn-settings-close');
  const settingDeviceName = document.getElementById('setting-device-name');

  const inputNotice = document.createElement('p');
  inputNotice.setAttribute('role', 'alert');
  inputNotice.hidden = true;
  inputNotice.style.cssText = 'padding:12px 16px;color:#ffb4a9;white-space:normal;';
  document.body.prepend(inputNotice);

  // Initial Sync
  initApp();

  async function initApp() {
    if (!window.crosskvm) {
      daemonStatusText.textContent = 'IPC Unavailable';
      return;
    }

    // Check daemon status right away
    try {
      const status = await window.crosskvm.getDaemonStatus();
      if (status && status.connected) {
        daemonBadge.className = 'status-pill status-connected';
        daemonStatusText.textContent = 'Daemon Connected';
        fetchInitialData();
      }
    } catch (_) {}

    // Daemon Connection Status Listener
    window.crosskvm.onDaemonStatus((status) => {
      if (status && status.connected) {
        daemonBadge.className = 'status-pill status-connected';
        daemonStatusText.textContent = 'Daemon Connected';
        fetchInitialData();
      } else {
        daemonBadge.className = 'status-pill status-disconnected';
        daemonStatusText.textContent = 'Daemon Disconnected';
      }
    });

    // Daemon Asynchronous Events Listener
    window.crosskvm.onEvent((payload) => {
      handleDaemonEvent(payload);
    });

    // Fetch initial state and retry periodically until daemon is ready
    fetchInitialData();
    setInterval(() => {
      if (!localInfo) {
        fetchInitialData();
      }
    }, 1200);

    // Periodically refresh peers and status so UI never displays stale offline state
    setInterval(async () => {
      try {
        const peers = await window.crosskvm.getPeers();
        if (peers && peers.length > 0) {
          knownPeers = peers;
          renderPeers(knownPeers);
        }
        {
          const status = await window.crosskvm.getStatus();
          if (status) applyStatus(status);
        }
      } catch (_) {}
    }, 2000);
  }

  async function fetchInitialData() {
    try {
      localInfo = await window.crosskvm.getLocalInfo();
      if (localInfo) {
        daemonBadge.className = 'status-pill status-connected';
        daemonStatusText.textContent = 'Daemon Connected';

        localDeviceName.textContent = localInfo.name || 'Local Machine';
        localDeviceOS.textContent = `${localInfo.os} (${localInfo.arch}) · ${localInfo.screenWidth}×${localInfo.screenHeight}`;
        localDeviceID.textContent = localInfo.id;
        settingDeviceName.textContent = localInfo.name;

        updateMonitorsTitles();
      }

      currentStatus = await window.crosskvm.getStatus();
      if (currentStatus) {
        applyStatus(currentStatus);
      }

      knownPeers = await window.crosskvm.getPeers() || [];
      renderPeers(knownPeers);
    } catch (e) {
      // Waiting for daemon connection...
    }
  }

  function handleDaemonEvent({ event, data }) {
    switch (event) {
      case 'peer_discovered':
        handlePeerDiscovered(data);
        break;

      case 'peer_lost':
        handlePeerLost(data);
        break;

      case 'connected':
        if (data.peer) {
          selectedPeerId = data.peer.id;
        }
        applyConnectionState('connected', data.peer, data.side);
        break;

      case 'disconnected':
        applyConnectionState('disconnected', null, null);
        break;

      case 'reconnecting':
        applyConnectionState('reconnecting', null, null);
        break;

      case 'control_switched':
        applyControlState(data.state);
        break;

      case 'peer_side_changed':
      case 'layout_updated':
        if (data && data.side) {
          setLayoutSide(data.side);
          if (currentStatus) currentStatus.peerSide = data.side;
        }
        break;

      case 'latency_updated':
        applyMetrics(data);
        break;

      case 'error':
        console.error('[Daemon Error Event]', data);
        inputNotice.textContent = data.message || 'Input sharing failed. Check input permissions.';
        inputNotice.hidden = false;
        break;
    }
  }

  function handlePeerDiscovered(peer) {
    const idx = knownPeers.findIndex(p => p.id === peer.id);
    if (idx >= 0) {
      knownPeers[idx] = peer;
    } else {
      knownPeers.push(peer);
    }
    renderPeers(knownPeers);

    // Auto-select first discovered peer if none selected
    if (!selectedPeerId) {
      selectedPeerId = peer.id;
      btnStartKVM.disabled = false;
    }
  }

  function handlePeerLost(data) {
    const peer = knownPeers.find(p => p.id === data.deviceId);
    if (peer) {
      peer.online = false;
    }
    renderPeers(knownPeers);
  }

  function renderPeers(peers) {
    if (!peers || peers.length === 0) {
      peersList.innerHTML = `
        <div class="empty-peers">
          <div class="pulse-loader"></div>
          <span>Scanning local network for CrossKVM machines...</span>
        </div>
      `;
      return;
    }

    peersList.innerHTML = '';
    peers.forEach(peer => {
      const isSelected = peer.id === selectedPeerId;
      const activePeer = currentStatus && currentStatus.currentPeer;
      const isConnected = currentStatus && currentStatus.connectionState === 'connected' && activePeer &&
        (activePeer.id === peer.id || (activePeer.address && activePeer.address === peer.address));

      const card = document.createElement('div');
      card.className = `peer-card ${isSelected ? 'is-active' : ''}`;
      card.innerHTML = `
        <div class="peer-card-top">
          <div>
            <div class="device-name">${escapeHTML(peer.name)}</div>
            <div class="device-meta">${escapeHTML(peer.os)} · ${peer.screenWidth}×${peer.screenHeight}</div>
          </div>
          <span class="badge ${isConnected || peer.online ? 'badge-success' : 'badge-amber'}" title="Discovery status is separate from connection availability">${isConnected ? 'Connected' : peer.online ? 'Discovered' : 'Not recently seen'}</span>
        </div>
        <div class="peer-card-actions">
          <span class="device-id-label">${escapeHTML(peer.id.slice(0, 14))}</span>
          ${isConnected
            ? `<button class="btn btn-secondary btn-sm btn-peer-disconnect">Disconnect</button>`
            : `<button class="btn btn-primary btn-sm btn-peer-connect">Connect</button>`
          }
        </div>
      `;

      card.addEventListener('click', (e) => {
        if (e.target.tagName !== 'BUTTON') {
          selectedPeerId = peer.id;
          btnStartKVM.disabled = false;
          renderPeers(knownPeers);
          updateMonitorsTitles();
        }
      });

      const btnConnect = card.querySelector('.btn-peer-connect');
      if (btnConnect) {
        btnConnect.addEventListener('click', (e) => {
          e.stopPropagation();
          selectedPeerId = peer.id;
          btnStartKVM.disabled = false;
          const targetAddr = peer.address || (peer.ip ? `${peer.ip}:${peer.port || 4545}` : '');
          connectToPeer(peer.id, targetAddr);
        });
      }

      const btnDisc = card.querySelector('.btn-peer-disconnect');
      if (btnDisc) {
        btnDisc.addEventListener('click', (e) => {
          e.stopPropagation();
          disconnectActivePeer();
        });
      }

      peersList.appendChild(card);
    });
  }

  function applyStatus(status) {
    currentStatus = status;
    if (layoutDrag || layoutSaving) return;
    inputNotice.textContent = status.inputError || '';
    inputNotice.hidden = !status.inputError;
    const isConn = status.connectionState === 'connected';

    applyConnectionState(status.connectionState, status.currentPeer, status.peerSide);
    applyControlState(status.controlState);

    setLayoutSide(['left', 'right', 'top', 'bottom'].includes(status.peerSide) ? status.peerSide : layoutSide);

    if (status.kvmActive && isConn) {
      btnStartKVM.style.display = 'none';
      btnStopKVM.style.display = 'inline-flex';
    } else {
      btnStartKVM.style.display = 'inline-flex';
      btnStopKVM.style.display = 'none';
    }
  }

  function applyConnectionState(state, peer, side) {
    kvmConnState.textContent = state.charAt(0).toUpperCase() + state.slice(1);
    if (state === 'connected') {
      kvmConnState.style.color = 'var(--accent-emerald)';
      btnDisconnect.style.display = 'inline-flex';
      btnStartKVM.disabled = false;
      if (peer) {
        selectedPeerId = peer.id;
      }
    } else if (state === 'connecting' || state === 'reconnecting') {
      kvmConnState.style.color = 'var(--accent-amber)';
      btnDisconnect.style.display = 'inline-flex';
    } else {
      kvmConnState.style.color = 'var(--text-main)';
      btnDisconnect.style.display = 'none';
      btnStartKVM.style.display = 'inline-flex';
      btnStopKVM.style.display = 'none';
      btnStartKVM.disabled = !selectedPeerId;
    }
    updateMonitorsTitles();
  }

  function applyControlState(ctrlState) {
    const isRemote = ctrlState === 'remote';
    for (const box of [screenLeft, screenRight]) box.classList.remove('is-focused', 'is-remote-focused');
    kvmFocusState.textContent = isRemote ? 'Remote Peer' : 'Local Machine';
    kvmFocusState.className = `status-summary-val ${isRemote ? 'focus-remote' : 'focus-local'}`;

    const isPeerLeft = peerFirst();
    const localBox = isPeerLeft ? screenRight : screenLeft;
    const remoteBox = isPeerLeft ? screenLeft : screenRight;

    localBox.querySelector('.monitor-focus-badge').textContent = isRemote ? 'LOCAL' : 'ACTIVE INPUT';
    remoteBox.querySelector('.monitor-focus-badge').textContent = isRemote ? 'ACTIVE INPUT' : 'REMOTE';

    if (isRemote) {
      localBox.classList.remove('is-focused');
      remoteBox.classList.add('is-remote-focused');
    } else {
      remoteBox.classList.remove('is-remote-focused');
      localBox.classList.add('is-focused');
    }
  }

  function applyMetrics(metrics) {
    if (metrics.EndToEndStats && metrics.EndToEndStats.AvgMs > 0) {
      kvmLatencyVal.textContent = `${metrics.EndToEndStats.AvgMs.toFixed(2)} ms`;
      metricP95.textContent = `${metrics.EndToEndStats.P95Ms.toFixed(2)} ms`;
    } else if (metrics.LastRTTMs > 0) {
      kvmLatencyVal.textContent = `${(metrics.LastRTTMs / 2).toFixed(2)} ms`;
    }

    if (metrics.RTTStats && metrics.RTTStats.AvgMs > 0) {
      metricRTT.textContent = `${metrics.RTTStats.AvgMs.toFixed(2)} ms`;
    } else if (metrics.LastRTTMs > 0) {
      metricRTT.textContent = `${metrics.LastRTTMs.toFixed(2)} ms`;
    }

    metricCaptureRate.textContent = metrics.CaptureRateSec || 0;
    metricQueueDepth.textContent = metrics.QueueDepth || 0;
  }

  function updateMonitorsTitles() {
    const localName = localInfo ? localInfo.name : 'MacBook';
    let peerName = 'Remote Peer';

    if (selectedPeerId) {
      const p = knownPeers.find(x => x.id === selectedPeerId);
      if (p) peerName = p.name;
    }

    if (peerFirst()) {
      // Peer on Left: screen-left is Remote, screen-right is Local
      screenLeftTitle.textContent = peerName;
      screenLeftSub.textContent = 'Remote Peer';
      screenRightTitle.textContent = localName;
      screenRightSub.textContent = 'Local Display';
    } else {
      // Peer on Right: screen-left is Local, screen-right is Remote
      screenLeftTitle.textContent = localName;
      screenLeftSub.textContent = 'Local Display';
      screenRightTitle.textContent = peerName;
      screenRightSub.textContent = 'Remote Peer';
    }
  }

  function setLayoutSide(side) {
    layoutSide = side;
    sideLeftRadio.checked = side === 'left';
    sideRightRadio.checked = side === 'right';
    layoutCanvas.className = `layout-canvas layout-peer-${side}`;
    updateMonitorsTitles();
    screenLeft.classList.toggle('monitor-local', !peerFirst());
    screenRight.classList.toggle('monitor-local', peerFirst());
    screenLeft.classList.toggle('monitor-remote', peerFirst());
    screenRight.classList.toggle('monitor-remote', !peerFirst());
    screenLeft.setAttribute('aria-label', `${screenLeftTitle.textContent}, ${['top', 'bottom'].includes(side) ? 'top' : 'left'} screen. Use arrow keys to rearrange.`);
    screenRight.setAttribute('aria-label', `${screenRightTitle.textContent}, ${['top', 'bottom'].includes(side) ? 'bottom' : 'right'} screen. Use arrow keys to rearrange.`);
    document.getElementById('arrangement-status').textContent = `Peer on ${side}`;
    applyControlState(currentStatus ? currentStatus.controlState : 'local');
  }

  async function saveArrangement(side) {
    const previous = layoutSide;
    if (side === previous || layoutSaving) return;
    layoutSaving = true;
    sideLeftRadio.checked = side === 'left';
    sideRightRadio.checked = side === 'right';
    setLayoutSide(side);
    const status = document.getElementById('arrangement-status');
    status.textContent = 'Saving arrangement…';
    try {
      await window.crosskvm.setPeerSide(side);
      if (currentStatus) currentStatus.peerSide = side;
      status.textContent = `Saved · peer on ${side}`;
    } catch (err) {
      sideLeftRadio.checked = previous === 'left';
      sideRightRadio.checked = previous === 'right';
      setLayoutSide(previous);
      status.textContent = 'Could not save. Try again.';
    } finally {
      layoutSaving = false;
    }
  }

  function canArrange() {
    if (layoutSaving) return false;
    if (currentStatus && currentStatus.controlState === 'remote') {
      document.getElementById('arrangement-status').textContent = 'Return input here to arrange screens';
      return false;
    }
    return true;
  }

  for (const box of [screenLeft, screenRight]) {
    box.addEventListener('pointerdown', (event) => {
      if (event.button !== 0 || !canArrange()) return;
      const rect = box.getBoundingClientRect();
      layoutDrag = { box, pointer: event.pointerId, startX: event.clientX, startY: event.clientY, centerX: rect.left + rect.width / 2, centerY: rect.top + rect.height / 2, isPeer: box === (peerFirst() ? screenLeft : screenRight) };
      box.setPointerCapture(event.pointerId);
      box.classList.add('is-dragging');
      layoutCanvas.classList.add('is-arranging');
      event.preventDefault();
    });
    box.addEventListener('pointermove', (event) => {
      if (!layoutDrag || layoutDrag.box !== box || layoutDrag.pointer !== event.pointerId) return;
      const dx = event.clientX - layoutDrag.startX;
      const dy = event.clientY - layoutDrag.startY;
      const canvas = layoutCanvas.getBoundingClientRect();
      box.style.transform = `translate(${dx}px, ${dy}px)`;
      const x = layoutDrag.centerX + dx - (canvas.left + canvas.width / 2);
      const y = layoutDrag.centerY + dy - (canvas.top + canvas.height / 2);
      const vertical = Math.abs(y / canvas.height) > Math.abs(x / canvas.width);
      const first = vertical ? y < 0 : x < 0;
      const peerBefore = layoutDrag.isPeer ? first : !first;
      layoutDrag.side = vertical ? (peerBefore ? 'top' : 'bottom') : (peerBefore ? 'left' : 'right');
      document.getElementById('arrangement-status').textContent = `Release · peer on ${layoutDrag.side}`;
    });
    const finish = (event) => {
      if (!layoutDrag || layoutDrag.box !== box || layoutDrag.pointer !== event.pointerId) return;
      const side = layoutDrag.side;
      layoutDrag = null;
      box.style.transform = '';
      box.classList.remove('is-dragging');
      layoutCanvas.classList.remove('is-arranging');
      if (box.hasPointerCapture(event.pointerId)) box.releasePointerCapture(event.pointerId);
      if (event.type === 'pointerup' && side) saveArrangement(side);
      else setLayoutSide(layoutSide);
    };
    box.addEventListener('pointerup', finish);
    box.addEventListener('pointercancel', finish);
    box.addEventListener('lostpointercapture', finish);
    box.addEventListener('keydown', (event) => {
      if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key) || !canArrange()) return;
      event.preventDefault();
      const isPeer = box === (peerFirst() ? screenLeft : screenRight);
      const directions = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'top', ArrowDown: 'bottom' };
      const opposite = { left: 'right', right: 'left', top: 'bottom', bottom: 'top' };
      const side = directions[event.key];
      saveArrangement(isPeer ? side : opposite[side]);
    });
  }

  function showConnectionError(err, addr) {
    const msg = (err && err.message) ? err.message : String(err);
    console.error('[Connection Error]', msg);

    let notice = `Connection to ${addr || 'remote peer'} failed.\n\n${msg}`;
    if (localInfo && localInfo.os === 'darwin') {
      notice += '\n\nIf this address works in Terminal, check System Settings → Privacy & Security → Local Network → CrossKVM, then quit and reopen the app. Local Network access is separate from Accessibility.';
    }
    notice += '\n\nThis error alone does not identify a firewall block.';

    inputNotice.textContent = notice;
    inputNotice.hidden = false;
    inputNotice.style.background = 'rgba(239, 68, 68, 0.15)';
    inputNotice.style.border = '1px solid rgba(239, 68, 68, 0.35)';
    inputNotice.style.borderRadius = '8px';
    inputNotice.style.margin = '12px 24px';
    inputNotice.style.padding = '12px 16px';
    inputNotice.style.color = '#fca5a5';
    inputNotice.style.fontSize = '13px';
    inputNotice.style.lineHeight = '1.5';
    inputNotice.style.whiteSpace = 'pre-wrap';

    try {
      alert(notice);
    } catch (_) {}
  }

  // User Actions
  async function connectToPeer(peerId, addr) {
    try {
      inputNotice.hidden = true;
      applyConnectionState('connecting', null, null);
      const res = await window.crosskvm.connect({ peerId, addr });
      if (res && res.success) {
        applyConnectionState('connected', null, null);
        btnStartKVM.style.display = 'none';
        btnStopKVM.style.display = 'inline-flex';
      }
    } catch (err) {
      showConnectionError(err, addr || peerId);
      applyConnectionState('disconnected', null, null);
    }
  }

  async function disconnectActivePeer() {
    try {
      await window.crosskvm.disconnect();
      applyConnectionState('disconnected', null, null);
    } catch (err) {
      console.error('Disconnect failed:', err);
    }
  }

  btnStartKVM.addEventListener('click', async () => {
    const side = layoutSide;
    try {
      inputNotice.hidden = true;
      await window.crosskvm.startKVM({ peerId: selectedPeerId, side });
      btnStartKVM.style.display = 'none';
      btnStopKVM.style.display = 'inline-flex';
    } catch (err) {
      showConnectionError(err, selectedPeerId);
    }
  });

  btnStopKVM.addEventListener('click', async () => {
    try {
      await window.crosskvm.stopKVM();
      applyStatus(await window.crosskvm.getStatus());
    } catch (err) {
      console.error('Failed to stop KVM:', err);
    }
  });

  btnDisconnect.addEventListener('click', () => {
    disconnectActivePeer();
  });

  sideLeftRadio.addEventListener('change', async () => {
    setLayoutSide('left');
    try {
      await window.crosskvm.setPeerSide('left');
    } catch (_) {}
  });

  sideRightRadio.addEventListener('change', async () => {
    setLayoutSide('right');
    try {
      await window.crosskvm.setPeerSide('right');
    } catch (_) {}
  });

  btnRefreshPeers.addEventListener('click', async () => {
    btnRefreshPeers.textContent = 'Scanning...';
    try {
      if (window.crosskvm && window.crosskvm.rescan) {
        await window.crosskvm.rescan();
      }
    } catch (_) {}
    setTimeout(async () => {
      knownPeers = await window.crosskvm.getPeers() || [];
      renderPeers(knownPeers);
      btnRefreshPeers.textContent = 'Rescan';
    }, 1200);
  });

  // Direct IP Connect Handler
  const directIpInput = document.getElementById('direct-ip-input');
  const btnDirectConnect = document.getElementById('btn-direct-connect');
  if (btnDirectConnect && directIpInput) {
    btnDirectConnect.addEventListener('click', async () => {
      let addr = directIpInput.value.trim();
      if (!addr) return;
      if (!addr.includes(':')) {
        addr = `${addr}:4545`;
      }
      await connectToPeer('', addr);
    });

    directIpInput.addEventListener('keydown', async (e) => {
      if (e.key === 'Enter') {
        btnDirectConnect.click();
      }
    });
  }

  // Settings Modal Handlers
  btnSettingsOpen.addEventListener('click', () => {
    settingsModal.style.display = 'flex';
  });

  btnSettingsClose.addEventListener('click', () => {
    settingsModal.style.display = 'none';
  });

  settingsModal.addEventListener('click', (e) => {
    if (e.target === settingsModal) {
      settingsModal.style.display = 'none';
    }
  });

  function escapeHTML(str) {
    if (!str) return '';
    return str.replace(/[&<>'"]/g, 
      tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
    );
  }
});
