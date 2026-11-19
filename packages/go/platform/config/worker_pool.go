package config

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/requestlimit"
	"os"
)

// WorkerPoolLimit lets a narrow worker load its own connection budget without
// opening HTTP, identity, object storage or Redis configuration.
func WorkerPoolLimit(role string, fallback int32) (int32, error) {
	arch, err := WorkerArchitecture(role)
	if err != nil || arch == nil {
		return fallback, err
	}
	if limit := arch.Pools[role]; limit > 0 {
		return arch.Connections(role), nil
	}
	return int32(arch.Share(int(fallback))), nil
}

func WorkerArchitecture(role string) (*Architecture, error) {
	return readArchitecture(os.Getenv("INFRAPRO_ARCHITECTURE_FILE"), role)
}

func WorkerObjectBudget(role string) (*requestlimit.Limit, error) {
	a, err := readArchitecture(os.Getenv("INFRAPRO_ARCHITECTURE_FILE"), role)
	if err != nil {
		return nil, err
	}
	return a.ObjectRequests(), nil
}
