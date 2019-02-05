GO       := go
COMMANDS  = $(shell ls cmd)
IMAGES    = $(shell ls build/docker/*.Dockerfile | sed 's:^.*/\(.*\).Dockerfile:\1:' )

all: build

# build the given commands
build: $(COMMANDS)

# clean the given commands
clean:
	@for cmd in $(COMMANDS); do \
	    echo "Cleaning up $$cmd..." && \
	    cd cmd/$$cmd && \
	    go clean && \
	    cd - >& /dev/null; \
	done

# build the given images
images: $(IMAGES)

# go-build the given commands
$(COMMANDS):
	@echo "Building $@..." && \
	    cd cmd/$@ && \
	    go build -o $@

# docker-build the given images
$(IMAGES):
	@echo "Building docker image for $@..." && \
	    build/docker/build-image.sh $@

.PHONY: all build clean images $(COMMANDS) $(IMAGES)
