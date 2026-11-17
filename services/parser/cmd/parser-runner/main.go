package main

import (
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/identification"
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
	var kind map[string]json.RawMessage
	if c.StrictJSON(b, &kind) != nil {
		os.Exit(1)
	}
	if _, ok := kind["ingestion_contract_version"]; ok {
		var req i.Request
		if c.StrictJSON(b, &req) != nil {
			os.Exit(1)
		}
		r, err := identification.Identify(req)
		if err != nil || json.NewEncoder(os.Stdout).Encode(r) != nil {
			os.Exit(1)
		}
		return
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
