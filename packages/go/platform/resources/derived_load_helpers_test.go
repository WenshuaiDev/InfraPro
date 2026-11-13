//go:build monitoringintegration

package resources_test

import (
	"context"
	"encoding/csv"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"io"
)

func walkLoadCSV(download md.Download, visit func(map[string]int, []string) error) error {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		e := download.Write(context.Background(), writer)
		_ = writer.CloseWithError(e)
		done <- e
	}()
	csv := csv.NewReader(reader)
	csv.ReuseRecord = true
	header, e := csv.Read()
	columns := map[string]int{}
	for i, name := range header {
		columns[name] = i
	}
	for e == nil {
		var row []string
		row, e = csv.Read()
		if e == io.EOF {
			e = nil
			break
		}
		if e == nil {
			e = visit(columns, row)
		}
	}
	_ = reader.CloseWithError(e)
	writeError := <-done
	if e != nil {
		return e
	}
	return writeError
}
