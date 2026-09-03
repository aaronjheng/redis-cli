package main

import (
	"fmt"
	"runtime"
)

const (
	version = "1.0.0"
)

func fullVersion() string {
	return fmt.Sprintf("%s (%s %s/%s)", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
