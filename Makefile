PUBLISH=publish_weave publish_weavedns publish_weavetools

.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=zettio
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)
WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVETOOLS_IMAGE=$(DOCKERHUB_USER)/weavetools
WEAVER_EXPORT=/var/tmp/weave.tar
WEAVEDNS_EXPORT=/var/tmp/weavedns.tar
WEAVETOOLS_EXPORT=/var/tmp/weavetools.tar

all: $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT)

update:
	go get -u -f -v -tags -netgo ./$(dir $(WEAVER_EXE)) ./$(dir $(WEAVEDNS_EXE))

$(WEAVER_EXE) $(WEAVEDNS_EXE): common/*.go
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(shell dirname $@)
	@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go clean -i net"; \
		echo "    sudo go install -tags netgo std"; \
		false; \
	}

$(WEAVER_EXE): router/*.go weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go weavedns/main.go

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

$(WEAVEDNS_EXPORT): weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) weavedns
	$(SUDO) docker save $(WEAVEDNS_IMAGE):latest > $@

$(WEAVETOOLS_EXPORT): tools/Dockerfile tools/driver
	$(SUDO) docker build -t $(WEAVETOOLS_IMAGE) tools
	$(SUDO) docker save $(WEAVETOOLS_IMAGE):latest > $@

# Add more directories in here as more tests are created
tests:
	cd router; go test -cover -tags netgo
	cd nameserver; go test -cover -tags netgo

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVETOOLS_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT)
