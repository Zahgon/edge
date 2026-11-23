// Package tablegen holds what the table generators of the module (the gen
// directories of the internal packages, each run with `go generate`) share:
// reading Unicode Character Database files, reading the JSON recordings the
// Node recorders write, and writing a formatted Go file.
package tablegen

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Open opens a data file: a local path, or an http(s) URL that is downloaded.
func Open(name string) (io.ReadCloser, error) {
	if !strings.HasPrefix(name, "https://") && !strings.HasPrefix(name, "http://") {
		return os.Open(name)
	}
	resp, err := http.Get(name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("GET %s: %s", name, resp.Status)
		return nil, errors.Join(err, resp.Body.Close())
	}
	return resp.Body, nil
}

// ReadRecords calls fn with the semicolon-separated fields, trimmed of spaces,
// of every line of the Unicode data file name that is not blank or a comment.
// A "#" starts a comment anywhere on a line.
func ReadRecords(name string, fn func(fields []string) error) (err error) {
	f, err := Open(name)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for line := 1; sc.Scan(); line++ {
		text, _, _ := strings.Cut(sc.Text(), "#")
		if strings.TrimSpace(text) == "" {
			continue
		}
		fields := strings.Split(text, ";")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		if err := fn(fields); err != nil {
			return fmt.Errorf("%s:%d: %w", name, line, err)
		}
	}
	return sc.Err()
}

// ParseRange parses a code point field, "0041" or the range "0041..005A".
func ParseRange(field string) (lo, hi rune, err error) {
	first, last, isRange := strings.Cut(field, "..")
	lo, err = ParseCodePoint(first)
	if err != nil || !isRange {
		return lo, lo, err
	}
	hi, err = ParseCodePoint(last)
	return lo, hi, err
}

// ParseCodePoint parses a code point written in hex, as in "00E9".
func ParseCodePoint(field string) (rune, error) {
	n, err := strconv.ParseUint(field, 16, 32)
	if err != nil || n > 0x10FFFF {
		return 0, fmt.Errorf("bad code point %q", field)
	}
	return rune(n), nil
}

// ParseCodePoints parses a space-separated sequence of code points, as in
// "0041 0301".
func ParseCodePoints(field string) ([]rune, error) {
	var out []rune
	for _, f := range strings.Fields(field) {
		r, err := ParseCodePoint(f)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// ReadJSON decodes the JSON file name into v.
func ReadJSON(name string, v any) (err error) {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	return json.NewDecoder(f).Decode(v)
}

// WriteGo formats src as gofmt does and writes it to the file name.
func WriteGo(name string, src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("formatting %s: %w", name, err)
	}
	return os.WriteFile(name, formatted, 0o644)
}
