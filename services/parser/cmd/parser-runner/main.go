package main

import (
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/methods"
	"io"
	"os"
)

func main() {
	if limitProcess() != nil {
		os.Exit(1)
	}
	b, e := io.ReadAll(io.LimitReader(os.Stdin, c.MaxRequest+1))
	if e != nil || len(b) > c.MaxRequest {
		os.Exit(1)
	}
	var req c.Request
	if c.StrictJSON(b, &req) != nil {
		os.Exit(1)
	}
	r, e := methods.Parse(req)
	if e != nil {
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
