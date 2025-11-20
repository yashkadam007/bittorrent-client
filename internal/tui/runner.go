package tui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yashkadam007/bittorrent-client/internal/download"
	"github.com/yashkadam007/bittorrent-client/internal/pieces"
	"github.com/yashkadam007/bittorrent-client/internal/storage"
	"github.com/yashkadam007/bittorrent-client/internal/torrent"
	"github.com/yashkadam007/bittorrent-client/internal/tracker"
)

// Runner manages the TUI and download process integration
type Runner struct {
	torrent   *torrent.TorrentFile
	outputDir string
	port      int
	verbose   bool

	// Download components
	pieceManager    *pieces.PieceManager
	fileStorage     *storage.FileStorage
	downloadManager *download.DownloadManager
	trackerClient   *tracker.TrackerClient

	// TUI
	program *tea.Program
	model   Model

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRunner creates a new TUI runner
func NewRunner(torrentPath, outputDir string, port int, verbose bool) (*Runner, error) {
	// Parse torrent file
	t, err := torrent.ParseTorrentFile(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse torrent file: %w", err)
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	runner := &Runner{
		torrent:   t,
		outputDir: outputDir,
		port:      port,
		verbose:   verbose,
		ctx:       ctx,
		cancel:    cancel,
	}

	return runner, nil
}

// Run starts the TUI and download process
func (r *Runner) Run() error {
	// Initialize download components
	err := r.initializeComponents()
	if err != nil {
		return err
	}

	// Create TUI model
	r.model = NewModel(r.torrent.Info.Name, r.torrent.Info.GetTotalLength(), r.downloadManager)

	// Create TUI program
	r.program = tea.NewProgram(r.model, tea.WithAltScreen())

	// Set up signal handling
	r.setupSignalHandling()

	// Start download process in background
	go r.startDownload()

	// Start TUI
	_, err = r.program.Run()
	return err
}

// initializeComponents sets up all download-related components
func (r *Runner) initializeComponents() error {
	// Create piece manager
	pieceHashes, err := r.torrent.Info.GetPieceHashes()
	if err != nil {
		return fmt.Errorf("failed to get piece hashes: %w", err)
	}

	r.pieceManager = pieces.NewPieceManagerWithOptions(
		int(r.torrent.Info.PieceLength),
		r.torrent.Info.GetTotalLength(),
		pieceHashes,
		true, // quiet mode for TUI
	)

	// Create file storage
	r.fileStorage, err = storage.NewFileStorage(r.torrent, r.outputDir)
	if err != nil {
		return fmt.Errorf("failed to create file storage: %w", err)
	}

	// Check existing completion
	existingBitfield, err := r.fileStorage.GetCompletionBitfield()
	if err == nil && existingBitfield != nil && existingBitfield.GetNumCompletePieces() > 0 {
		// Update piece manager with existing progress
		for i := 0; i < existingBitfield.GetNumPieces(); i++ {
			if existingBitfield.HasPiece(i) {
				// Mark piece as complete in piece manager
				// This is a simplified approach - in a full implementation,
				// you'd restore the actual piece data
			}
		}
	}

	// Create tracker client
	r.trackerClient = tracker.NewTrackerClient()

	// Create download manager with rarest-first strategy (quiet mode for TUI)
	strategy := download.NewRarestFirstStrategy()
	r.downloadManager = download.NewDownloadManagerWithOptions(r.pieceManager, strategy, true)

	return nil
}

// startDownload begins the download process
func (r *Runner) startDownload() {
	// Start download manager
	r.downloadManager.Start()
	defer r.downloadManager.Stop()

	// Get initial peers from tracker (silently in TUI mode)
	trackerResp, err := r.trackerClient.GetPeers(r.torrent, r.port, "started")
	if err != nil {
		// In TUI mode, we don't print errors to stdout as it interferes with the UI
		// Errors will be visible in the TUI interface or logs
		return
	}

	if len(trackerResp.Peers) == 0 {
		// No peers found - TUI will show this in the peer count
		return
	}

	// Add peers to download manager
	r.downloadManager.AddPeers(trackerResp.Peers, r.torrent.InfoHash, r.trackerClient.GetPeerID())

	// Periodic tracker announcements
	go r.announceToTracker(trackerResp.Interval)

	// Monitor for completion
	go r.monitorCompletion()
}

// announceToTracker handles periodic tracker announcements
func (r *Runner) announceToTracker(interval int64) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if !r.downloadManager.IsActive() {
				return
			}

			resp, err := r.trackerClient.GetPeers(r.torrent, r.port, "")
			if err != nil {
				if r.verbose {
					fmt.Printf("Tracker announce failed: %v\n", err)
				}
				continue
			}

			if len(resp.Peers) > 0 {
				r.downloadManager.AddPeers(resp.Peers, r.torrent.InfoHash, r.trackerClient.GetPeerID())
			}
		}
	}
}

// monitorCompletion watches for download completion
func (r *Runner) monitorCompletion() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if r.pieceManager.IsComplete() {
				// Announce completion to tracker
				r.trackerClient.GetPeers(r.torrent, r.port, "completed")

				// Send completion message to TUI
				if r.program != nil {
					r.program.Send(completionMsg{})
				}
				return
			}
		}
	}
}

// setupSignalHandling configures graceful shutdown
func (r *Runner) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		r.shutdown()
	}()
}

// shutdown gracefully shuts down all components
func (r *Runner) shutdown() {
	// Cancel context to stop background goroutines
	r.cancel()

	// Stop download manager
	if r.downloadManager != nil {
		r.downloadManager.Stop()
	}

	// Close file storage
	if r.fileStorage != nil {
		r.fileStorage.Close()
	}

	// Final tracker announce
	if r.trackerClient != nil && r.torrent != nil {
		if r.pieceManager != nil && r.pieceManager.IsComplete() {
			r.trackerClient.GetPeers(r.torrent, r.port, "completed")
		} else {
			r.trackerClient.GetPeers(r.torrent, r.port, "stopped")
		}
	}

	// Quit TUI
	if r.program != nil {
		r.program.Quit()
	}
}

// The completionMsg type is defined in model.go
