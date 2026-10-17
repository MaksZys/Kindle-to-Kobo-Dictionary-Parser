# Kindle dictionary to Kobo

This Go CLI converts Kindle `.mobi` dictionaries into Kobo `dicthtml-*.zip` dictionaries. It uses Calibre to extract the MOBI content, then builds Kobo's HTML files and word index.

## Requirements

- Go 1.22 or newer
- [Calibre](https://calibre-ebook.com/) with `ebook-convert` on `PATH`

## Convert

```sh
go build -o kindle-to-kobo .
./kindle-to-kobo -locale xx-yy path/to/dictionary.mobi
```

Replace `xx-yy` with the dictionary's two-letter source and target language codes. For a single-language dictionary, use one two-letter code. The locale is required because it cannot be inferred reliably from every MOBI file.

The default output is `dicthtml-LOCALE.zip` in the directory where you run the command. Temporary conversion files are created there and removed when the run ends. Use `-o path/to/output.zip` to choose another output path. The tool refuses to overwrite an existing output.

Copy the generated ZIP to `.kobo/custom-dict/` on the Kobo. The output filename should keep the `dicthtml-LOCALE.zip` form.

The extractor identifies repeated Calibre HTML entries whose headword is the first bold span in a definition block. It fails with an error when it cannot recognize that layout. Kindle dictionaries with different internal entry markup may need a specific extractor; producing a ZIP without correct headwords would make lookups fail silently on Kobo.
