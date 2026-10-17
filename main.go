package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pgaskin/dictutil/dictgen"
	"github.com/pgaskin/dictutil/kobodict"
	_ "github.com/pgaskin/dictutil/kobodict/marisa"
)

var (
	entryStart = regexp.MustCompile(`<div class="([^"]+)">\s*<span[^>]*>\s*<span class="bold">([^<]+)</span>`)
	cssRule    = regexp.MustCompile(`(?s)\.([A-Za-z0-9_-]+)\s*\{([^}]+)\}`)
	hrTag      = regexp.MustCompile(`(?i)<hr\b[^>]*>`)
	boldSpan   = regexp.MustCompile(`(?s)<span class="bold">(.*?)</span>`)
	italicSpan = regexp.MustCompile(`(?s)<span class="italic">(.*?)</span>`)
	otherSpan  = regexp.MustCompile(`</?span(?: [^>]*)?>`)
	linkStart  = regexp.MustCompile(`<a(?: [^>]*)?>`)
	attribute  = regexp.MustCompile(` (?:class|id|href)="[^"]*"`)
	space      = regexp.MustCompile(`\s+`)
	locale     = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]{2})?$`)
)

func main() {
	var output, language string
	flag.StringVar(&output, "o", "", "output Kobo ZIP (default: dicthtml-LOCALE.zip)")
	flag.StringVar(&language, "locale", "", "Kobo language code, e.g. en-pl")
	flag.Parse()
	if flag.NArg() != 1 || !locale.MatchString(language) {
		fmt.Fprintln(os.Stderr, "usage: kindle-to-kobo -locale en-pl [-o dicthtml-en-pl.zip] dictionary.mobi")
		os.Exit(2)
	}
	if output == "" {
		output = "dicthtml-" + language + ".zip"
	}
	if err := convert(flag.Arg(0), output); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func convert(input, output string) (err error) {
	if strings.ToLower(filepath.Ext(input)) != ".mobi" {
		return fmt.Errorf("input must be a .mobi file: %s", input)
	}
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("output already exists: %s", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp(cwd, ".kindle-to-kobo-")
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := os.RemoveAll(work); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temporary files: %w", cleanupErr))
		}
	}()

	htmlz := filepath.Join(work, "source.htmlz")
	cmd := exec.Command("ebook-convert", input, htmlz)
	cmd.Env = append(os.Environ(), "TMPDIR="+work)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Calibre extraction failed: %w", err)
	}
	source, styles, err := readHTML(htmlz)
	if err != nil {
		return err
	}
	dictfile, count, err := makeDictfile(normalizeHTML(source, styles))
	if err != nil {
		return err
	}
	if err := writeDictionary(output, dictfile); err != nil {
		return fmt.Errorf("Kobo packing failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d entries to %s\n", count, output)
	return nil
}

func writeDictionary(output string, dictfile []byte) (err error) {
	entries, err := dictgen.ParseDictFile(bytes.NewReader(dictfile))
	if err != nil {
		return err
	}
	if err := entries.Validate(); err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			if removeErr := os.Remove(output); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove partial output: %w", removeErr))
			}
		}
	}()
	writer := kobodict.NewWriter(file)
	if err := entries.WriteDictzip(writer, new(dictgen.ImageHandlerRemove), dictgen.ImageFuncFilesystem); err != nil {
		return err
	}
	return writer.Close()
}

func readHTML(path string) ([]byte, []byte, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open Calibre HTMLZ: %w", err)
	}
	defer archive.Close()
	var largest *zip.File
	var stylesheet *zip.File
	for _, file := range archive.File {
		if strings.EqualFold(filepath.Ext(file.Name), ".html") && (largest == nil || file.UncompressedSize64 > largest.UncompressedSize64) {
			largest = file
		}
		if strings.EqualFold(filepath.Ext(file.Name), ".css") {
			stylesheet = file
		}
	}
	if largest == nil {
		return nil, nil, errors.New("Calibre HTMLZ contains no HTML")
	}
	reader, err := largest.Open()
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	source, err := io.ReadAll(reader)
	if err != nil || stylesheet == nil {
		return source, nil, err
	}
	reader, err = stylesheet.Open()
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	styles, err := io.ReadAll(reader)
	return source, styles, err
}

func normalizeHTML(source, styles []byte) []byte {
	for _, rule := range cssRule.FindAllSubmatch(styles, -1) {
		declarations := string(rule[2])
		style := ""
		if strings.Contains(declarations, "font-weight: bold") || strings.Contains(declarations, "font-weight: 700") {
			style = "bold"
		} else if strings.Contains(declarations, "font-style: italic") {
			style = "italic"
		}
		if style != "" {
			source = bytes.ReplaceAll(source, []byte(`class="`+string(rule[1])+`"`), []byte(`class="`+style+`"`))
		}
	}
	return hrTag.ReplaceAll(source, []byte("<hr/>"))
}

func makeDictfile(source []byte) ([]byte, int, error) {
	matches := entryStart.FindAllSubmatchIndex(source, -1)
	if len(matches) == 0 {
		return nil, 0, errors.New("no Kindle entries found in Calibre HTML; unsupported dictionary layout")
	}
	frequency := map[string]int{}
	for _, match := range matches {
		frequency[string(source[match[2]:match[3]])]++
	}
	mainClass := ""
	for class, count := range frequency {
		if count > frequency[mainClass] {
			mainClass = class
		}
	}
	if frequency[mainClass] < 10 {
		return nil, 0, errors.New("could not identify repeated Kindle dictionary entries")
	}

	var output bytes.Buffer
	count := 0
	for i, match := range matches {
		if string(source[match[2]:match[3]]) != mainClass {
			continue
		}
		end := len(source)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		if boundary := bytes.Index(source[match[1]:end], []byte("<hr/>")); boundary >= 0 {
			end = match[1] + boundary
		}
		word := strings.TrimSpace(html.UnescapeString(string(source[match[4]:match[5]])))
		if word == "" || strings.ContainsAny(word, "\r\n") {
			continue
		}
		body := cleanHTML(string(source[match[0]:end]))
		fmt.Fprintf(&output, "@ %s\n::\n<html>%s\n\n", word, body)
		count++
	}
	if count == 0 {
		return nil, 0, errors.New("no usable dictionary entries found")
	}
	return output.Bytes(), count, nil
}

func cleanHTML(body string) string {
	body = boldSpan.ReplaceAllString(body, "<b>$1</b>")
	body = italicSpan.ReplaceAllString(body, "<i>$1</i>")
	body = otherSpan.ReplaceAllString(body, "")
	body = linkStart.ReplaceAllString(body, "")
	body = strings.ReplaceAll(body, "</a>", "")
	body = attribute.ReplaceAllString(body, "")
	body = space.ReplaceAllString(body, " ")
	return strings.TrimSpace(body)
}
