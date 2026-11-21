//go:build !windows

package main

import "errors"

func watchTrayStop(name string) (<-chan struct{}, func(), error) {
	if name != "" {
		return nil, nil, errors.New("Tray shutdown events require Windows")
	}
	return nil, func() {}, nil
}
