package contentcheck

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestAvatarRejectsMalformedAndOversizedInput(t *testing.T) {
	for _, input := range [][]byte{nil, []byte("<svg/>"), bytes.Repeat([]byte{1}, AvatarInputBytes+1), []byte("GIF89a")} {
		if _, e := Avatar(context.Background(), input); e == nil {
			t.Fatal("invalid image accepted")
		}
	}
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{20, 40, 60, 255})
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	huge := bytes.Clone(b.Bytes())
	binary.BigEndian.PutUint32(huge[16:20], 50000)
	binary.BigEndian.PutUint32(huge[20:24], 50000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	if _, e := Avatar(context.Background(), huge); e == nil {
		t.Fatal("pixel bomb accepted")
	}
	if _, e := Avatar(context.Background(), b.Bytes()[:len(b.Bytes())/2]); e == nil {
		t.Fatal("truncated image accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Avatar(ctx, b.Bytes()); e == nil {
		t.Fatal("canceled processing succeeded")
	}
	input := append(bytes.Clone(b.Bytes()), []byte("PRIVATE EXIF LOCATION")...)
	out, e := Avatar(context.Background(), input)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(out, []byte("PRIVATE")) {
		t.Fatal("metadata retained")
	}
}
