cicd:
	act --secret-file=.secret.env --var-file=.vars.env $(ARGS)

deploy-%:
	./infra/deploy.sh $* $(ARGS)
