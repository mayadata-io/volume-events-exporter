# Copyright © 2021 The MayaData Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ifeq (${XC_ARCH}, )
  XC_ARCH:=$(shell go env GOARCH)
endif
export XC_ARCH

# The images can be pushed to any docker/image registeries
# like docker hub, quay. The registries are specified in 
# the `buildscripts/push` script.
#
# The images of a project or company can then be grouped
# or hosted under a unique organization key like `openebs`
#
# Each component (container) will be pushed to a unique 
# repository under an organization. 
# Putting all this together, an unique uri for a given 
# image comprises of:
#   <registry url>/<image org>/<image repo>:<image-tag>
#
# IMAGE_ORG can be used to customize the organization 
# under which images should be pushed. 
# By default the organization name is `openebs`. 

ifeq (${IMAGE_ORG}, )
  IMAGE_ORG = mayadataio
endif

# Default tag for the image
# If IMAGE_TAG is mentioned then TAG will be set to IMAGE_TAG
# If RELEASE_TAG is mentioned then TAG will be set to RELEAE_TAG
# If both are mentioned then TAG will be set to RELEASE_TAG
TAG=ci

ifneq (${IMAGE_TAG}, )
  TAG=${IMAGE_TAG:v%=%}
endif

ifneq (${RELEASE_TAG}, )
  TAG=${RELEASE_TAG:v%=%}
endif

# list only the source code directories
PACKAGES = $(shell go list ./... | grep -v 'vendor\|pkg/client/generated\|tests')

# list only the integration tests code directories
PACKAGES_IT = $(shell go list ./... | grep -v 'vendor\|pkg/client/generated' | grep 'tests')

# Specify the directory location of main package after bin directory
# e.g. bin/{DIRECTORY_NAME_OF_APP}
VOLUME_EVENTS_EXPORTER=volume-events-exporter

# Specify the date of build
DBUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# final tag name for volume events exporter
VOLUME_EVENTS_EXPORTER_IMAGE_TAG=${IMAGE_ORG}/${VOLUME_EVENTS_EXPORTER}:${TAG}

# Specify the docker arg for repository url
ifeq (${DBUILD_REPO_URL}, )
  DBUILD_REPO_URL="https://github.com/mayadata-io/volume-events-exporter"
  export DBUILD_REPO_URL
endif

# Specify the docker arg for website url
ifeq (${DBUILD_SITE_URL}, )
  DBUILD_SITE_URL="https://mayadata.io"
  export DBUILD_SITE_URL
endif

export DBUILD_ARGS=--build-arg DBUILD_DATE=${DBUILD_DATE} --build-arg DBUILD_REPO_URL=${DBUILD_REPO_URL} --build-arg DBUILD_SITE_URL=${DBUILD_SITE_URL} --build-arg RELEASE_TAG=${TAG}

.PHONY: deps
deps:
	@echo "--> Tidying up submodules"
	@go mod tidy
	@echo "--> Veryfying submodules"
	@go mod verify

.PHONY: verify-deps
verify-deps: deps
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

.PHONY: vendor
vendor: go.mod go.sum deps
	@go mod vendor

.PHONY: test-coverage
test-coverage: format vet
	@echo "--> Running go test";
	$(PWD)/buildscripts/test.sh ${XC_ARCH}

.PHONY: vet
vet:
	@echo "--> Running go vet"
	@go list ./... | grep -v "./vendor/*" | xargs go vet -composites

.PHONY: testv
testv: format
	@echo "--> Running go test verbose" ;
	@go test -v $(PACKAGES)

.PHONY: format
format:
	@echo "--> Running go fmt"
	@go fmt $(PACKAGES) $(PACKAGES_IT)

vendor-stamp: go.sum
	go mod vendor
	touch vendor-stamp

.PHONY: volume-events-exporter-bin
volume-events-exporter-bin: vendor-stamp
	@echo "-----------------------------------------------"
	@echo "--> Generating volume-events-exporter bin <--"
	@echo "-----------------------------------------------"
	@PNAME=${VOLUME_EVENTS_EXPORTER} CTLNAME=${VOLUME_EVENTS_EXPORTER} sh -c "'$(PWD)/buildscripts/build.sh'"

.PHONY: volume-events-exporter-image
volume-events-exporter-image: volume-events-exporter-bin
	@echo "-----------------------------------------------"
	@echo "--> Building volume-events-exporter Image <--"
	@echo "-----------------------------------------------"
	@cp bin/volume-events-exporter/${VOLUME_EVENTS_EXPORTER} buildscripts/volume-events-exporter/
	@cd buildscripts/volume-events-exporter && docker build -t ${VOLUME_EVENTS_EXPORTER_IMAGE_TAG} ${DBUILD_ARGS} . --no-cache
	@rm buildscripts/volume-events-exporter/${VOLUME_EVENTS_EXPORTER}

# Bootstrap downloads tools required during build time:
## Tool1: golangci-lint tool used to check linting tools in codebase
## Example: golangci-lint document is not recommending
##			to use `go get <path>`. For more info:
##          https://golangci-lint.run/usage/install/#install-from-source
##
## Install golangci-lint only if tool doesn't exist in system
## Fetch hook_types from openebs/dynamic-nfs-provisioner
.PHONY: bootstrap
bootstrap:
	@echo "Install golangci-lint tool"
	$(if $(shell which golangci-lint), @echo "golangci-lint already exist in system", (curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo sh -s -- -b "${GOPATH}/bin" v1.40.1))
	wget https://raw.githubusercontent.com/openebs/dynamic-nfs-provisioner/HEAD/pkg/hook/types.go -O tests/hook_types.go
	sed -i 's/package hook/package tests/g' tests/hook_types.go

## Currently we are running with Default options + other options
## Explanation for explicitly mentioned linters:
## exportloopref: checks for pointers to enclosing loop variables
## dupl: Tool for code clone detection within repo
## revive: Drop-in replacement of golint. It allows to enable or disable
##         rules using configuration file.
## bodyclose: checks whether HTTP response body is closed successfully
## goconst: Find repeated strings that could be replaced by a constant
## misspell: Finds commonly misspelled English words in comments
##
## NOTE: Disabling structcheck since it is reporting false positive cases
##       for more information look at https://github.com/golangci/golangci-lint/issues/537
## Skip checking test files they may contain duplicate code
.PHONY: golangcilint
golangcilint:
	@echo "--> Running golint"
	golangci-lint run --tests=false -E exportloopref,dupl,revive,bodyclose,goconst,misspell -D structcheck --timeout 5m0s
	@echo "Completed golangci-lint no recommendations !!"
	@echo "--------------------------------"
	@echo ""

.PHONY: license-check
license-check:
	@echo "--> Checking license header..."
	@licRes=$$(for file in $$(find . -type f -regex '.*\.sh\|.*\.go\|.*Docker.*\|.*\Makefile*' ! -path './vendor/*' ) ; do \
               awk 'NR<=5' $$file | grep -Eq "(Copyright|generated|GENERATED)" || echo $$file; \
       done); \
       if [ -n "$${licRes}" ]; then \
               echo "license header checking failed:"; echo "$${licRes}"; \
               exit 1; \
       fi
	@echo "--> Done checking license."
	@echo

.PHONY: sanity-test
sanity-test: sanity-test
	@echo "--> Running sanity test";
	go test -v -timeout 40m ./tests/...

.PHONY: clean
clean:
	@echo "--> Cleaning Directory"
	go clean -testcache
	rm -rf bin/

.PHONY: push
push:
	DIMAGE=${IMAGE_ORG}/${VOLUME_EVENTS_EXPORTER} ./buildscripts/push.sh
