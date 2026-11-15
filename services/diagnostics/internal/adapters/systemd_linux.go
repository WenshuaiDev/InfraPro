//go:build linux

package adapters

import "github.com/godbus/dbus/v5"

func systemBus() (PropertyReader, error) {
	conn, e := dbus.ConnectSystemBus()
	if e != nil {
		return nil, e
	}
	return &dbusReader{conn}, nil
}
