.DEFAULT_GOAL := help

# Export inputs as environment variables; never interpolate versions into shell code.
export VERSION
export RELEASE_DIR
export PLAYWRIGHT_CHANNEL
export TEST_WORKERS
export OPEN_BROWSER
export USERNAME
export GENERATE_PASSWORD
export VPN_API_BIN
export VPN_DATA_DIR
export VPN_CREDENTIALS_DIR
export FRONTEND_PORT

.PHONY: help install start stop restart status logs test check build release publish admin-create admin-reset-password admin-reset-two-factor native-start native-stop native-restart native-status native-logs

help:
	@python3 scripts/project.py help

install test check build:
	@python3 scripts/project.py $@

start stop restart status logs:
	@python3 scripts/compose.py $@

# Each target names its action explicitly so an inherited ADMIN_ACTION cannot change it.
admin-create:
	@ADMIN_ACTION=create sh scripts/admin-create.sh

admin-reset-password admin-reset-two-factor:
	@ADMIN_ACTION=$(patsubst admin-%,%,$@) sh scripts/admin-create.sh

native-start native-stop native-restart native-status native-logs:
	@$(MAKE) --no-print-directory -C app/backend $@

release:
	@python3 scripts/release.py

publish:
	@python3 scripts/publish.py
