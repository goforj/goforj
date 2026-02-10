GREEN  := $(shell tput -Txterm setaf 2)
WHITE  := $(shell tput -Txterm setaf 7)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

.PHONY: help install watcher node-deps
DEVCONSOLE_UI_DIR := templates/internal/devconsole/ui
DEMO_UI_DIR := templates/demo/frontend
NODE_CACHE_DIR := $(HOME)/.cache/goforj
DEMO_NODE_CACHE_DIR := $(NODE_CACHE_DIR)/demo-frontend/node_modules
DEVCONSOLE_NODE_CACHE_DIR := $(NODE_CACHE_DIR)/devconsole-ui/node_modules

HELP_FUN = \
	%help; \
	while(<>) { \
		push @{$$help{$$2 // 'options'}}, [$$1, $$3] if /^([a-zA-Z\-]+)\s*:.*\#\#(?:@([a-zA-Z\-]+))?\s(.*)$$/ }; \
		print "\n"; \
		for (sort keys %help) { \
			print "${WHITE}$$_${RESET \
		}\n"; \
		for (@{$$help{$$_}}) { \
			$$sep = " " x (32 - length $$_->[0]); \
			print "  ${YELLOW}$$_->[0]${RESET}$$sep${GREEN}$$_->[1]${RESET}\n"; \
		}; \
		print ""; \
	}

help: ##@other Show this help.
	@perl -e '$(HELP_FUN)' $(MAKEFILE_LIST)

##@build
install: ##@build Build devconsole assets and install goforj.
	$(MAKE) node-deps
	cd $(DEVCONSOLE_UI_DIR) && npm run build
	go install ./cmd/forj

node-deps: ##@build Link template node_modules to cache and install UI dependencies.
	mkdir -p $(DEMO_NODE_CACHE_DIR)
	mkdir -p $(DEVCONSOLE_NODE_CACHE_DIR)
	rm -rf $(DEMO_UI_DIR)/node_modules
	rm -rf $(DEVCONSOLE_UI_DIR)/node_modules
	cd $(DEMO_UI_DIR) && npm install
	rm -rf $(DEMO_NODE_CACHE_DIR)
	mv $(DEMO_UI_DIR)/node_modules $(DEMO_NODE_CACHE_DIR)
	ln -sfn $(DEMO_NODE_CACHE_DIR) $(DEMO_UI_DIR)/node_modules
	cd $(DEVCONSOLE_UI_DIR) && npm install
	rm -rf $(DEVCONSOLE_NODE_CACHE_DIR)
	mv $(DEVCONSOLE_UI_DIR)/node_modules $(DEVCONSOLE_NODE_CACHE_DIR)
	ln -sfn $(DEVCONSOLE_NODE_CACHE_DIR) $(DEVCONSOLE_UI_DIR)/node_modules

##@dev
watcher: ##@dev Run wgo go install for the CLI with watchers
	wgo go install ./cmd/forj
