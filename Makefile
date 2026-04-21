.SILENT:
.PHONY: *

run: build
	 ./warthunder-commander

build:
	 go build -o warthunder-commander cmd/main.go

debug: build
	 ./warthunder-commander --debug

clear:
	rm -rf warthunder-commander*
	rm -rf dist
	rm -rf logs/*
