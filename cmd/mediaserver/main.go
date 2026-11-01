// Command mediaserver is a local streaming server for movies, series and anime.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"mediaserver/internal/api"
	"mediaserver/internal/config"
	"mediaserver/internal/db"
	"mediaserver/internal/intro"
	"mediaserver/internal/library"
	"mediaserver/internal/metadata"
	"mediaserver/internal/probe"
	"mediaserver/internal/scanner"
	"mediaserver/internal/stream"
	"mediaserver/internal/subtitles"
	"mediaserver/migrations"
	"mediaserver/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const shutdownTimeout = 10 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	start := time.Now()
	if code, ok := command(os.Args[1:], os.Stdout, os.Stderr); ok {
		return code
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveMain(ctx, os.Args[1:], start, false)
}

// serveMain loads the configuration from args and runs the server until ctx
// ends. logFile: logs go to DATA_DIR/mediaserver.log instead of stderr (a
// Windows service has no console).
func serveMain(ctx context.Context, args []string, start time.Time, logFile bool) int {
	cfg, opts, err := config.Load(args, os.Getenv, os.Stderr)
	switch {
	case errors.Is(err, config.ErrHelp):
		return 0
	case err != nil:
		fmt.Fprintln(os.Stderr, "mediaserver: config:", err)
		return 2
	case opts.ShowVersion:
		fmt.Println("mediaserver", version)
		return 0
	}

	var out io.Writer = os.Stderr
	if logFile {
		f, err := openLogFile(cfg.DataDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mediaserver: log file:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	logger := newLogger(out, cfg.Env, cfg.LogLevel)
	if err := serve(ctx, cfg, logger, start); err != nil {
		logger.Error("server stopped with error", "err", err)
		return 1
	}
	return 0
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger, start time.Time) error {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir %s: %w", cfg.DataDir, err)
	}

	dbPath := filepath.Join(cfg.DataDir, db.FileName)
	conn, err := db.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Error("close db", "err", err)
		}
	}()
	applied, err := db.Migrate(ctx, conn, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	schema, err := db.CurrentVersion(ctx, conn)
	if err != nil {
		return err
	}

	// Background work (scan, watcher) stops with this context; it is canceled
	// before waiting for it, also when serve fails early.
	ctx, cancel := context.WithCancel(ctx)
	// Openings and credits of new episodes, looked for after every scan.
	analyzer := intro.NewAnalyzer(conn, cfg.MediaDir, cfg.FFmpegPath, logger.With("component", "intro"))
	// Genres and collections, only when the user turned TMDB on (off by default).
	meta := metadata.New(conn, logger.With("component", "metadata"))
	afterScan := func() {
		analyzer.Trigger()
		meta.Trigger()
	}
	lib, stopLib, err := startLibrary(ctx, cfg, conn, logger, afterScan)
	if err != nil {
		cancel()
		return err
	}
	defer func() {
		cancel()
		stopLib()
	}()

	mediaRoot, err := os.OpenRoot(cfg.MediaDir)
	if err != nil {
		return fmt.Errorf("open media dir: %w", err)
	}
	defer mediaRoot.Close()
	store := library.Store{DB: conn}
	secret, err := store.Secret(ctx)
	if err != nil {
		return err
	}
	images, err := stream.NewImages(conn, mediaRoot, cfg.FFmpegPath, filepath.Join(cfg.DataDir, "images"),
		logger.With("component", "images"))
	if err != nil {
		return err
	}

	subs, err := subtitles.NewStore(conn, mediaRoot, cfg.FFmpegPath, filepath.Join(cfg.DataDir, "subtitles"),
		logger.With("component", "subtitles"))
	if err != nil {
		return err
	}
	transcoder, err := stream.NewTranscoder(ctx, stream.TranscoderOptions{
		Root: mediaRoot, FFmpeg: cfg.FFmpegPath, FFprobe: cfg.FFprobePath,
		Dir: filepath.Join(cfg.DataDir, "transcode"), HWAccel: cfg.HWAccel,
		MaxTranscodes: cfg.MaxTranscodes, CacheBytes: int64(cfg.TranscodeCacheMB) << 20,
		Logger: logger.With("component", "transcode"),
	})
	if err != nil {
		return err
	}
	// Stops every FFmpeg when ctx ends; waited for before closing MEDIA_DIR.
	var bg sync.WaitGroup
	bg.Go(func() { transcoder.Run(ctx) })
	bg.Go(func() { analyzer.Run(ctx) })
	bg.Go(func() { meta.Run(ctx) })
	defer func() {
		cancel()
		bg.Wait()
	}()

	assets, err := web.Assets()
	if err != nil {
		return fmt.Errorf("load frontend: %w", err)
	}

	srv := &http.Server{
		Addr: cfg.Addr(),
		Handler: api.NewRouter(api.Deps{
			Logger: logger, DB: conn, Version: version, Web: assets,
			Scanner: lib, StartScan: func() bool { return lib.Start(ctx) },
			Library: store, Stream: &stream.Server{Root: mediaRoot, Files: store}, Images: images,
			Transcoder: transcoder, Subtitles: subs, Secret: secret, Metadata: meta,
			DataDir: cfg.DataDir, Started: start,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}

	logger.Info("server ready",
		"version", version,
		"addr", ln.Addr().String(),
		"media_dir", cfg.MediaDir,
		"db", dbPath,
		"schema_version", schema,
		"migrations_applied", applied,
		"startup_ms", time.Since(start).Milliseconds(),
	)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// copyGrace is how long a file must stay untouched before the scanner
// indexes it (a copy in progress is left for the next scan).
const copyGrace = 2 * time.Second

// startLibrary creates the scanner, runs the startup scan and the watcher in
// the background. stop waits for both after ctx is canceled.
func startLibrary(ctx context.Context, cfg config.Config, conn *sql.DB, logger *slog.Logger, afterScan func()) (*scanner.Scanner, func(), error) {
	var prober scanner.Prober
	p := probe.Prober{Path: cfg.FFprobePath}
	if err := p.Available(ctx); err != nil {
		logger.Warn("ffprobe not available: files are indexed by name only (no codecs, duration or tracks)",
			"ffprobe", cfg.FFprobePath, "err", err)
	} else {
		prober = p
	}

	lib, err := scanner.New(ctx, conn, scanner.Options{
		Root: cfg.MediaDir, Workers: cfg.ScanWorkers, MinAge: copyGrace, Prober: prober,
		Logger: logger.With("component", "scanner"), AfterScan: afterScan,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scanner: %w", err)
	}
	if cfg.ScanOnStart {
		lib.Start(ctx)
	}
	var wg sync.WaitGroup
	if cfg.Watch {
		wg.Go(func() {
			if err := lib.Watch(ctx, scanner.WatchOptions{}); err != nil {
				logger.Error("watcher stopped", "err", err)
			}
		})
	}
	stop := func() {
		wg.Wait()
		lib.Wait()
	}
	return lib, stop, nil
}
