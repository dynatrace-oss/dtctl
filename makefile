.PHONY: markdownlint markdownlint-fix

MD_LINT_CLI_IMAGE := "ghcr.io/igorshubovych/markdownlint-cli:v0.31.1"

markdownlint:
	docker run -v $(CURDIR):/workdir --rm  $(MD_LINT_CLI_IMAGE)  "**/*.md"

markdownlint-fix:
	docker run -v $(CURDIR):/workdir --rm  $(MD_LINT_CLI_IMAGE)  "**/*.md" --fix

