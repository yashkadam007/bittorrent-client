# BitTorrent Client in Go

A simplified, educational BitTorrent client implementation demonstrating the core concepts of the BitTorrent protocol. This project is designed for learning purposes and resume demonstrations.

## Features

- ✅ **Bencode Encoding/Decoding**: Parse .torrent files using the BitTorrent bencode format
- ✅ **Tracker Communication**: Support for HTTP/HTTPS and UDP trackers
- ✅ **Peer Wire Protocol**: Establish connections and exchange messages with peers
- ✅ **Piece Management**: Download, verify, and assemble file pieces
- ✅ **File Storage**: Handle both single-file and multi-file torrents
- ✅ **Download Strategies**: Random and rarest-first piece selection
- ✅ **Progress Tracking**: Real-time download statistics and completion tracking
- ✅ **Terminal UI**: Beautiful, real-time terminal interface with progress visualization

## Architecture Overview

The client is organized into clean, modular packages:

### Core Components

```
internal/
├── bencode/     # Bencode encoding/decoding for .torrent files
├── torrent/     # Torrent file parsing and metadata handling
├── tracker/     # Communication with BitTorrent trackers
├── peer/        # Peer wire protocol implementation
├── pieces/      # Piece management and verification
├── download/    # Download coordination and strategy
├── storage/     # File storage and assembly
└── tui/         # Terminal user interface
```

### Data Flow

1. **Parse Torrent** → Extract metadata and piece hashes
2. **Contact Tracker** → Get list of available peers
3. **Connect to Peers** → Establish TCP connections and handshake
4. **Download Pieces** → Request and receive data blocks
5. **Verify Pieces** → Check SHA1 hashes for integrity
6. **Assemble Files** → Write verified pieces to disk

## Usage

### Basic Usage

**With Terminal UI (Default):**
```bash
# Download with beautiful terminal interface
go run main.go example.torrent

# Specify output directory and port
go run main.go example.torrent -output ./downloads -port 6881
```

**Command Line Mode:**
```bash
# Use traditional command-line output
go run main.go example.torrent -tui=false -verbose

# All options combined
go run main.go example.torrent -output ./downloads -port 6881 -tui=false -verbose
```

**Terminal UI Features:**
- 🎨 Real-time progress bar with completion percentage
- 📊 Live download statistics (speed, peers, ETA)
- 🧩 Visual piece completion map
- ⌨️ Interactive controls (h for help, q to quit)
- 🎯 Responsive design that adapts to terminal size

### Auto-detection
If no torrent file is specified, the client will automatically use the first `.torrent` file found in the current directory.

## Implementation Details

### BitTorrent Protocol Concepts

**Bencode Format**: BitTorrent uses a simple encoding format for structured data:
- Integers: `i42e`
- Strings: `4:spam`
- Lists: `l4:spam4:eggse`
- Dictionaries: `d3:cow3:moo4:spam4:eggse`

**Piece System**: Files are split into fixed-size pieces (typically 256KB-1MB):
- Each piece has a SHA1 hash for verification
- Pieces are downloaded in smaller blocks (16KB)
- Download can happen in any order

**Peer Wire Protocol**: TCP-based protocol for peer communication:
- Handshake exchange with torrent info hash
- Message types: choke, unchoke, interested, have, request, piece
- Pipeline multiple requests for efficiency

### Key Algorithms

**Rarest First Strategy**: Prioritizes downloading pieces that are rarest among all peers:
- Improves overall swarm health
- Ensures all pieces remain available
- Better than random selection for most cases

**Piece Verification**: Every piece is verified using SHA1 hash:
- Corrupted or incomplete pieces are re-downloaded
- Ensures file integrity
- Critical for BitTorrent's resilience

## Interview Talking Points

### Architecture & Design Patterns
- **Separation of Concerns**: Each package handles a specific protocol layer
- **Interface-based Design**: Strategy pattern for piece selection
- **Concurrent Programming**: Goroutines for peer management and progress tracking
- **Error Handling**: Comprehensive error propagation with context

### Protocol Understanding
- **BitTorrent Fundamentals**: DHT-less design focusing on tracker-based peer discovery
- **Network Programming**: TCP connections, binary protocol handling, timeouts
- **Data Integrity**: Hash verification, piece reconstruction, file assembly
- **P2P Concepts**: Swarm dynamics, tit-for-tat, choking algorithms (simplified)

### Go-Specific Features
- **Goroutines & Channels**: Concurrent peer handling and progress reporting
- **Interfaces**: Clean abstraction for different piece selection strategies
- **Error Handling**: Idiomatic Go error handling with wrapped errors
- **Standard Library**: Effective use of net, crypto, encoding packages

### Scalability Considerations
- **Connection Limits**: Configurable maximum peer connections
- **Memory Management**: Streaming piece processing, no full-file buffering
- **Resource Cleanup**: Proper file handle and connection management
- **Graceful Shutdown**: Clean termination with signal handling

## Simplifications

This implementation focuses on core concepts and makes several simplifications:

- **Download-only**: No uploading to other peers (leech mode)
- **Single torrent**: One torrent at a time
- **Basic DHT**: Relies on trackers, no Distributed Hash Table
- **No encryption**: Plain TCP connections (most trackers support this)
- **Simplified choking**: Basic connection management

## Testing

```bash
# Run the client with a small test torrent
go run main.go test.torrent -verbose

# The client will show:
# - Torrent information parsing
# - Tracker communication
# - Peer connections
# - Download progress
# - Piece verification
```

## Dependencies

**Core BitTorrent Protocol** (Standard Library):
- `net` - TCP connections
- `crypto/sha1` - Piece verification
- `encoding/binary` - Binary protocol handling
- `fmt`, `os`, `io` - Basic I/O operations

**Terminal UI** (External Libraries):
- `github.com/charmbracelet/bubbletea` - Modern TUI framework
- `github.com/charmbracelet/lipgloss` - Style and layout engine

The core protocol implementation uses only standard library - TUI is an optional enhancement!

## Learning Outcomes

Building this client demonstrates:

1. **Network Protocol Implementation**: Understanding binary protocols and state machines
2. **Concurrent Programming**: Managing multiple peer connections simultaneously
3. **File I/O & Storage**: Efficient handling of large file operations
4. **Error Handling**: Robust error management in distributed systems
5. **Algorithm Implementation**: Piece selection strategies and optimization
6. **System Design**: Clean architecture for complex, multi-component systems

This project showcases practical Go programming skills while implementing a real-world, production-level network protocol.