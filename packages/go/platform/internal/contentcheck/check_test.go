package contentcheck

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func run(t *testing.T, data []byte, format, operation string) Result {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "attempt")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "input"), data, 0600); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(data)
	return Check(context.Background(), root, Request{Protocol: 1, Operation: operation, RequestID: "test", Nonce: "one", Directory: "attempt", Input: "input", Output: "preview", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Format: format, Policy: Policy{MaxPixels: 25000000, Edge: 1600, MaxBytes: 2 << 20}})
}
func officeFile(t *testing.T, format string, extra map[string][]byte) []byte {
	t.Helper()
	main := map[string]string{"docx": "word/document.xml", "xlsx": "xl/workbook.xml", "pptx": "ppt/presentation.xml"}[format]
	typ := map[string]string{"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"}[format]
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	entries := map[string][]byte{"[Content_Types].xml": []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/` + main + `" ContentType="` + typ + `"/></Types>`), main: []byte("<document/>")}
	for k, v := range extra {
		entries[k] = v
	}
	for k, v := range entries {
		w, e := z.Create(k)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(v); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return buf.Bytes()
}
func TestContentFormatsAndDeceptiveBodies(t *testing.T) {
	for _, format := range []string{"docx", "xlsx", "pptx"} {
		t.Run(format, func(t *testing.T) {
			if got := run(t, officeFile(t, format, nil), format, "validate"); got.Status != "ok" {
				t.Fatal(got)
			}
		})
	}
	for _, test := range []struct {
		name, format string
		data         []byte
		ok           bool
	}{
		{"one byte", "txt", []byte("x"), true}, {"csv", "csv", []byte("id,value\n1,2\n"), true}, {"pdf", "pdf", []byte("%PDF-1.7\n1 0 obj\nendobj\n%%EOF\n"), true},
		{"renamed executable", "txt", []byte("MZ executable"), false}, {"renamed zip", "csv", officeFile(t, "docx", nil), false}, {"wrong document kind", "docx", officeFile(t, "xlsx", nil), false}, {"incomplete PDF", "pdf", []byte("%PDF-1.7 only header"), false},
		{"traversal", "docx", officeFile(t, "docx", map[string][]byte{"../escape": []byte("x")}), false}, {"backslash", "xlsx", officeFile(t, "xlsx", map[string][]byte{"a\\..\\escape": []byte("x")}), false}, {"macro", "docx", officeFile(t, "docx", map[string][]byte{"word/vbaProject.bin": []byte("macro")}), false}, {"zip expansion", "xlsx", officeFile(t, "xlsx", map[string][]byte{"xl/oversized.xml": bytes.Repeat([]byte("x"), 64<<20+1)}), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := run(t, test.data, test.format, "validate")
			if (got.Status == "ok") != test.ok {
				t.Fatal(got)
			}
		})
	}
}
func TestImagePreviewIsNewContentAndSourceValidationIndependent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1800, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1800; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 200), uint8(y % 200), 25, 255})
		}
	}
	for _, format := range []string{"png", "jpeg"} {
		var b bytes.Buffer
		if format == "png" {
			if e := png.Encode(&b, img); e != nil {
				t.Fatal(e)
			}
		} else {
			if e := jpeg.Encode(&b, img, nil); e != nil {
				t.Fatal(e)
			}
		}
		v := run(t, b.Bytes(), format, "validate")
		p := run(t, b.Bytes(), format, "preview")
		if v.Status != "ok" || p.Status != "ok" || p.Width != 1600 || p.Height != 800 || p.Size > 2<<20 {
			t.Fatal(v, p)
		}
		sum := sha256.Sum256(b.Bytes())
		if p.SHA256 == hex.EncodeToString(sum[:]) {
			t.Fatal("preview reused original body")
		}
	}
	var b bytes.Buffer
	if e := png.Encode(&b, img); e != nil {
		t.Fatal(e)
	}
	header := append([]byte{}, b.Bytes()[:33]...)
	// A valid dimension header with a missing image stream is a valid original
	// signature, but it must never be reported as a working preview.
	if v := run(t, header, "png", "validate"); v.Status != "ok" {
		t.Fatal(v)
	}
	if v := run(t, header, "png", "preview"); v.Status != "failed" || v.Code != "preview_decode_failed" {
		t.Fatal(v)
	}
	binary.BigEndian.PutUint32(header[16:20], 100000)
	binary.BigEndian.PutUint32(header[20:24], 100000)
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	if v := run(t, header, "png", "preview"); v.Code != "preview_pixel_limit" {
		t.Fatal(v)
	}
}
func TestOfficeDuplicateEncryptionSymlinkAndEntryBudget(t *testing.T) {
	for _, kind := range []string{"duplicate", "encrypted", "symlink", "entries"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			if kind == "entries" {
				for n := 0; n < 4097; n++ {
					_, e := z.Create(string(rune(n + 0x1000)))
					if e != nil {
						t.Fatal(e)
					}
				}
			} else {
				header := &zip.FileHeader{Name: "test", Method: zip.Store}
				if kind == "symlink" {
					header.SetMode(os.ModeSymlink | 0777)
				}
				w, e := z.CreateHeader(header)
				if e != nil {
					t.Fatal(e)
				}
				_, _ = w.Write([]byte("x"))
				if kind == "duplicate" {
					_, _ = z.Create("test")
				} else {
					_, _ = z.Create("two")
				}
			}
			if e := z.Close(); e != nil {
				t.Fatal(e)
			}
			data := b.Bytes()
			if kind == "encrypted" {
				at := bytes.Index(data, []byte("PK\x01\x02"))
				if at < 0 {
					t.Fatal("central entry missing")
				}
				data[at+8] |= 1
			}
			if v := run(t, data, "docx", "validate"); v.Status != "failed" {
				t.Fatal(v)
			}
		})
	}
}
