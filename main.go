// BitTorrent Client
//
// A simplified BitTorrent client implementation in Go that demonstrates
// the core concepts of the BitTorrent protocol:
// - Bencode encoding/decoding for .torrent files
// - Tracker communication (HTTP/HTTPS and UDP)
// - Peer wire protocol for downloading pieces
// - Piece verification using SHA1 hashes
// - File assembly and storage
//
// This client is designed for educational purposes and resume demonstrations.
// It implements a download-only client with rarest-first piece selection.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/yashkadam007/bittorrent-client/cmd"
)

func main() {
	// Auto-detect .torrent file if not provided
	if len(os.Args) < 2 {
		files, err := filepath.Glob("*.torrent")
		if err != nil || len(files) == 0 {
			fmt.Println("Usage: go run main.go <file.torrent> [options]")
			fmt.Println("Or place a .torrent file in the current directory")
			os.Exit(1)
		}
		// Use the first .torrent file found
		os.Args = append([]string{os.Args[0], files[0]}, os.Args[1:]...)
		fmt.Printf("Using found torrent file: %s\n", files[0])
	}

	// Parse command line arguments
	torrentFile := os.Args[1]

	// Set up flags for remaining arguments
	outputDir := flag.String("output", ".", "Output directory")
	port := flag.Int("port", 6881, "Port to listen on")
	verbose := flag.Bool("verbose", false, "Verbose output")
	useTUI := flag.Bool("tui", true, "Use terminal UI (default: true)")

	flag.CommandLine.Parse(os.Args[2:])

	// Show startup info only in non-TUI mode
	if !*useTUI {
		fmt.Printf("BitTorrent Client\n")
		fmt.Printf("Torrent: %s\n", torrentFile)
		fmt.Printf("Output: %s\n", *outputDir)
		fmt.Printf("Port: %d\n", *port)
	}

	// Delegate to cmd package
	var err error
	if *useTUI {
		err = cmd.RunWithTUI(torrentFile, *outputDir, *port, *verbose)
	} else {
		err = cmd.Run(torrentFile, *outputDir, *port, *verbose)
	}
	if err != nil {
		log.Fatal(err)
	}
}
