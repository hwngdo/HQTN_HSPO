//go:build !windows

package appupdate

import "os"

func Token() string {
	if os.Getenv("HSPO_GITHUB_REPOSITORY") != Repository {
		return ""
	}
	return os.Getenv("HSPO_GITHUB_READ_TOKEN")
}
