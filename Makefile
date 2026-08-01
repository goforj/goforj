LIGHTHOUSE_UI_DIR := templates/internal/lighthouse/ui
DEMO_UI_DIR := templates/demo/frontend
NODE_CACHE_DIR := $(HOME)/.cache/goforj
DEMO_NODE_CACHE_DIR := $(NODE_CACHE_DIR)/demo-frontend/node_modules
LIGHTHOUSE_NODE_CACHE_DIR := $(NODE_CACHE_DIR)/lighthouse-ui/node_modules

HELP_FUN = %help; while (<>) { /^([A-Za-z0-9_-]+)\s*:.*\#\#(?:@([A-Za-z0-9_-]+))?\s(.*)$$/ or next; push @{$$help{$$2 || "other"}}, [$$1, $$3]; $$width = length($$1) if length($$1) > $$width } print "\e[1;97m$(or $(HELP_NAME),$(notdir $(CURDIR)))\e[0m\n\n"; for $$category (sort keys %help) { print "\e[1;97m$$category\e[0m\n"; for $$entry (@{$$help{$$category}}) { printf "  \e[1;32m%-*s\e[0m  \e[90m%s\e[0m\n", $$width, $$entry->[0], $$entry->[1] } }

help: ##@other Show this help.
	@perl -e '$(HELP_FUN)' $(MAKEFILE_LIST)

##@build
install: ##@build Build lighthouse assets and install goforj.
	$(MAKE) deps
	cd $(LIGHTHOUSE_UI_DIR) && npm run build
	go install ./cmd/forj

deps: ##@build Link template node_modules to cache and install UI dependencies.
	mkdir -p $(DEMO_NODE_CACHE_DIR)
	mkdir -p $(LIGHTHOUSE_NODE_CACHE_DIR)
	rm -rf $(DEMO_UI_DIR)/node_modules
	rm -rf $(LIGHTHOUSE_UI_DIR)/node_modules
	cd $(DEMO_UI_DIR) && npm install
	rm -rf $(DEMO_NODE_CACHE_DIR)
	mv $(DEMO_UI_DIR)/node_modules $(DEMO_NODE_CACHE_DIR)
	ln -sfn $(DEMO_NODE_CACHE_DIR) $(DEMO_UI_DIR)/node_modules
	cd $(LIGHTHOUSE_UI_DIR) && npm install
	rm -rf $(LIGHTHOUSE_NODE_CACHE_DIR)
	mv $(LIGHTHOUSE_UI_DIR)/node_modules $(LIGHTHOUSE_NODE_CACHE_DIR)
	ln -sfn $(LIGHTHOUSE_NODE_CACHE_DIR) $(LIGHTHOUSE_UI_DIR)/node_modules

##@dev
watch: ##@dev Reinstall the CLI when Go files change.
	wgo go install ./cmd/forj
