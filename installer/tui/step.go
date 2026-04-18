package tui

type step int

const (
	stepWelcome step = iota
	stepInstallDir
	stepConfigFields // one sub-step per field
	stepConfirm
	stepDownloading
	stepInstalling
	stepDone
	stepError
)
