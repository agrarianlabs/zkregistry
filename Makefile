# RM as a variable so we can disable it for CircleCI and co.
RM_TMP ?= true
TTY    ?= true

ZK_I    = jplock/zookeeper:3.4.9
ZK_C    = zkregistry_zk_c
ZK_PORT = 2181
ZK_CLI ?= docker run --rm=$(RM_TMP) --link $(ZK_C):zk -i --entrypoint /bin/sh $(ZK_I) -c './bin/zkCli.sh -server zk' > /dev/null

TEST_I ?= agrarianlabs/zkregistry-test
TEST_C  = zkregistry_test_c
REV_ID  = $(shell git rev-parse HEAD)
REV_NAME= $(shell git describe --dirty --tags)

GLIDE_I ?= calavera/go-glide:v0.12.2
APP_NAME = github.com/agrarianlabs/zkregistry

# gometalinter options.
L_OPTS ?=
# go test options.
OPTS   ?= -v -run .

SRCS    = $(shell find . -name '*.go') \
          Dockerfile \
          glide.lock \
          vendor

# Default target.
all     : test

## Zookeeper target for integration tests.
zk_start: .zk_port
.zk_port:
	@echo "Starting zookeeper." >&2
# Make sure we don't have a previous instead not cleaned up.
	@docker rm -f -v $(ZK_C) > /dev/null 2> /dev/null || true
# Start zookeeper.
	@docker run -d -p $(ZK_PORT) --name $(ZK_C) $(ZK_I) > /dev/null
# Wait for it to be ready to serve.
	@while ! echo "ls /" | $(ZK_CLI) > /dev/null 2> /dev/null; do \
		echo "Waiting for zookeeper to start..." >&2; \
		sleep 1; \
		if [ "$(docker inspect -f '{{.State.Status}}' $(ZK_C))" = "exited" ]; then \
			echo 'Zookeeper failed to start.' >&2; \
			docker logs $(ZK_C) | tail -20; \
			exit 1; \
		fi; \
	done
# Lookup the port.
	@docker port $(ZK_C) $(ZK_PORT) | sed 's/.*://' > $@
	@echo "Zookeeper started." >&2

zk_stop :
	@docker rm -f -v $(ZK_C) > /dev/null 2> /dev/null || true

## !Zookeeper targets.

## Build targets.
build   : .build
.build  : $(SRCS)
	@echo "Building docker image." >&2
	docker build --rm=$(RM_TMP) -t '$(TEST_I):$(REV_ID)' --build-arg APP_NAME=$(APP_NAME) .

	@$(eval NOW := $(shell date -u +%Y-%m-%d.%H-%M-%S))
	docker tag '$(TEST_I):$(REV_ID)' '$(TEST_I):$(NOW).$(REV_ID)'
	docker tag '$(TEST_I):$(REV_ID)' '$(TEST_I):$(NOW).$(REV_NAME)'

	@echo "Docker image built" >&2
	@touch $@
## !Build targts.

## Vendor targets.
glide_up: glide.yaml
	@rm -rf vendor glide.lock
	docker run -i -t=$(TTY) --rm=$(RM_TMP) \
		-v $(shell pwd):/go/src/$(APP_NAME) \
		-w /go/src/$(APP_NAME) \
	$(GLIDE_I) \
	glide up -v --skip-test --all-dependencies
## !Vendor targets.

# Unless we specify SKIP_FMT=1, run test_fmt with test.
ifeq ($(SKIP_FMT),)
test    : test_fmt
endif

# run test_int with test.
test    : test_int

# The fmt depends on the docker image to be built.
test_fmt: build
	docker run -i -t=$(TTY) --rm=$(RM_TMP) $(TEST_I):$(REV_ID) sh -c 'go install && \
		gometalinter --tests --vendor --enable-all \
		  --deadline       5m  \
                  --line-length    120 \
		  --cyclo-over     15  \
		  --dupl-threshold 100 \
		  $(L_OPTS)            \
		  ./...'

# The integration test depends on zk to be started and the docker image to be built.
test_int: zk_start build
	@docker rm -f -v $(TEST_C) > /dev/null 2> /dev/null || true
	docker run -i -t=$(TTY) --name $(TEST_C) --link '$(ZK_C):zk' '$(TEST_I):$(REV_ID)' go test $(OPTS) -cover -covermode=count -coverprofile=/tmp/coverprofile -zk-host=zk
	@docker cp $(TEST_C):/tmp/coverprofile coverprofile
	@docker rm -f -v $(TEST_C) > /dev/null 2> /dev/null || true

clean   : zk_stop
	@rm -f coverprofile .build .zk_port

.PHONY  : all zk_start zk_stop build test test_fmt test_int clean glide_up
