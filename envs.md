# Environment Variables

## Config

 - `LISTEN_ADDRESS` (default: `:8080`) - HTTP server bind address.
 - `TRUSTED_PROXIES` (comma-separated) - CIDR ranges of reverse proxies whose X-Forwarded-For headers are trusted
 - `LIMIT_REQUESTS` (default: `10`) - HTTP rate limit requests.
 - `LIMIT_WINDOW` (default: `1m`) - HTTP rate limit window.

