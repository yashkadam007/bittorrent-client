q# Terminal UI Demo

## What the TUI looks like:

```
🌊 BitTorrent Client
ubuntu-20.04.6-desktop-amd64.iso

📥 Download Progress:
████████████████████████████████████████████░░░░░░░░░░░░░░░░ 73.2%

📊 Statistics:
Size:      1.83 GB / 2.50 GB
Pieces:    1342 / 1834
Speed:     2.45 MB/s
Peers:     12
ETA:       4m 32s

🧩 Pieces:
██████████████████████████████████████░░░░░░░░░░░░░░░░░░░░
████████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░

Press 'h' for help • 'q' to quit
```

## Interactive Features:

### Help Screen (Press 'h'):
```
┌─────────────────────────────────────────────────────────────┐
│ 🌊 BitTorrent Client - Help                                 │
│                                                             │
│ Keyboard Controls:                                          │
│   h, ?    Toggle this help screen                          │
│   q       Quit the application                             │
│   Ctrl+C  Force quit                                       │
│                                                             │
│ Information Display:                                        │
│   📥 Progress bar shows download completion                 │
│   📊 Statistics show speed, peers, and ETA                 │
│   🧩 Piece visualization shows which parts are complete    │
│                                                             │
│ The client automatically:                                   │
│   • Connects to peers from trackers                        │
│   • Downloads pieces using rarest-first strategy           │
│   • Verifies pieces with SHA1 hashes                       │
│   • Assembles files on disk                                │
│                                                             │
│ Press 'h' again to return to the main view.               │
└─────────────────────────────────────────────────────────────┘
```

## Real-time Updates:

The interface updates every second with:
- ✅ **Live Progress**: Visual progress bar with exact percentage
- ✅ **Download Speed**: Real-time speed calculation
- ✅ **Peer Count**: Number of active connections
- ✅ **ETA Estimation**: Time remaining based on current speed
- ✅ **Piece Visualization**: Individual piece completion status
- ✅ **File Size**: Downloaded vs total size with smart units (B/KB/MB/GB)

## Usage Examples:

```bash
# Start with TUI (default)
./bittorrent-client my-file.torrent

# Use specific output directory
./bittorrent-client my-file.torrent -output ~/Downloads

# Traditional command-line mode
./bittorrent-client my-file.torrent -tui=false -verbose

# Custom port
./bittorrent-client my-file.torrent -port 8080
```

## Benefits for Resume/Interview:

1. **Modern UI/UX**: Demonstrates ability to create polished user interfaces
2. **Real-time Systems**: Shows skills in building responsive, live-updating applications
3. **Go TUI Libraries**: Experience with modern Go ecosystem (Bubble Tea)
4. **User Experience**: Focus on making technical tools user-friendly
5. **Architecture**: Clean separation between UI and business logic
6. **Responsive Design**: UI adapts to different terminal sizes
7. **Error Handling**: Graceful handling of edge cases and cleanup

The TUI makes the BitTorrent client much more professional and interview-ready!
