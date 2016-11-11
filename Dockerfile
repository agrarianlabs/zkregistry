FROM            golang:1.7
MAINTAINER      Guillaume J. Charmes <guillaume@leaf.ag>

RUN             go get github.com/alecthomas/gometalinter && gometalinter -i

ENV             APP_NAME      zkregistry
ENV             APP_PATH      github.com/agrarianlabs/$APP_NAME
WORKDIR         $GOPATH/src/$APP_PATH

ADD             .         $GOPATH/src/$APP_PATH
#RUN             go install
