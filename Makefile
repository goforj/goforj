GREEN  := $(shell tput -Txterm setaf 2)
WHITE  := $(shell tput -Txterm setaf 7)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

.PHONY: help install watcher

HELP_FUN = \
	%help; \
	while(<>) { \
		push @{$$help{$$2 // 'options'}}, [$$1, $$3] if /^([a-zA-Z\-]+)\s*:.*\#\#(?:@([a-zA-Z\-]+))?\s(.*)$$/ }; \
		print "\n"; \
		for (sort keys %help) { \
			print "${WHITE}$$_${RESET}\n"; \
			for (@{$$help{$$_}}) { \
				$$sep = " " x (32 - length $$_->[0]); \
				print "  ${YELLOW}$$_->[0]${RESET}$$sep${GREEN}$$_->[1]${RESET}\n"; \
			}; \
			print ""; \
		}; \
	}

help: ##@other Show this help.
	@perl -e '$(HELP_FUN)' $(MAKEFILE_LIST)

##@build
install: ##@build Build devconsole assets and install goforj.
	cd internal/forj/templates/internal/devconsole/ui && npm install
	cd internal/forj/templates/internal/devconsole/ui && npm run build
	go install ./cmd/forj

##@dev
watcher: ##@dev Run wgo go install for the CLI with watchers
	wgo go install ./cmd/forj
