.PHONY: build test vet clean

BINARY := hashi-pulse
CMD := ./cmd/hashi-pulse

build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
