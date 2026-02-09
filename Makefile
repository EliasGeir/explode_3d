.PHONY: build run generate clean

build: generate
	CGO_ENABLED=1 go build -tags fts5 -o 3dmodels .

run: build
	./3dmodels

generate:
	templ generate ./...

clean:
	rm -f 3dmodels
	rm -f templates/*_templ.go
