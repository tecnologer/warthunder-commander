.SILENT:
.PHONY: *

run: build
	 ./warthunder-commander

build:
	 go build -o warthunder-commander cmd/main.go

debug: build
	 ./warthunder-commander --debug
