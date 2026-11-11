// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maloquacious/tecto"
)

// smallWorld is the flags for a world that takes a few milliseconds.
var smallWorld = []string{"--seed", "42", "--detail", "4", "--columns", "64", "--rows", "32"}

func TestRunWritesTheSameFileEveryTime(t *testing.T) {
	dir := t.TempDir()
	var files [2][]byte
	for i := range files {
		name := filepath.Join(dir, "world.tecto")
		if err := run(t.Context(), append(smallWorld, "--output", name), io.Discard); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		files[i] = data
	}
	if !bytes.Equal(files[0], files[1]) {
		t.Error("the same flags wrote different files")
	}
	// Nothing is left beside the file.
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files in the directory, want 1", len(entries))
	}

	other := filepath.Join(dir, "other.tecto")
	if err := run(t.Context(), append(smallWorld, "--seed", "43", "--output", other), io.Discard); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(other); bytes.Equal(files[0], data) {
		t.Error("different seeds wrote the same file")
	}
}

func TestHeightsFileLayout(t *testing.T) {
	name := filepath.Join(t.TempDir(), "world.tecto")
	if err := run(t.Context(), append(smallWorld, "--output", name), io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 40+4*64*32 {
		t.Fatalf("file is %d bytes, want %d", len(data), 40+4*64*32)
	}
	le := binary.LittleEndian
	if string(data[:8]) != "TECTOHGT" {
		t.Errorf("magic %q", data[:8])
	}
	for what, c := range map[string]struct {
		got, want uint64
	}{
		"version": {uint64(le.Uint32(data[8:])), 1},
		"columns": {uint64(le.Uint32(data[12:])), 64},
		"rows":    {uint64(le.Uint32(data[16:])), 32},
		"detail":  {uint64(le.Uint32(data[20:])), 4},
		"seed":    {le.Uint64(data[24:]), 42},
		"myr":     {le.Uint64(data[32:]), math.Float64bits(0)},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, want %d", what, c.got, c.want)
		}
	}
	// The heights are the package's, in order.
	r, err := tecto.Generate(tecto.Params{Seed: 42, Detail: 4, Columns: 64, Rows: 32})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range r.Heights {
		if got := math.Float32frombits(le.Uint32(data[40+4*i:])); got != want {
			t.Fatalf("hex %d is %v in the file, %v from the package", i, got, want)
		}
	}
}

func TestRunWritesAPicture(t *testing.T) {
	name := filepath.Join(t.TempDir(), "world.png")
	if err := run(t.Context(), append(smallWorld, "--png", name), io.Discard); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 32 {
		t.Errorf("picture is %d by %d, want 64 by 32", b.Dx(), b.Dy())
	}
}

func TestColorAt(t *testing.T) {
	// Land is not the color of the sea, and the ends hold.
	if colorAt(0) == colorAt(1) {
		t.Error("the coast does not show")
	}
	if colorAt(-20000) != colorAt(-6000) || colorAt(20000) != colorAt(6000) {
		t.Error("heights beyond the ends do not take the ends' colors")
	}
	if c := colorAt(-3000); c.R != 20 || c.G != 60 || c.B != 130 {
		t.Errorf("-3000 m is %v", c)
	}
	if c := colorAt(500); c.R != 105 || c.G != 155 || c.B != 85 {
		t.Errorf("500 m is %v", c)
	}
}

func TestRunSummary(t *testing.T) {
	var out strings.Builder
	if err := run(t.Context(), smallWorld, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"seed 42", "cells          2562", "land fraction", "height, m", "elapsed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	if err := run(t.Context(), []string{"--version"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != tecto.Version().String() {
		t.Errorf("--version printed %q, want %q", got, tecto.Version())
	}
}

func TestRunFailsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "world.tecto")
	for what, args := range map[string][]string{
		"an unknown flag":     {"--nonesuch"},
		"a stray argument":    {"world.tecto"},
		"an odd column count": {"--columns", "65", "--output", name},
		"time, for now":       {"--myr", "100", "--output", name},
	} {
		if err := run(t.Context(), args, io.Discard); err == nil {
			t.Errorf("%s: no error", what)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a failed run left %d files", len(entries))
	}
}
