.PHONY: build test vet clean docker-image

BINARY := hashi-pulse
CMD := ./cmd/hashi-pulse
IMAGE ?= hashi-pulse:local

build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

docker-image:
	docker build -t $(IMAGE) .

clean:
	rm -f $(BINARY)
