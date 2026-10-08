# CrossKVM

**CrossKVM** is a high-performance, lightweight, open-source software KVM application designed to seamlessly share mouse and keyboard inputs across macOS and Windows computers on the same local network (LAN).

---

## Project Status

- **Phase 1 — Protocol & Transport**: Complete (Framed JSON, TCP, State Management).
- **Phase 2 — Windows Input Injection**: Complete (Win32 `SendInput`, Scan Codes, Extended Keys).
- **Phase 3 — macOS Input Injection**: Complete (`CoreGraphics` `CGEvent`, Accessibility Checks, Modifier Tracking).
- **Phase 4A — macOS Physical Input Capture**: Complete (`CGEventTap`, baseline relative deltas, modifier state tracking, synthetic loop prevention).
- **Phase 4B — Windows Physical Input Capture**: Complete (`WH_MOUSE_LL`, `WH_KEYBOARD_LL`, `SetWindowsHookEx`, baseline relative deltas, modifier detection, `dwExtraInfo` synthetic loop prevention).
- **Phase 5 — Screen Switching & Input Ownership**: Complete (Seamless two-machine horizontal switching, dynamic local input suppression on macOS & Windows, normalized cursor entry geometry, hysteresis/arming, emergency release hotkey, and stuck-key cleanup).

> [!NOTE]
> - **Phase 5 Scope**: Supports exactly two machines on the same local network in a horizontal arrangement (`[Left][Right]`).
> - **Primary Display**: Uses primary display dimensions for edge detection and cursor translation. Multi-monitor layouts are deferred to future phases.
> - **Electron UI**: CrossKVM currently operates as a high-performance, native CLI service.

---

## Architecture Overview

```
Mac Screen                                    Windows Screen
┌───────────────────────────┐                 ┌───────────────────────────┐
│                           │                 │                           │
│                       → → │ ──────────────► │ →                         │
│                           │ (Switch Remote) │                           │
└───────────────────────────┘                 └───────────────────────────┘
```

```
Physical Input
     │
     ▼
InputBackend (macOS CGEventTap / Win32 Low-Level Hooks)
     │
     ▼
InputEvent (Platform-Neutral Model)
     │
     ▼
Control Router
     ├── StateLocal  ──► Physical input operates local OS normally (Suppression OFF)
     │                    └── At boundary + intent ──► Switch to REMOTE
     │
     └── StateRemote ──► Local input consumed (Suppression ON)
                          ├── Emergency chord? (Ctrl+Alt+Shift+Escape) ──► Restore LOCAL
                          ├── Reaches return edge? ──► Restore LOCAL
                          └── Forward to Peer via Framed TCP (Outbound Queue)
                                └── Peer Injects Native Input
```

---

## Phase 5 Key Mechanisms

### 1. Ownership State Machine
Input ownership is governed by a thread-safe `StateManager`:
- `StateLocal`: User input directly drives the local machine. Local input suppression is `false`.
- `StateRemote`: Local physical input is captured, suppressed from affecting the local machine, and forwarded across TCP to the remote peer.
- `StateTransitioning`: Atomic handover between local and remote states.

### 2. Edge Detection, Intent & Hysteresis
- **Threshold**: Triggers when cursor is within `edgeThreshold` pixels of the configured screen edge (default 3 pixels).
- **Intent Check**: Requires deliberate movement towards the peer (`DX > 0` for right-side peer, `DX < 0` for left-side peer). Top/bottom layouts use the same intent check on DY. Movement parallel to the selected edge does not switch control.
- **Hysteresis / Arming**: Upon returning from remote control, the edge is unarmed (`edgeArmed = false`) and the cursor is placed with a re-entry offset (20 pixels) away from the boundary. The edge is only re-armed once the user moves further into the screen, completely preventing instant switch-back loops.

### 3. Local Input Suppression
- **macOS**: Configured using `CGEventTapCreate` with `kCGEventTapOptionDefault`. When `suppressed == true`, physical mouse and keyboard events are translated and forwarded, and the callback returns `NULL` to discard local OS delivery. CrossKVM synthetic events (`CROSSKVM_USER_DATA_MARKER = 0x584B564D`) are never suppressed or re-captured.
- **Windows**: Low-level hooks (`WH_MOUSE_LL` and `WH_KEYBOARD_LL`) inspect incoming events. When `suppressed == true` and `dwExtraInfo != CrossKVMWindowsMarker`, the callback translates the event, enqueues it, and returns `1` (non-zero), preventing Windows from processing it locally. Injected synthetic events pass through to `CallNextHookEx`.

### 4. Normalized Cursor Geometry Translation
Screen height differences between devices (e.g. 1080p laptop vs 1440p monitor) are handled by exchanging primary display dimensions during connection handshake:
$$\text{normY} = \frac{\text{localY}}{\text{localHeight}}$$
$$\text{remoteEntryY} = \text{normY} \times \text{remoteHeight}$$
When moving between screens, vertical cursor position is preserved smoothly without jumping.

### 5. Emergency Release Shortcut
- Hard-coded emergency chord: `Ctrl + Alt + Shift + Escape`
- Works instantly while in `StateRemote` with suppression active.
- Bypasses normal event forwarding, immediately releases all pressed keys/buttons, disables local suppression, and restores local control.

### 6. Stuck-Key Prevention & Failure Recovery
- **Held-Input Tracking**: Router tracks all forwarded key-down and mouse-button-down events in an active input map.
- **Clean Release**: Upon control release, emergency restore, peer disconnect, or process shutdown, synthetic `KeyUp` and `MouseButtonUp` messages are transmitted to the peer for all tracked held keys/buttons.
- **Disconnect Safety**: If the TCP connection breaks or write fails during `StateRemote`, local control is immediately restored and suppression disabled. The user is never trapped without mouse/keyboard control.

---

## Build & Test

### Build Binaries

```bash
# Build native macOS binary
go build -o bin/crosskvm ./cmd/crosskvm

# Cross-compile for Windows (AMD64)
GOOS=windows GOARCH=amd64 go build -o bin/crosskvm_amd64.exe ./cmd/crosskvm
```

### Run Full Test Suite

```bash
go test -v ./internal/config ./internal/protocol ./internal/input ./internal/control
```

---

## Verified Setup: Mac ↔ Windows Two-Machine KVM

### Layout: MacBook (Left) + Windows PC (Right)

```
[ MacBook (Left) ]  ──────  [ Windows PC (Right) ]
```

#### Step 1: Start Windows PC (Listener / Server)
On the Windows PC, start CrossKVM in KVM listener mode with peer on the left:
```powershell
.\crosskvm_amd64.exe kvm --listen :4545 --peer-side left
```

#### Step 2: Start MacBook (Connector / Client)
On the MacBook, start CrossKVM connecting to the Windows PC with peer on the right:
```bash
./crosskvm kvm --connect 192.168.1.50:4545 --peer-side right
```

#### Operation:
1. Move the Mac mouse cursor smoothly past the right screen edge.
2. Control instantly switches to Windows:
   - Mac cursor stops moving locally.
   - Windows cursor moves naturally using physical Mac mouse movements.
   - Mac keyboard types directly into Windows applications.
3. Move the Windows cursor past the left screen edge.
4. Control returns seamlessly to the MacBook.
5. Press `Ctrl + Alt + Shift + Escape` at any time to instantly reclaim local control.

---

### Layout: Windows PC (Left) + MacBook (Right)

```
[ Windows PC (Left) ]  ──────  [ MacBook (Right) ]
```

#### Step 1: Start MacBook (Listener)
```bash
./crosskvm kvm --listen :4545 --peer-side left
```

#### Step 2: Start Windows PC (Connector)
```powershell
.\crosskvm_amd64.exe kvm --connect 192.168.1.50:4545 --peer-side right
```

---

## Phase 7 — Electron Desktop UI

CrossKVM features a desktop interface built with Electron as a thin UI layer over the native Go daemon core.

```
Electron Renderer (HTML5/CSS3/Vanilla JS)
        │  (contextBridge / window.crosskvm)
        ▼
Electron Main (Node.js)
        │  (Local IPC: Unix Domain Socket / Loopback TCP)
        ▼
CrossKVM Go Daemon (`crosskvm daemon`)
        │
        ▼
CrossKVM Native Core (CoreGraphics / Win32 Hooks, Framing, Discovery)
```

### Starting the Desktop App

```bash
# 1. Build the Go binaries
go build -o bin/crosskvm ./cmd/crosskvm
GOOS=windows GOARCH=amd64 go build -o bin/crosskvm_amd64.exe ./cmd/crosskvm

# 2. Launch Electron
cd desktop
npm start
```

---

## CLI Command Reference

| Command | Description |
| :--- | :--- |
| `crosskvm kvm [flags]` | Launch seamless two-machine software KVM mode |
| `crosskvm daemon [flags]` | Start background Go IPC daemon for Electron Desktop UI |
| `crosskvm peers` | List known and cached LAN peers |
| `crosskvm discover` | Actively scan local network for CrossKVM peers |
| `crosskvm metrics [--watch]` | View or stream real-time latency and performance metrics |
| `crosskvm listen [flags]` | Start in listener (server) mode |
| `crosskvm connect [flags]` | Start in connect (client) mode |
| `crosskvm inject-test [cmd]` | Test direct OS input injection locally |
| `crosskvm capture-test` | Test physical input capture and print neutral events |


### Vertical screen arrangement

The desktop app supports dragging either display above or below the other display; Up/Down arrow keys also rearrange a focused display. `--peer-side` specifies the other computer’s position relative to this computer. Both computers need the updated binary for vertical handoff.

For a Mac below a Windows screen, use `top` on the Mac and `bottom` on Windows:

```bash
./bin/crosskvm kvm --listen :4545 --peer-side top
```

```powershell
.\crosskvm_amd64.exe kvm --connect <MAC-IP>:4545 --peer-side bottom
```

Cursor handoff preserves the relative horizontal position across screen widths. Move through the bottom edge of Windows to enter the Mac, or the top edge of the Mac to enter Windows.

### Windows emergency recovery

Press **Ctrl + Alt + Shift + Esc on the physical Windows keyboard** to release local input and disconnect the peer. The native hook handles this before the event queue; the desktop app also registers the same recovery shortcut. Closing the desktop app terminates its owned daemon before asynchronous shutdown work.

If an older build is stuck, use **Ctrl + Alt + Delete → Task Manager** and end both `CrossKVM.exe` and `crosskvm_amd64.exe` (or `crosskvm_arm64.exe`). `bin/stop-crosskvm.bat` provides the same termination commands. The recovery ZIP includes it beside `CrossKVM.exe`; extract the complete folder before launching.

Windows daemon logs are in `%APPDATA%\CrossKVM\logs\daemon.log`.
