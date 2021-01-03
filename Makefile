all: image

export PLATFORM=linux
export DOCKER_BUILDKIT=1

image:
	@docker build . --target bin \
	--platform ${PLATFORM} -t rmd:latest
