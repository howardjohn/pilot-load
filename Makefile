format:
	goimports -l -w -local github.com/howardjohn/pilot-load *.go cmd/*.go client/*.go

install:
	go install -v

docker:
	docker build . -t ${HUB}/pilot-load

push:
	docker push ${HUB}/pilot-load

deploy:
	kubectl apply -f install

all: install docker push deploy
