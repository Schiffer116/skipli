cicd:
	act --env-file=<(aws configure export-credentials --profile $$AWS_PROFILE --format env-no-export) \
		--var-file=<(gh variable list --json name,value -q '.[] | "\(.name)=\(.value)"') \
		$(ARGS)

deploy-%:
	./infra/deploy.sh $* $(ARGS)
