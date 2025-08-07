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
	if len(os.Args) < 2 {
		// Try to find a .torrent file in current directory
		files, err := filepath.Glob("*.torrent")
		if err != nil || len(files) == 0 {
			fmt.Println("Usage: go run main.go <file.torrent> [options]")
			fmt.Println("Or place a .torrent file in the current directory")
			os.Exit(1)
		}

		// Add the found torrent file as the first argument
		os.Args = append([]string{os.Args[0], files[0]}, os.Args[1:]...)
		fmt.Printf("Using found torrent file: %s\n", files[0])
	}

	// Parse command line arguments
	torrentFile := os.Args[1]

	// Set up flags for remaining arguments
	outputDir := flag.String("output", ".", "Output directory")
	port := flag.Int("port", 6881, "Port to listen on")
	verbose := flag.Bool("verbose", false, "Verbose output")

	flag.CommandLine.Parse(os.Args[2:])

	fmt.Printf("BitTorrent Client\n")
	fmt.Printf("Torrent: %s\n", torrentFile)
	fmt.Printf("Output: %s\n", *outputDir)
	fmt.Printf("Port: %d\n", *port)

	// Delegate to cmd package
	err := cmd.Run(torrentFile, *outputDir, *port, *verbose)
	if err != nil {
		log.Fatal(err)
	}
}
