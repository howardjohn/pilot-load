format:
	goimports -l -w -local github.com/howardjohn/pilot-load *.go cmd/*.go client/*.go

install:
	go install -v