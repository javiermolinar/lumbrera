package servecmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/javiermolinar/lumbrera/internal/mcpserver"
)

// Run serves a local brain directory through read-only MCP.
func Run(args []string, version string) error {
	return run(args, version, os.Stderr)
}

func run(args []string, version string, out io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(out)
	var config mcpserver.Config
	var listen string
	flags.StringVar(&config.Brain, "brain", "", "Local Lumbrera brain directory")
	flags.StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address; remote access requires private authenticated ingress")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: lumbrera serve --brain <directory> [--listen 127.0.0.1:8080]\n\nRead-only Streamable HTTP MCP at /mcp; /livez and /readyz health checks.\nReads the current brain directly. Git is not required. No MCP write tools.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("serve accepts flags only")
	}
	config.Version = version
	svc, err := mcpserver.New(config)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	httpServer := &http.Server{Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		prepareCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := svc.Prepare(prepareCtx); err != nil {
			fmt.Fprintf(out, "index preparation failed (search can retry): %v\n", err)
		}
	}()
	fmt.Fprintf(out, "Lumbrera MCP listening on %s/mcp\n", listener.Addr())
	select {
	case <-ctx.Done():
	case err = <-serveErr:
		stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		_ = httpServer.Close()
	}
	<-workerDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
