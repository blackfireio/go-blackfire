project ?= go-blackfire

# Project name must be compatible with docker-compose
override project := $(shell echo $(project) | tr -d -c '[a-z0-9]' | cut -c 1-55)

COMPOSE=docker-compose --project-directory . -f docker-compose.yml --project-name $(project)
RUN_DASHBOARD=$(COMPOSE) run --rm --no-deps go_dashboard
ifdef CI
	COMPOSE_BUILD_OPT = --progress=plain
endif

.DEFAULT_GOAL := help

##
#### Dashboard
##

dashboard-clean: ## Clean dirs
	@$(RUN_DASHBOARD) rm -Rf "build" "dist"
.PHONY: dashboard-clean

dashboard-build: dashboard/node_modules dashboard-clean ## Build the app
	@$(RUN_DASHBOARD) yarn build
	@$(RUN_DASHBOARD) yarn merge
	@$(RUN_DASHBOARD) node packaging/packager.js
	@$(RUN_DASHBOARD) rm -Rf build

	@echo "\n\n\n\nApp has been built in \033[32mdashboard/dist/index.html\033[0m, run \033[32myarn serve\033[0m to use it\n\n"
.PHONY: dashboard-build

dashboard-update-statik: dashboard-build
	@go get -u github.com/rakyll/statik
	@statik -src=dashboard/dist/

dashboard-serve-dev: dashboard/node_modules ## Serve the app for development purpose (live reload)
	@yarn --cwd=dashboard start
.PHONY: dashboard-serve-dev

dashboard-serve: dashboard-build ## Build then serve the app
	@yarn --cwd=dashboard serve
.PHONY: dashboard-serve

dashboard/node_modules: dashboard/yarn.lock
	@$(RUN_DASHBOARD) yarn

dashboard-test: dashboard/node_modules
	@$(RUN_DASHBOARD) yarn test

dashboard-eslint: dashboard/node_modules
	@$(RUN_DASHBOARD) yarn eslint

dashboard-build-docker:
	$(COMPOSE) build --pull --parallel $(COMPOSE_BUILD_OPT)

down: ## Stop and remove containers, networks, images, and volumes
	@$(COMPOSE) down --remove-orphans
.PHONY: down

help:
	@grep -hE '(^[a-zA-Z_-]+:.*?##.*$$)|(^###)' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}' | sed -e 's/\[32m##/[33m\n/'
.PHONY: help
