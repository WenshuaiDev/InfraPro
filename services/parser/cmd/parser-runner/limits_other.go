//go:build !unix

package main

import "errors"

func limitProcess() error { return errors.New("parser runner requires Unix resource limits") }
