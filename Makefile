BIN     := gapull
REPO    := 029527/gapull
DIST    := dist
LDFLAGS := -ldflags="-s -w"

TARGETS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64

.PHONY: build clean release

build:
	@mkdir -p $(DIST)
	@$(foreach t,$(TARGETS), \
		$(eval OS   := $(word 1,$(subst /, ,$(t)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(t)))) \
		echo "building $(OS)/$(ARCH)..." && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(DIST)/$(BIN)-$(OS)-$(ARCH) . ; \
	)
	@echo "" && ls -lh $(DIST)/

clean:
	rm -rf $(DIST)

# make release TAG=v1.0.0
release: build
	@test -n "$(TAG)" || (echo "用法: make release TAG=v1.0.0" && exit 1)
	gh release create $(TAG) \
		--repo "$(REPO)" \
		--title "$(TAG)" \
		--generate-notes \
		$(DIST)/*
