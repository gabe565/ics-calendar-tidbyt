package config

//go:generate go tool envdoc -types Config -output ../../envs.md

type Config struct {
	// HTTP server bind address.
	ListenAddress string `env:"LISTEN_ADDRESS" envDefault:":8080"`
	// Get client IP address from the "Real-IP" header.
	RealIPHeader bool `env:"REAL_IP_HEADER"                    default:"true"`
}
