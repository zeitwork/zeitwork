package bin

import _ "embed"

//go:embed traefik
var TraefikBinary []byte

//go:embed traefik.yaml
var TraefikConfig []byte
