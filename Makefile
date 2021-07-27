# Specify the directory location of main package after bin directory
# e.g. bin/{DIRECTORY_NAME_OF_APP}
VOLUME_EVENT_EXPORTER=volume-event-exporter

vendor-stamp: go.sum
	go mod vendor
	touch vendor-stamp

volume-event-exporter-bin: vendor-stamp
	@echo "--> Generating find developer server bin <--"
	@echo "==> Building ${VOLUME_EVENT_EXPORTER} using $(shell go version)..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/${VOLUME_EVENT_EXPORTER} ./cmd/volume-events-collector
	@echo "\n\n==> Results:"
	@ls -hl bin/
	@echo "${VOLUME_EVENT_EXPORTER} bin build succeeded"
