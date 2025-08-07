# BitTorrent Client

A full-featured BitTorrent client implemented in Go, following best practices and clean architecture patterns.

## Features

- **Complete BitTorrent Protocol Implementation**
  - Bencode encoding/decoding
  - Torrent file parsing (.torrent files)
  - Tracker communication (HTTP and UDP)
  - Peer wire protocol with handshake and messaging
  - Piece and block management
  - File verification using SHA-1 hashes

- **Download Strategies**
  - Random piece selection
  - Rarest-first piece selection for optimal swarm performance
  - Concurrent downloads from multiple peers
  - Automatic peer discovery and connection management

- **Storage Management**
  - Support for single-file and multi-file torrents
  - Cross-platform file I/O
  - Resume capability (checks existing files on startup)
  - Atomic piece verification

- **Network Features**
  - Concurrent peer connections
  - Keep-alive message handling
  - Connection timeout management
  - Bandwidth monitoring

## Project Structure

```
bittorrent-client/
├── cmd/                    # Application entry point
├── internal/
│   ├── bencode/           # Bencode encoding/decoding
│   ├── torrent/           # Torrent file parsing
│   ├── tracker/           # Tracker communication
│   ├── peer/              # Peer wire protocol
│   ├── pieces/            # Piece and bitfield management
│   ├── download/          # Download strategies and management
│   └── storage/           # File storage and I/O
├── build/                 # Build artifacts
├── Makefile              # Build automation
└── README.md
```

## Installation

### Prerequisites

- Go 1.21 or later
- Make (optional, for using Makefile)

### Building from Source

1. Clone the repository:
```bash
git clone <repository-url>
cd bittorrent-client
```

2. Build using Make:
```bash
make build
```

Or build manually:
```bash
go build -o build/bittorrent-client .
```

## Usage

### Basic Usage

```bash
# Run with a torrent file in current directory
./build/bittorrent-client example.torrent

# Specify custom output directory
./build/bittorrent-client example.torrent -output /path/to/downloads

# Enable verbose logging
./build/bittorrent-client example.torrent -verbose

# Use custom port
./build/bittorrent-client example.torrent -port 6882
```

### Using Makefile

```bash
# Build and run with available torrent
make run

# Run with specific torrent file
make run-with TORRENT=example.torrent

# Quick development build and run
make dev ARGS='example.torrent -verbose'

# List available torrent files
make list-torrents
```

### Command Line Options

- `-output <dir>`: Directory to save downloaded files (default: current directory)
- `-port <port>`: Port to listen on for peer connections (default: 6881)
- `-verbose`: Enable verbose logging for debugging

## Development

### Available Make Targets

```bash
make build        # Build the binary
make build-all    # Build for all platforms (Linux, Windows, macOS)
make clean        # Clean build artifacts
make test         # Run tests
make coverage     # Run tests with coverage report
make lint         # Run linter (requires golangci-lint)
make fmt          # Format code
make vet          # Run go vet
make deps         # Download and tidy dependencies
make all          # Run full development pipeline
```

### Code Quality

The project follows Go best practices:

- **Clean Architecture**: Separation of concerns with clear layer boundaries
- **Dependency Injection**: Loose coupling between components
- **Error Handling**: Comprehensive error handling throughout
- **Concurrency**: Safe concurrent operations with proper synchronization
- **Testing**: Unit tests for core functionality
- **Documentation**: Well-documented code and APIs

### Architecture Overview

1. **Bencode Layer**: Handles BitTorrent's custom encoding format
2. **Torrent Parser**: Extracts metadata from .torrent files
3. **Tracker Communication**: Discovers peers via HTTP/UDP trackers
4. **Peer Protocol**: Implements BitTorrent peer wire protocol
5. **Piece Management**: Tracks download progress and verifies integrity
6. **Download Strategy**: Optimizes piece selection for efficient downloads
7. **Storage Layer**: Manages file I/O and data persistence

## How It Works

1. **Parse Torrent**: Read and decode the .torrent file to extract metadata
2. **Contact Tracker**: Announce to tracker and discover peers
3. **Connect to Peers**: Establish connections using BitTorrent handshake
4. **Exchange Pieces**: Request and download file pieces from peers
5. **Verify Integrity**: Validate each piece using SHA-1 hashes
6. **Assemble File**: Combine verified pieces into final files

## Performance

- **Concurrent Downloads**: Downloads from multiple peers simultaneously
- **Optimized Piece Selection**: Uses rarest-first strategy for better swarm performance
- **Memory Efficient**: Streams data to disk without loading entire files
- **Resume Capability**: Automatically resumes interrupted downloads

## Limitations

- Downloads only (no seeding/uploading)
- HTTP and UDP trackers only (no DHT or peer exchange)
- IPv4 support only
- Single-threaded piece verification

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes following Go best practices
4. Add tests for new functionality
5. Run `make all` to ensure code quality
6. Submit a pull request

## License

This project is open source. See LICENSE file for details.

## References

- [BitTorrent Protocol Specification](https://wiki.theory.org/BitTorrentSpecification)
- [BEP-0003: The BitTorrent Protocol Specification](http://bittorrent.org/beps/bep_0003.html)
- [BEP-0015: UDP Tracker Protocol](http://bittorrent.org/beps/bep_0015.html)
