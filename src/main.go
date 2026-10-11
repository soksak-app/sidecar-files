// The files sidecar receives requests on the standard input and sends the replies on the standard output. The format is defined
// in docs/spec/sidecars.md of the soksak core repository. The sidecar ends when the standard input closes.
//
// The argument --log-dir is the folder of the event file files-<boot>.jsonl, which the host names as the logs folder of the
// configuration directory.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/soksak-app/sidecar-files/src/events"
	"github.com/soksak-app/sidecar-files/src/files"
	"github.com/soksak-app/sidecar-files/src/platform"
	"github.com/soksak-app/sidecar-files/src/routine"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run starts the event log, serves the requests and returns the exit status: 0 when the input ended, 1 for a failure that
// ended the sidecar and 2 for an argument that is not valid.
func run(args []string, in io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("files sidecar", flag.ContinueOnError)
	flags.SetOutput(stderr)
	logDir := flags.String("log-dir", "", "the folder of the event file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "files sidecar: unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	var log *events.Log
	if *logDir == "" {
		fmt.Fprintln(stderr, "files sidecar: started without --log-dir, so no event file is written")
	} else {
		current, err := platform.Current()
		if err != nil {
			fmt.Fprintf(stderr, "files sidecar: %v\n", err)
			return 1
		}
		if log, err = events.Start(events.DefaultOptions(*logDir, "files", current.MonotonicNanos)); err != nil {
			fmt.Fprintf(stderr, "files sidecar: event file: %v\n", err)
			return 1
		}
	}
	routine.Use(log)
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": os.Getpid(), "goos": runtime.GOOS, "goarch": runtime.GOARCH})
	status := 0
	if err := files.Serve(in, out); err != nil {
		fmt.Fprintf(stderr, "files sidecar: %v\n", err)
		log.EmitHost(events.LevelError, "sidecar.files.failure", map[string]any{"error": err.Error()})
		status = 1
	}
	log.EmitHost(events.LevelInfo, "sidecar.files.exit", map[string]any{"status": status})
	if err := log.Flush(events.ExitDeadline); err != nil {
		fmt.Fprintf(stderr, "files sidecar: %v\n", err)
	}
	log.Close()
	return status
}
