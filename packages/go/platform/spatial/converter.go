package spatial

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type PythonConverter struct {
	Python, Script string
	Timeout        time.Duration
}

func (c PythonConverter) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Python, append([]string{"-I", c.Script}, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "OPENBLAS_NUM_THREADS=1", "OMP_NUM_THREADS=1"}
	return cmd
}
func (c PythonConverter) Ready(ctx context.Context) error {
	if c.Python == "" || c.Script == "" {
		return unavailable()
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	defer diagnosticreport.TrackDeadline(parent, ctx)()
	if e := c.command(ctx, "--check").Run(); e != nil {
		return unavailable()
	}
	return nil
}
func (c PythonConverter) Convert(ctx context.Context, format, source, out string) (Manifest, error) {
	result := Manifest{}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 150 * time.Second
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer diagnosticreport.TrackDeadline(parent, ctx)()
	cmd := c.command(ctx, format, source, out)
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	cmd.Stdout = &limitedBuffer{}
	if e := cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return result, kProblem("conversion_timeout", "转换已取消或超过 150 秒时间边界。")
		}
		var report struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(stderr.Bytes(), &report) == nil && report.Code != "" {
			return result, kProblem(report.Code, report.Message)
		}
		return result, kProblem("conversion_failed", "转换器未能解析此文件，请查看支持的格式范围。")
	}
	info, e := os.Stat(filepath.Join(out, "manifest.json"))
	if e != nil || info.Size() > 64*1024*1024 {
		return result, kProblem("manifest_invalid", "转换器未返回有界清单。")
	}
	data, e := os.ReadFile(filepath.Join(out, "manifest.json"))
	if e != nil {
		return result, e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e = d.Decode(&result); e != nil {
		return result, kProblem("manifest_invalid", "转换清单不符合固定格式。")
	}
	return result, nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(data []byte) (int, error) {
	n := len(data)
	remaining := 8192 - b.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.Buffer.Write(data)
	}
	return n, nil
}
func kProblem(code, message string) error {
	return &Error{Code: code, Message: message, Status: 422, Issues: []Issue{}}
}
