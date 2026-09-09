package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/caarlos0/env/v11"
)

//go:generate go tool envdoc -types Config -output ../../envs.md

type Config struct {
	// HTTP server bind address.
	ListenAddress string `env:"LISTEN_ADDRESS" envDefault:":8080"`
	// CIDR ranges of reverse proxies whose X-Forwarded-For headers are trusted
	TrustedProxies []string `env:"TRUSTED_PROXIES"`
}

// RealIPHeader reports whether the deprecated REAL_IP_HEADER variable is set.
func RealIPHeader() (bool, error) {
	v := os.Getenv("REAL_IP_HEADER")
	if v == "" {
		return false, nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("env: %w", env.ParseError{
			Name: "RealIPHeader",
			Type: reflect.TypeFor[bool](),
			Err:  err,
		})
	}

	return b, nil
}
