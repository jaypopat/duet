.PHONY: build run clean test

build:
	go build -o bin/duet .

run: build
	./bin/duet

clean:
	rm -rf bin/

test:
	go test -v ./...

dev:
	go run main.go

install:
	go install .
