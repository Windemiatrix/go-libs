// Command cmd writes a synthetic Photos library:
//
//	go run ./internal/gen/cmd -out <dir> [-n 1000] [-seed 1]
//
// Everything else comes from gen.DefaultConfig. It prints nothing on success;
// a generation error goes to stderr with exit code 1, a usage error with
// exit code 2.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/gen"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stderr)
	stop()
	os.Exit(code)
}

// run parses the flags and calls gen.Generate; it returns the exit code.
func run(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "directory of the library; it must not exist or must be empty (required)")
	n := flags.Int("n", 1000, "number of assets")
	seed := flags.Uint64("seed", 1, "seed of the generator")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	usage := func(msg string) int {
		_, _ = fmt.Fprintln(stderr, msg)
		flags.Usage()
		return 2
	}
	if flags.NArg() > 0 {
		return usage(fmt.Sprintf("gen: unexpected arguments: %q", flags.Args()))
	}
	if *out == "" {
		return usage("gen: -out is required")
	}
	cfg := gen.DefaultConfig()
	cfg.Assets, cfg.Seed = *n, *seed
	if err := gen.Generate(ctx, *out, cfg); err != nil {
		// The errors of gen start with "gen: ".
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
