FROM            golang:1.7
MAINTAINER      Guillaume J. Charmes <guillaume@leaf.ag>

RUN             go get github.com/alecthomas/gometalinter && gometalinter -i

ARG             APP_NAME
WORKDIR         $GOPATH/src/$APP_NAME

ADD             .         $GOPATH/src/$APP_NAME
