FROM golang:1.12

ADD . /go/src/github.com/howardjohn/pilot-load

RUN go install github.com/howardjohn/pilot-load

CMD /go/bin/pilot-load
