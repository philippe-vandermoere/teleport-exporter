BINARY := teleport-exporter
IMAGE := teleport-exporter:dev

.PHONY: build test vet fmt docker-build e2e-test clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/teleport-exporter

test:
	go test ./... -race

vet:
	go vet ./...

fmt:
	gofmt -l .

docker-build:
	docker build -t $(IMAGE) .

e2e-test:
	./test/e2e/run.sh

clean:
	rm -rf bin/
