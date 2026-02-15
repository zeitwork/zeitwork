package dockerfiles

import _ "embed"

//go:embed nuxt.Dockerfile
var nuxtDockerfile []byte

//go:embed nextjs.Dockerfile
var nextjsDockerfile []byte

//go:embed rails.Dockerfile
var railsDockerfile []byte

//go:embed laravel.Dockerfile
var laravelDockerfile []byte

// Get returns the Dockerfile content for a given framework name.
// Returns nil if the framework is not supported.
func Get(framework string) []byte {
	switch framework {
	case "nuxt":
		return nuxtDockerfile
	case "nextjs":
		return nextjsDockerfile
	case "rails":
		return railsDockerfile
	case "laravel":
		return laravelDockerfile
	default:
		return nil
	}
}
