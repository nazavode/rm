all: bin/rmd

PLATFORM=local
export DOCKER_BUILDKIT=1

.PHONY: bin/rmd
bin/rmd:
	@docker build . --target bin \
	--output bin/ \
	--platform ${PLATFORM}

.PHONY: image
image:
	@docker build . --target deploy \
	--platform linux \
	-t rmd:dev
