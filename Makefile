.ONESHELL:
SHELL := /bin/bash
GOBIN ?= $(GOPATH)/bin
MODULE = github.com/howardjohn/pilot-load
HUB ?= gcr.io/howardjohn-istio
export GO111MODULE ?= on

all: format lint install

.PHONY: check-git
check-git:
	@
	if [[ -n $$(git status --porcelain) ]]; then
		echo "Error: git is not clean"
		git status
		git diff
		exit 1
	fi

.PHONY: gen-check
gen-check: check-git format

.PHONY: format
format: $(GOBIN)/goimports
	@go mod tidy
	@goimports -l -w -local $(MODULE) .

.PHONY: lint
lint: $(GOBIN)/golangci-lint
	@golangci-lint run --fix

.PHONY: install
install:
	@go install

.PHONY: docker
docker:
	docker buildx build . -t ${HUB}/pilot-load --load

.PHONY: push
push:
	docker buildx build . -t ${HUB}/pilot-load --push

.PHONY: setup
setup:
	./kube/deploy.sh

.PHONY: deploy
deploy:
	kubectl apply -f install

all: install docker push deploy
