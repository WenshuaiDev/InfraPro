package config

import "fmt"

type MonitoringBackup struct{ Endpoint, AccessKey, SecretKey, Bucket, NamedCollection string }

func LoadMonitoringBackup(path string) (MonitoringBackup, error) {
	raw, e := loadInput(path)
	if e != nil {
		return MonitoringBackup{}, e
	}
	c := MonitoringBackup{raw.MonitoringBackupEndpoint, raw.MonitoringBackupAccessKey, raw.MonitoringBackupSecretKey, raw.MonitoringBackupBucket, raw.MonitoringBackupCollection}
	if c.Endpoint == "" || c.AccessKey == "" || c.SecretKey == "" || c.Bucket == "" || c.NamedCollection == "" {
		return c, fmt.Errorf("independent monitoring backup storage is not configured")
	}
	return c, nil
}
