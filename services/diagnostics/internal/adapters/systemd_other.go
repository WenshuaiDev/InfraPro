//go:build !linux

package adapters

import "github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"

func systemBus() (PropertyReader, error) { return nil, settings.Error("adapter_unsupported") }
