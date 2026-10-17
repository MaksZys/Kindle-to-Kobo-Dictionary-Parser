package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeDictfileKeepsKindleEntries(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<h1>Front matter</h1><p>Do not index this.</p>`)
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&source, `<div class="entry"><span><span class="bold">word%d</span></span> <span class="italic">n</span> znaczenie %d</div><blockquote>example %d</blockquote><hr/>`, i, i, i)
	}
	dictfile, count, err := makeDictfile([]byte(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Fatalf("got %d entries, want 10", count)
	}
	text := string(dictfile)
	if !strings.Contains(text, "@ word9\n::\n<html><div><b>word9</b> <i>n</i> znaczenie 9</div><blockquote>example 9</blockquote>") {
		t.Fatalf("last entry or definition missing: %s", text)
	}
	if strings.Contains(text, "Front matter") {
		t.Fatal("front matter appeared in dictionary")
	}
}

func TestNormalizeCalibreHTML(t *testing.T) {
	styles := []byte(`.calibre4 { font-weight: bold } .calibre5 { font-style: italic }`)
	source := []byte(`<div class="entry"><span><span class="calibre4">word</span></span> <span class="calibre5">n</span></div><hr class="calibre11" />`)
	got := string(normalizeHTML(source, styles))
	want := `<div class="entry"><span><span class="bold">word</span></span> <span class="italic">n</span></div><hr/>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConvertWithoutDictgenInPath(t *testing.T) {
	work := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) })

	fixture := filepath.Join(work, "fixture.htmlz")
	file, err := os.Create(fixture)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	htmlFile, err := archive.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		fmt.Fprintf(htmlFile, `<div class="entry"><span><span class="headword">word%d</span></span> definition %d</div><hr class="rule" />`, i, i)
	}
	styleFile, err := archive.Create("style.css")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(styleFile, `.headword { font-weight: bold }`)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(work, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\n/bin/cp \"$FIXTURE_HTMLZ\" \"$2\"\nprintf '%s' \"$2\" > \"$CAPTURE_PATH\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ebook-convert"), []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("FIXTURE_HTMLZ", fixture)
	capture := filepath.Join(work, "capture.txt")
	t.Setenv("CAPTURE_PATH", capture)

	if err := convert("dictionary.mobi", "dicthtml-en-pl.zip"); err != nil {
		t.Fatal(err)
	}
	path, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(string(path))) != cwd {
		t.Fatalf("temporary output outside working directory: %s", path)
	}
	leftovers, err := filepath.Glob(filepath.Join(work, ".kindle-to-kobo-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files remain: %v", leftovers)
	}
	result, err := zip.OpenReader("dicthtml-en-pl.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	for _, entry := range result.File {
		if entry.Name == "words" {
			return
		}
	}
	t.Fatal("Kobo word index missing")
}
