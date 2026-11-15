// Package contentcheck implements finite, credential-free content inspection.
package contentcheck

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Policy struct {
	MaxPixels int64 `json:"max_pixels"`
	Edge      int   `json:"edge"`
	MaxBytes  int64 `json:"max_bytes"`
}
type Request struct {
	Protocol   int    `json:"protocol"`
	Operation  string `json:"operation"`
	RequestID  string `json:"request_id"`
	JobID      string `json:"job_id"`
	Generation string `json:"generation"`
	Fence      string `json:"fence"`
	Nonce      string `json:"attempt_nonce"`
	Directory  string `json:"directory"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Format     string `json:"format"`
	Policy     Policy `json:"policy"`
}
type Result struct {
	RequestID string `json:"request_id"`
	Nonce     string `json:"attempt_nonce"`
	Status    string `json:"status"`
	Code      string `json:"code"`
	Format    string `json:"format"`
	Preview   string `json:"preview,omitempty"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

func failed(r Request, code string) Result {
	return Result{RequestID: r.RequestID, Nonce: r.Nonce, Status: "failed", Code: code}
}
func Check(ctx context.Context, root string, r Request) Result {
	if r.Protocol != 1 || r.Size < 1 || r.Size > 100<<20 || len(r.SHA256) != 64 || !component(r.Directory) || !component(r.Input) || !component(r.Output) || r.Input == r.Output || r.Policy.MaxPixels < 1 || r.Policy.MaxPixels > 25000000 || r.Policy.Edge < 1 || r.Policy.Edge > 1600 || r.Policy.MaxBytes < 1 || r.Policy.MaxBytes > 2<<20 {
		return failed(r, "checker_invalid_request")
	}
	dir := filepath.Join(root, r.Directory)
	if !regular(dir, true) {
		return failed(r, "checker_invalid_path")
	}
	input := filepath.Join(dir, r.Input)
	if !regular(input, false) {
		return failed(r, "checker_invalid_path")
	}
	f, e := os.Open(input)
	if e != nil {
		return failed(r, "checker_input_unavailable")
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(&contextReader{ctx, f}, r.Size+1))
	if e != nil || n != r.Size || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return failed(r, "file_integrity_mismatch")
	}
	if _, e = f.Seek(0, 0); e != nil {
		return failed(r, "checker_input_unavailable")
	}
	result := Result{RequestID: r.RequestID, Nonce: r.Nonce, Status: "ok", Format: r.Format}
	if r.Operation == "validate" {
		if code := validate(ctx, f, r); code != "" {
			return failed(r, code)
		}
		return result
	}
	if r.Operation != "preview" || (r.Format != "png" && r.Format != "jpeg") {
		return failed(r, "checker_invalid_request")
	}
	cfg, format, e := image.DecodeConfig(f)
	if e != nil || format != r.Format {
		return failed(r, "preview_decode_failed")
	}
	if int64(cfg.Width)*int64(cfg.Height) > r.Policy.MaxPixels {
		return failed(r, "preview_pixel_limit")
	}
	if _, e = f.Seek(0, 0); e != nil {
		return failed(r, "preview_decode_failed")
	}
	img, _, e := image.Decode(&contextReader{ctx, f})
	if e != nil {
		return failed(r, "preview_decode_failed")
	}
	width, height := cfg.Width, cfg.Height
	if width > r.Policy.Edge || height > r.Policy.Edge {
		if width >= height {
			height = max(1, height*r.Policy.Edge/width)
			width = r.Policy.Edge
		} else {
			width = max(1, width*r.Policy.Edge/height)
			height = r.Policy.Edge
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := img.Bounds()
	for y := 0; y < height; y++ {
		if ctx.Err() != nil {
			return failed(r, "preview_timeout")
		}
		for x := 0; x < width; x++ {
			dst.Set(x, y, img.At(bounds.Min.X+x*cfg.Width/width, bounds.Min.Y+y*cfg.Height/height))
		}
	}
	// Re-encoding deliberately discards all source metadata. Alpha is retained for PNG.
	var buf bytes.Buffer
	if r.Format == "png" {
		e = png.Encode(&buf, dst)
	} else {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if dst.RGBAAt(x, y).A == 0 {
					dst.Set(x, y, color.White)
				}
			}
		}
		e = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
	}
	if e != nil || int64(buf.Len()) > r.Policy.MaxBytes {
		return failed(r, "preview_byte_limit")
	}
	out, e := os.OpenFile(filepath.Join(dir, r.Output), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return failed(r, "checker_output_unavailable")
	}
	_, e = out.Write(buf.Bytes())
	closeErr := out.Close()
	if e != nil || closeErr != nil {
		return failed(r, "checker_output_unavailable")
	}
	sum := sha256.Sum256(buf.Bytes())
	result.Preview = r.Output
	result.Size = int64(buf.Len())
	result.SHA256 = hex.EncodeToString(sum[:])
	result.Width = width
	result.Height = height
	return result
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}
func component(v string) bool {
	return v != "" && len(v) <= 100 && v != "." && v != ".." && !strings.ContainsAny(v, "/\\:\x00")
}
func regular(p string, dir bool) bool {
	st, e := os.Lstat(p)
	return e == nil && st.Mode()&os.ModeSymlink == 0 && ((dir && st.IsDir()) || (!dir && st.Mode().IsRegular()))
}
func validate(ctx context.Context, f *os.File, r Request) string {
	header := make([]byte, min(r.Size, 512))
	_, e := io.ReadFull(f, header)
	if e != nil {
		return "file_integrity_mismatch"
	}
	if _, e = f.Seek(0, 0); e != nil {
		return "checker_input_unavailable"
	}
	switch r.Format {
	case "png", "jpeg":
		cfg, format, e := image.DecodeConfig(f)
		if e != nil || format != r.Format || cfg.Width < 1 || cfg.Height < 1 {
			return "file_type_unsupported"
		}
	case "pdf":
		if !bytes.HasPrefix(header, []byte("%PDF-")) {
			return "file_type_unsupported"
		}
		if _, e = f.Seek(max(0, r.Size-1024), 0); e != nil {
			return "file_integrity_mismatch"
		}
		tail, e := io.ReadAll(io.LimitReader(f, 1024))
		if e != nil || !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
			return "file_type_unsupported"
		}
	case "txt", "csv":
		for _, prefix := range [][]byte{[]byte("MZ"), []byte("\x7fELF"), []byte("PK\x03\x04"), []byte("\x89PNG"), {0xff, 0xd8, 0xff}, []byte("%PDF-"), []byte("GIF8"), []byte("Rar!"), {0xd0, 0xcf, 0x11, 0xe0}} {
			if bytes.HasPrefix(header, prefix) {
				return "file_type_unsupported"
			}
		}
	case "docx", "xlsx", "pptx":
		return office(ctx, f, r)
	default:
		return "file_type_unsupported"
	}
	return ""
}
func office(ctx context.Context, f *os.File, r Request) string {
	archive, e := zip.NewReader(f, r.Size)
	if e != nil || len(archive.File) > 4096 || len(archive.File) < 2 {
		return "office_invalid_container"
	}
	expected := map[string]string{"docx": "/word/document.xml", "xlsx": "/xl/workbook.xml", "pptx": "/ppt/presentation.xml"}[r.Format]
	contentType := map[string]string{"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"}[r.Format]
	seen := map[string]bool{}
	var total int64
	hasType := false
	for _, entry := range archive.File {
		if ctx.Err() != nil {
			return "content_check_timeout"
		}
		name := entry.Name
		lower := strings.ToLower(name)
		if len(name) > 512 || name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || path.Clean(strings.TrimSuffix(name, "/")) != strings.TrimSuffix(name, "/") || strings.Contains("/"+name, "/../") || seen[lower] || entry.Mode()&os.ModeSymlink != 0 || entry.Flags&1 != 0 || (entry.Method != zip.Store && entry.Method != zip.Deflate) {
			return "office_unsafe_entry"
		}
		if strings.Contains(lower, "vbaproject") || strings.Contains(lower, "vbadata") || strings.Contains(lower, "macros/") {
			return "office_macro_rejected"
		}
		seen[lower] = true
		limit := int64(64 << 20)
		if name == "[Content_Types].xml" || strings.HasSuffix(lower, ".rels") {
			limit = 2 << 20
		}
		if entry.UncompressedSize64 > uint64(limit) {
			return "office_expansion_limit"
		}
		reader, e := entry.Open()
		if e != nil {
			return "office_invalid_container"
		}
		var data []byte
		var n int64
		if name == "[Content_Types].xml" {
			data, e = io.ReadAll(io.LimitReader(&contextReader{ctx, reader}, limit+1))
			n = int64(len(data))
		} else {
			n, e = io.Copy(io.Discard, io.LimitReader(&contextReader{ctx, reader}, limit+1))
		}
		closeErr := reader.Close()
		total += n
		if n > limit || total > 256<<20 {
			return "office_expansion_limit"
		}
		if e != nil || closeErr != nil {
			return "office_invalid_container"
		}
		if name == "[Content_Types].xml" {
			decoder := xml.NewDecoder(bytes.NewReader(data))
			for {
				token, e := decoder.Token()
				if errors.Is(e, io.EOF) {
					break
				}
				if e != nil {
					return "office_invalid_types"
				}
				if start, ok := token.(xml.StartElement); ok {
					part, typ := "", ""
					for _, a := range start.Attr {
						if a.Name.Local == "PartName" {
							part = a.Value
						}
						if a.Name.Local == "ContentType" {
							typ = a.Value
						}
					}
					if strings.Contains(strings.ToLower(typ), "macroenabled") || strings.Contains(strings.ToLower(typ), "vbaproject") {
						return "office_macro_rejected"
					}
					if start.Name.Local == "Override" && part == expected && typ == contentType {
						hasType = true
					}
				}
			}
		}
	}
	if !hasType || !seen[strings.TrimPrefix(expected, "/")] {
		return "office_type_mismatch"
	}
	return ""
}
