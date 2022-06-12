VERSION ?= latest
REGISTRY ?= henderiw
IMG ?= $(REGISTRY)/set-image-with-configmap:${VERSION}

.PHONY: all
all: test

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: fmt vet ## Run tests.
	kpt fn render data

docker-build: test ## Build docker images.
	docker build -t ${IMG} .

docker-push: ## Build docker images.
	docker build -t ${IMG} .