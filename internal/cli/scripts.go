package cli

import (
	_ "embed"
)

// Embed all bash scripts from the scripts directory

//go:embed scripts/main.sh
var MainScript string

//go:embed scripts/utils.sh
var UtilsScript string

//go:embed scripts/setup.sh
var SetupScript string

//go:embed scripts/deploy.sh
var DeployScript string

//go:embed scripts/status.sh
var StatusScript string

//go:embed scripts/logs.sh
var LogsScript string

//go:embed scripts/cleanup.sh
var CleanupScript string

//go:embed scripts/config.sh
var ConfigScript string

// GetScripts returns a map of all embedded scripts
func GetScripts() map[string]string {
	return map[string]string{
		"main.sh":    MainScript,
		"utils.sh":   UtilsScript,
		"setup.sh":   SetupScript,
		"deploy.sh":  DeployScript,
		"status.sh":  StatusScript,
		"logs.sh":    LogsScript,
		"cleanup.sh": CleanupScript,
		"config.sh":  ConfigScript,
	}
}
