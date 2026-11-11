// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command tecto runs the tecto package and writes the world it returns to a
// file, for inspection.
//
//	tecto --seed 42 --myr 150 --detail 6 --columns 1920 --rows 800 --output world.tecto
//	tecto --seed 42 --myr 150 --png world.png
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/maloquacious/tecto"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr)
	stop()
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
	default:
		fmt.Fprintln(os.Stderr, "tecto:", err)
		os.Exit(1)
	}
}

// run is the command: args are the command line without the program's
// name, and progress and the summary go to stderr.
func run(ctx context.Context, args []string, stderr io.Writer) error {
	var p tecto.Params
	var output, png string
	var showVersion bool
	fs := flag.NewFlagSet("tecto", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Uint64Var(&p.Seed, "seed", 0, "seed for the world")
	fs.Float64Var(&p.Myr, "myr", 0, "how long the plates run, in millions of years; 0 writes the starting crust")
	fs.IntVar(&p.Detail, "detail", tecto.DefaultDetail, "size of the model's cells: the sphere has 10·4^detail + 2")
	fs.IntVar(&p.Columns, "columns", tecto.DefaultColumns, "columns of hexes in the map; must be even")
	fs.IntVar(&p.Rows, "rows", tecto.DefaultRows, "rows of hexes in the map")
	fs.StringVar(&output, "output", "", "write the heights to this `file`")
	fs.StringVar(&png, "png", "", "write a picture of the heights to this `file`")
	fs.BoolVar(&showVersion, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if showVersion {
		fmt.Fprintln(stderr, tecto.Version())
		return nil
	}

	fmt.Fprintf(stderr, "tecto %v: seed %d, %v Myr, detail %d, %d by %d hexes\n",
		tecto.Version(), p.Seed, p.Myr, p.Detail, p.Columns, p.Rows)
	started := time.Now()

	// The package has no way to be interrupted, so it runs to one side and
	// an interrupt abandons it.
	type outcome struct {
		result *tecto.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := tecto.Generate(p)
		done <- outcome{r, err}
	}()
	var r *tecto.Result
	select {
	case <-ctx.Done():
		return errors.New("interrupted")
	case o := <-done:
		if o.err != nil {
			return o.err
		}
		r = o.result
	}

	s := r.Summary
	fmt.Fprintf(stderr, "cells          %d\n", s.Cells)
	if s.Plates > 0 {
		fmt.Fprintf(stderr, "plates         %d, overlapping over %.3f of the sphere\n", s.Plates, s.OverlapFraction)
		b := s.Continental
		fmt.Fprintf(stderr, "continents     %.4g kg at the start: %.3f%% subducted, %.3f%% accreted, %+.3f%% from resampling, %.4g kg at the end\n",
			b.Start, 100*b.Subducted/b.Start, 100*b.Accreted/b.Start, 100*b.Resampled/b.Start, b.End)
	}
	fmt.Fprintf(stderr, "land fraction  %.3f in %d landmasses\n", s.LandFraction, s.Landmasses)
	fmt.Fprintf(stderr, "high ground    %.2f times as long as wide", s.HighGroundElongation)
	if s.Plates > 0 {
		fmt.Fprintf(stderr, ", %.0f km from a plate margin, where land is %.0f km", s.HighGroundToMarginKm, s.LandToMarginKm)
	}
	fmt.Fprintln(stderr)
	fmt.Fprintf(stderr, "height, m      min %.0f, 5%% %.0f, 25%% %.0f, 50%% %.0f, 75%% %.0f, 95%% %.0f, max %.0f\n",
		s.Min, s.P5, s.P25, s.P50, s.P75, s.P95, s.Max)

	if output != "" {
		if err := writeFile(output, encodeHeights(p, r)); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "wrote %s\n", output)
	}
	if png != "" {
		data, err := encodePNG(r)
		if err != nil {
			return err
		}
		if err := writeFile(png, data); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "wrote %s\n", png)
	}
	fmt.Fprintf(stderr, "elapsed        %v\n", time.Since(started).Round(time.Millisecond))
	return nil
}

// writeFile writes data to a file by way of a temporary file beside it, so
// that a failure does not leave half a file under the name asked for.
func writeFile(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), name)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
