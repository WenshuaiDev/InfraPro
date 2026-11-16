package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Parsing struct {
	ObjectEndpoint, ObjectAccessKey, ObjectSecretKey, Bucket string
	Enabled                                                  bool
	URL, TokenFile                                           string
	Timeout                                                  time.Duration
	Queue, Concurrency                                       int
	StorageBudget                                            int64
}
type parsingInput struct {
	ObjectEndpoint  string `yaml:"object_endpoint"`
	ObjectAccessKey string `yaml:"object_access_key"`
	ObjectSecretKey string `yaml:"object_secret_key"`
	Bucket          string `yaml:"bucket"`
	Enabled         string `yaml:"enabled"`
	URL             string `yaml:"url"`
	TokenFile       string `yaml:"token_file"`
	Timeout         string `yaml:"timeout"`
	Queue           string `yaml:"queue"`
	Concurrency     string `yaml:"concurrency"`
	StorageBudget   string `yaml:"storage_budget"`
}

func defaultParsingInput() parsingInput {
	return parsingInput{Enabled: "false", Timeout: "10s", Queue: "20", Concurrency: "2", StorageBudget: "10737418240"}
}
func parsingEnvironment(raw *parsingInput) {
	v := reflect.ValueOf(raw).Elem()
	for i := 0; i < v.NumField(); i++ {
		if value, ok := os.LookupEnv("INFRAPRO_PARSING_" + strings.ToUpper(v.Type().Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
}
func parseParsing(raw parsingInput) (Parsing, error) {
	var out Parsing
	var e error
	out.Enabled, e = strconv.ParseBool(raw.Enabled)
	if e != nil {
		return out, fmt.Errorf("invalid parsing enabled")
	}
	out.ObjectEndpoint, out.ObjectAccessKey, out.ObjectSecretKey, out.Bucket = raw.ObjectEndpoint, raw.ObjectAccessKey, raw.ObjectSecretKey, raw.Bucket
	if out.ObjectEndpoint != "" && (out.ObjectAccessKey == "" || out.ObjectSecretKey == "" || out.Bucket == "") {
		out.ObjectEndpoint, out.ObjectAccessKey, out.ObjectSecretKey, out.Bucket = "", "", "", ""
		out.Enabled = false
	}
	out.URL = raw.URL
	out.TokenFile = raw.TokenFile
	if out.URL == "" || out.TokenFile == "" {
		out.Enabled = false
	}
	out.Timeout, e = time.ParseDuration(raw.Timeout)
	if e != nil || out.Timeout <= 0 || out.Timeout > 10*time.Second {
		return out, fmt.Errorf("invalid parsing timeout")
	}
	out.Queue, e = strconv.Atoi(raw.Queue)
	if e != nil || out.Queue < 1 || out.Queue > 200 {
		return out, fmt.Errorf("invalid parsing queue")
	}
	out.Concurrency, e = strconv.Atoi(raw.Concurrency)
	if e != nil || out.Concurrency < 1 || out.Concurrency > 2 {
		return out, fmt.Errorf("invalid parsing concurrency")
	}
	out.StorageBudget, e = strconv.ParseInt(raw.StorageBudget, 10, 64)
	if e != nil || out.StorageBudget < 1 {
		return out, fmt.Errorf("invalid parsing storage budget")
	}
	return out, nil
}
