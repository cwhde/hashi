.PHONY: build test vet clean docker-image

BINARY := hashi-pulse
IMAGE ?= hashi-pulse:local

build:
	dotnet publish -c Release -o dist /p:PublishAot=true
	@if [ -f dist/Hashi.Pulse ]; then cp dist/Hashi.Pulse $(BINARY); fi
	@if [ -f dist/Hashi.Pulse.exe ]; then cp dist/Hashi.Pulse.exe $(BINARY).exe; fi

test:
	dotnet test

vet:
	dotnet build

docker-image:
	docker build -t $(IMAGE) .

clean:
	dotnet clean
	rm -rf dist $(BINARY) $(BINARY).exe
