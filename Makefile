.PHONY: build run generate clean

build: generate
	go build -o 3dmodels .

run: build
	./3dmodels

generate:
	templ generate ./...

clean:
	rm -f 3dmodels
	rm -f templates/*_templ.go
