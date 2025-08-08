package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yashkadam007/bittorrent-client/internal/download"
	"github.com/yashkadam007/bittorrent-client/internal/pieces"
	"github.com/yashkadam007/bittorrent-client/internal/storage"
	"github.com/yashkadam007/bittorrent-client/internal/torrent"
	"github.com/yashkadam007/bittorrent-client/internal/tracker"
)

// Run executes the BitTorrent client with the given parameters.
// This is the main orchestration function that coordinates all components.
func Run(torrentPath, outputDir string, port int, verbose bool) error {
	// Parse torrent file
	fmt.Printf("Parsing torrent file: %s\n", torrentPath)
	t, err := torrent.ParseTorrentFile(torrentPath)
	if err != nil {
		return fmt.Errorf("failed to parse torrent file: %w", err)
	}

	// Print torrent information
	fmt.Println("\n" + t.String())

	// Create piece manager
	pieceHashes, err := t.Info.GetPieceHashes()
	if err != nil {
		return fmt.Errorf("failed to get piece hashes: %w", err)
	}

	pieceManager := pieces.NewPieceManager(
		int(t.Info.PieceLength),
		t.Info.GetTotalLength(),
		pieceHashes,
	)

	// Create file storage
	fmt.Printf("Setting up file storage in: %s\n", outputDir)
	fileStorage, err := storage.NewFileStorage(t, outputDir)
	if err != nil {
		return fmt.Errorf("failed to create file storage: %w", err)
	}
	defer fileStorage.Close()

	// Check existing completion
	existingBitfield, err := fileStorage.GetCompletionBitfield()
	if err != nil && verbose {
		fmt.Printf("Warning: Failed to check existing files: %v\n", err)
	} else if existingBitfield != nil {
		completed, total, percentage := existingBitfield.GetNumCompletePieces(),
			existingBitfield.GetNumPieces(), existingBitfield.GetCompletionPercentage()

		if completed > 0 {
			fmt.Printf("Found existing progress: %d/%d pieces (%.1f%%)\n",
				completed, total, percentage)

			if existingBitfield.IsComplete() {
				fmt.Println("Download already complete!")
				return nil
			}
		}
	}

	// Create tracker client
	trackerClient := tracker.NewTrackerClient()

	// Create download manager with rarest-first strategy
	strategy := download.NewRarestFirstStrategy()
	downloadManager := download.NewDownloadManager(pieceManager, strategy)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Start download
	fmt.Println("Starting download...")
	downloadManager.Start()
	defer downloadManager.Stop()

	// Get initial peers from tracker
	fmt.Println("Contacting tracker...")
	trackerResp, err := trackerClient.GetPeers(t, port, "started")
	if err != nil {
		return fmt.Errorf("failed to get peers from tracker: %w", err)
	}

	fmt.Printf("Tracker response: %d seeders, %d leechers, %d peers\n",
		trackerResp.Complete, trackerResp.Incomplete, len(trackerResp.Peers))

	if len(trackerResp.Peers) == 0 {
		return fmt.Errorf("no peers found")
	}

	if verbose {
		fmt.Printf("Found peers: %s\n", tracker.FormatPeers(trackerResp.Peers))
	}

	// Add peers to download manager
	downloadManager.AddPeers(trackerResp.Peers, t.InfoHash, trackerClient.GetPeerID())

	// Progress reporting
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !downloadManager.IsActive() {
					return
				}

				completed, total, percentage := downloadManager.GetProgress()
				stats := downloadManager.GetStats()

				fmt.Printf("Progress: %d/%d pieces (%.1f%%) | Speed: %.2f KB/s | Peers: %d\n",
					completed, total, percentage,
					stats.DownloadSpeed/1024, stats.PeersConnected)

				if pieceManager.IsComplete() {
					fmt.Println("Download completed!")
					cancel()
					return
				}
			}
		}
	}()

	// Periodic tracker announcements
	go func() {
		ticker := time.NewTicker(time.Duration(trackerResp.Interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !downloadManager.IsActive() {
					return
				}

				resp, err := trackerClient.GetPeers(t, port, "")
				if err != nil {
					if verbose {
						fmt.Printf("Tracker announce failed: %v\n", err)
					}
					continue
				}

				if len(resp.Peers) > 0 {
					downloadManager.AddPeers(resp.Peers, t.InfoHash, trackerClient.GetPeerID())
				}
			}
		}
	}()

	// Wait for completion or cancellation
	<-ctx.Done()

	// Final tracker announce
	if pieceManager.IsComplete() {
		trackerClient.GetPeers(t, port, "completed")
		fmt.Println("Download completed successfully!")
	} else {
		trackerClient.GetPeers(t, port, "stopped")
		completed, total, percentage := downloadManager.GetProgress()
		fmt.Printf("Download stopped at %.1f%% (%d/%d pieces)\n",
			percentage, completed, total)
	}

	return nil
}
