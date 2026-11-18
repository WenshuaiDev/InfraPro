package contentcheck

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
)

const AvatarInputBytes = 5 << 20
const AvatarEdge = 256

// Avatar returns only newly encoded pixels. Neither uploaded metadata nor an
// uploaded URL is ever served. The UI supplies its cropped square; direct API
// clients get a centered square. Decode work must be bounded by the caller.
func Avatar(ctx context.Context, input []byte) ([]byte, error) {
	if len(input) == 0 || len(input) > AvatarInputBytes {
		return nil, fmt.Errorf("avatar_size")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, fmt.Errorf("avatar_format")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 25000000/cfg.Height {
		return nil, fmt.Errorf("avatar_pixels")
	}
	img, _, err := image.Decode(&contextReader{ctx, bytes.NewReader(input)})
	if err != nil {
		return nil, fmt.Errorf("avatar_decode")
	}
	edge := min(cfg.Width, cfg.Height)
	left, top := (cfg.Width-edge)/2, (cfg.Height-edge)/2
	out := image.NewNRGBA(image.Rect(0, 0, AvatarEdge, AvatarEdge))
	for y := 0; y < AvatarEdge; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < AvatarEdge; x++ {
			out.Set(x, y, img.At(left+x*edge/AvatarEdge, top+y*edge/AvatarEdge))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
