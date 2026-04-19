package tui

type msgProgress struct {
	written int64
	total   int64
}

type msgDownloadDone struct {
	tmpPath string
}

type msgInstallDone struct {
	binaryPath string
	configPath string
	envPath    string
}

type msgErr struct {
	err error
}
