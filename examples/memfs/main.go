package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	aferofuse "github.com/unofs/afero-cgofuse"

	"github.com/spf13/afero"
	"github.com/winfsp/cgofuse/fuse"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s mount_point volume_name\n", os.Args[0])
		os.Exit(1)
	}

	mountpoint := os.Args[1]
	volumeName := os.Args[2]

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	memFs := afero.NewMemMapFs()
	afs := aferofuse.New(memFs, logger)

	host := fuse.NewFileSystemHost(afs)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		host.Unmount()
	}()

	fmt.Printf("mounting %s on %s\n", volumeName, mountpoint)
	ok := host.Mount(mountpoint, []string{"-o", "volname=" + volumeName})
	if !ok {
		fmt.Fprintln(os.Stderr, "mount failed")
		os.Exit(1)
	}
}
