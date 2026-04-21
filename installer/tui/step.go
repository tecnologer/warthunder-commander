package tui

type step int

const (
	stepWelcome step = iota
	stepInstallDir
	stepConfigFields // one screen per TOML section
	stepEnvVarPrompt // yes/no: set the value of this env var?
	stepEnvVarValue  // password input for the env var value
	stepConfirm
	stepDownloading
	stepInstalling
	stepDone
	stepError
)
