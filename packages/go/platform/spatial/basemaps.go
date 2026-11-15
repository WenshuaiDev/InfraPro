package spatial

import (
	"bytes"
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
)

type Basemap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	Attribution string `json:"attribution"`
	MinZoom     int    `json:"min_zoom"`
	MaxZoom     int    `json:"max_zoom"`
}

func (s *Service) WithBasemaps(b []Basemap) *Service {
	s.basemaps = append([]Basemap{}, b...)
	return s
}
func (s *Service) knownBasemap(id string) bool {
	if id == "" || id == "blank" || id == "local" {
		return true
	}
	for _, b := range s.basemaps {
		if b.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) Basemaps(ctx context.Context, a access.Actor, scope Scope) (List[Basemap], error) {
	out := List[Basemap]{Items: []Basemap{}}
	e := s.transact(ctx, a, scope, "basemaps", false, func(tx pgx.Tx, auth k.Authorization) error {
		out.Items = []Basemap{{ID: "blank", Name: "空白背景", Attribution: ""}, {ID: "local", Name: "本地示意底图", URL: fmt.Sprintf("/api/v1/tenants/%s%s/spatial/basemaps/local/tiles/{z}-{x}-{y}.png", scope.Tenant, func() string {
			if scope.Project != "" {
				return "/projects/" + scope.Project
			}
			return ""
		}()), Attribution: "InfraPro 合成示例 · 非测绘资料", MinZoom: 0, MaxZoom: 18}}
		out.Items = append(out.Items, s.basemaps...)
		return nil
	})
	return out, e
}

// LocalTile is a reproducible XYZ background, deliberately labelled as synthetic
// by its catalog attribution. It never requests a third-party map or fabricates resources.
func LocalTile(raw string) ([]byte, error) {
	parts := strings.Split(strings.TrimSuffix(raw, ".png"), "-")
	if len(parts) != 3 {
		return nil, notFound()
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, e := strconv.Atoi(p)
		if e != nil || n < 0 {
			return nil, notFound()
		}
		nums[i] = n
	}
	if nums[0] > 18 || nums[1] >= 1<<nums[0] || nums[2] >= 1<<nums[0] {
		return nil, notFound()
	}
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			c := color.RGBA{237, 242, 239, 255}
			if x%64 == 0 || y%64 == 0 {
				c = color.RGBA{205, 217, 213, 255}
			}
			if x > 100+y/6 && x < 115+y/6 {
				c = color.RGBA{191, 215, 230, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	e := png.Encode(&b, img)
	return b.Bytes(), e
}
