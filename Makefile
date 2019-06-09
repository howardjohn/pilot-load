
.PHONY: format
format:
	goimports -l -w -local github.com/howardjohn/pilot-load *.go cmd/*.go client/*.go

.PHONY: install
install:
	go install -v

.PHONY: docker
docker:
	docker build . -t ${HUB}/pilot-load

.PHONY: push
push:
	docker push ${HUB}/pilot-load

.PHONY: deploy
deploy:
	kubectl apply -f install

all: install docker push deploy
